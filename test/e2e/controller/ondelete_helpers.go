// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"
	"github.com/gardener/etcd-druid/test/e2e/testenv"
	e2eutils "github.com/gardener/etcd-druid/test/e2e/utils"
	testutils "github.com/gardener/etcd-druid/test/utils"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/retry"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/gomega"
)

const (
	// druidDeployNamespace is the place where the operator + its configmap live in an e2e run.
	druidDeployNamespace           = "etcd-druid-e2e"
	druidOperatorConfigMapName     = "etcd-druid-operator-config"
	druidDeploymentName            = "etcd-druid"
	quorumAwareUpdatesGateName     = "QuorumAwareUpdates"
	timeoutOnDeleteRolloutStart    = 30 * time.Second
	timeoutOnDeleteRolloutComplete = 8 * time.Minute
	timeoutReplacementPending      = 3 * time.Minute
	timeoutReplacementReady        = 5 * time.Minute
	timeoutEtcdConditionChange     = 1 * time.Minute
	// timeoutAllMembersNotReady must exceed the 1m etcdMemberUnknownThreshold after which a member whose lease is not renewed stops being Ready.
	timeoutAllMembersNotReady = 3 * time.Minute
	// podsUntouchedWindow spans at least two requeues (10s) of the OnDelete controller.
	podsUntouchedWindow = 30 * time.Second
)

// isOnDeleteGateEnabled reports whether the deployed operator has the QuorumAwareUpdates feature gate enabled.
func isOnDeleteGateEnabled(t *testing.T, env *testenv.TestEnvironment) bool {
	t.Helper()
	cm := &corev1.ConfigMap{}
	err := env.Client().Get(env.Context(), types.NamespacedName{
		Name: druidOperatorConfigMapName, Namespace: druidDeployNamespace,
	}, cm)
	if err != nil {
		t.Fatalf("cannot determine %s gate state from operator ConfigMap %s/%s: %v", quorumAwareUpdatesGateName, druidDeployNamespace, druidOperatorConfigMapName, err)
	}
	for _, v := range cm.Data {
		if strings.Contains(v, quorumAwareUpdatesGateName+": true") {
			return true
		}
	}
	return false
}

// skipIfOnDeleteGateDisabled skips the test unless the deployed operator has the QuorumAwareUpdates feature gate enabled.
func skipIfOnDeleteGateDisabled(t *testing.T, env *testenv.TestEnvironment) {
	t.Helper()
	if !isOnDeleteGateEnabled(t, env) {
		t.Skipf("skipping: %s feature gate is not enabled; run 'make ci-e2e-kind-ondelete' to enable", quorumAwareUpdatesGateName)
	}
}

// skipIfOnDeleteGateEnabled skips the test if the QuorumAwareUpdates gate is enabled.
// Used for the gate-off scenario that verifies RollingUpdate behaviour is preserved.
func skipIfOnDeleteGateEnabled(t *testing.T, env *testenv.TestEnvironment) {
	t.Helper()
	if isOnDeleteGateEnabled(t, env) {
		t.Skipf("skipping gate-off scenario: %s feature gate is enabled", quorumAwareUpdatesGateName)
	}
}

// newOnDeleteTestEtcd builds the TLS-enabled Etcd without a backup store that the OnDelete tests use.
func newOnDeleteTestEtcd(namespace string, replicas int32) *druidv1alpha1.Etcd {
	return testutils.EtcdBuilderWithoutDefaults(defaultEtcdName, namespace).
		WithReplicas(replicas).
		WithEtcdClientPort(ptr.To[int32](2379)).
		WithClientTLS().
		WithPeerTLS().
		WithGRPCGatewayEnabled().
		WithBackupRestoreTLS().
		Build()
}

// waitForStsUpdateStrategy blocks until the StatefulSet's spec.updateStrategy.type matches the expected strategy.
func waitForStsUpdateStrategy(g *WithT, env *testenv.TestEnvironment, logger logr.Logger, etcd *druidv1alpha1.Etcd, expected appsv1.StatefulSetUpdateStrategyType, timeout time.Duration) {
	logger.Info("waiting for StatefulSet update strategy", "expected", expected)
	g.Eventually(func() (appsv1.StatefulSetUpdateStrategyType, error) {
		sts := &appsv1.StatefulSet{}
		if err := env.Client().Get(env.Context(), types.NamespacedName{Name: etcd.Name, Namespace: etcd.Namespace}, sts); err != nil {
			return "", err
		}
		return sts.Spec.UpdateStrategy.Type, nil
	}, timeout, 2*time.Second).Should(Equal(expected))
}

// waitForOnDeleteRolloutComplete blocks until all pods of the Etcd's sts carry the target updateRevision on their controller-revision-hash label.
func waitForOnDeleteRolloutComplete(g *WithT, env *testenv.TestEnvironment, logger logr.Logger, etcd *druidv1alpha1.Etcd, timeout time.Duration) {
	logger.Info("waiting for OnDelete rollout to complete")
	g.Eventually(func() error {
		sts, pods, err := listStsPods(env, etcd)
		if err != nil {
			return err
		}
		if sts.Status.UpdateRevision == "" {
			return fmt.Errorf("statefulSet has no updateRevision yet")
		}
		if sts.Spec.Replicas == nil {
			return fmt.Errorf("statefulSet has nil replicas")
		}
		if len(pods) != int(*sts.Spec.Replicas) {
			return fmt.Errorf("expected %d pods, got %d", *sts.Spec.Replicas, len(pods))
		}
		for i := range pods {
			p := &pods[i]
			if p.DeletionTimestamp != nil {
				return fmt.Errorf("pod %s is terminating", p.Name)
			}
			if p.Labels[appsv1.StatefulSetRevisionLabel] != sts.Status.UpdateRevision {
				return fmt.Errorf("pod %s at revision %s, target %s", p.Name, p.Labels[appsv1.StatefulSetRevisionLabel], sts.Status.UpdateRevision)
			}
		}
		return nil
	}, timeout, 5*time.Second).Should(Succeed())
}

// assertRolloutConditions verifies that after a rollout completes the Etcd resource reports all members updated and ready.
func assertRolloutConditions(g *WithT, env *testenv.TestEnvironment, logger logr.Logger, etcd *druidv1alpha1.Etcd, timeout time.Duration) {
	logger.Info("asserting post-rollout status conditions")
	g.Eventually(func() error {
		fetched, err := env.GetEtcd(etcd.Name, etcd.Namespace)
		if err != nil {
			return err
		}
		if !ptr.Deref(fetched.Status.Ready, false) {
			return fmt.Errorf("etcd.status.ready is not true")
		}
		for _, condType := range []druidv1alpha1.ConditionType{druidv1alpha1.ConditionTypeAllMembersUpdated, druidv1alpha1.ConditionTypeAllMembersReady} {
			if status := etcdConditionStatus(fetched, condType); status != druidv1alpha1.ConditionTrue {
				return fmt.Errorf("%s condition is %q, want True", condType, status)
			}
		}
		return nil
	}, timeout, 5*time.Second).Should(Succeed())
	logger.Info("post-rollout conditions verified: AllMembersUpdated=True, AllMembersReady=True, etcd.status.ready=true")
}

// etcdConditionStatus returns the status of the given condition, or "" if the Etcd does not report it.
func etcdConditionStatus(etcd *druidv1alpha1.Etcd, condType druidv1alpha1.ConditionType) druidv1alpha1.ConditionStatus {
	for _, cond := range etcd.Status.Conditions {
		if cond.Type == condType {
			return cond.Status
		}
	}
	return ""
}

// waitForEtcdCondition blocks until the Etcd reports the given condition with the given status.
func waitForEtcdCondition(g *WithT, env *testenv.TestEnvironment, logger logr.Logger, etcd *druidv1alpha1.Etcd, condType druidv1alpha1.ConditionType, status druidv1alpha1.ConditionStatus, timeout time.Duration) {
	logger.Info("waiting for Etcd condition", "type", condType, "status", status)
	g.Eventually(func() (druidv1alpha1.ConditionStatus, error) {
		fetched, err := env.GetEtcd(etcd.Name, etcd.Namespace)
		if err != nil {
			return "", err
		}
		return etcdConditionStatus(fetched, condType), nil
	}, timeout, 2*time.Second).Should(Equal(status), "Etcd condition %s", condType)
}

// triggerPodTemplateUpdate changes the pod template through the Etcd spec without waiting for the Etcd to become
// ready, which under OnDelete only happens once the whole rollout has finished.
func triggerPodTemplateUpdate(g *WithT, env *testenv.TestEnvironment, logger logr.Logger, etcd *druidv1alpha1.Etcd) {
	logger.Info("updating Etcd spec to change the pod template")
	g.Expect(retry.RetryOnConflict(retry.DefaultRetry, func() error {
		if err := env.Client().Get(env.Context(), client.ObjectKeyFromObject(etcd), etcd); err != nil {
			return err
		}
		etcd.Spec.Etcd.Metrics = ptr.To(druidv1alpha1.Extensive)
		metav1.SetMetaDataAnnotation(&etcd.ObjectMeta, druidv1alpha1.DruidOperationAnnotation, druidv1alpha1.DruidOperationReconcile)
		return env.Client().Update(env.Context(), etcd)
	})).To(Succeed())
}

// listStsPods returns the Etcd's StatefulSet and the pods it selects.
func listStsPods(env *testenv.TestEnvironment, etcd *druidv1alpha1.Etcd) (*appsv1.StatefulSet, []corev1.Pod, error) {
	sts := &appsv1.StatefulSet{}
	if err := env.Client().Get(env.Context(), types.NamespacedName{Name: druidv1alpha1.GetStatefulSetName(etcd.ObjectMeta), Namespace: etcd.Namespace}, sts); err != nil {
		return nil, nil, err
	}
	if sts.Spec.Selector == nil {
		return nil, nil, fmt.Errorf("statefulSet %s has no selector", sts.Name)
	}
	pods := &corev1.PodList{}
	if err := env.Client().List(env.Context(), pods, client.InNamespace(etcd.Namespace), client.MatchingLabels(sts.Spec.Selector.MatchLabels)); err != nil {
		return nil, nil, err
	}
	return sts, pods.Items, nil
}

// stsPodSnapshot records a StatefulSet's update revision and the UIDs of its pods, all of which run that revision.
type stsPodSnapshot struct {
	updateRevision string
	podUIDs        map[string]types.UID
}

// snapshotStsPods records the Etcd's StatefulSet pods and asserts that all of them run the current update revision.
func snapshotStsPods(g *WithT, env *testenv.TestEnvironment, logger logr.Logger, etcd *druidv1alpha1.Etcd) stsPodSnapshot {
	sts, pods, err := listStsPods(env, etcd)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(sts.Status.UpdateRevision).ToNot(BeEmpty())
	g.Expect(pods).To(HaveLen(int(ptr.Deref(sts.Spec.Replicas, 0))))
	snapshot := stsPodSnapshot{updateRevision: sts.Status.UpdateRevision, podUIDs: make(map[string]types.UID, len(pods))}
	for i := range pods {
		g.Expect(pods[i].Labels).To(HaveKeyWithValue(appsv1.StatefulSetRevisionLabel, sts.Status.UpdateRevision), "pod %s is not at the update revision", pods[i].Name)
		snapshot.podUIDs[pods[i].Name] = pods[i].UID
	}
	logger.Info("recorded StatefulSet pods", "updateRevision", snapshot.updateRevision, "podUIDs", snapshot.podUIDs)
	return snapshot
}

// podNamesExcept returns the sorted names of the recorded pods other than name.
func (s stsPodSnapshot) podNamesExcept(name string) []string {
	names := make([]string, 0, len(s.podUIDs))
	for podName := range s.podUIDs {
		if podName != name {
			names = append(names, podName)
		}
	}
	slices.Sort(names)
	return names
}

// waitForPendingReplacement waits until exactly one recorded pod has been replaced by an unschedulable pod at the new
// update revision while the other recorded pods still exist, and returns the replacement.
func waitForPendingReplacement(g *WithT, env *testenv.TestEnvironment, logger logr.Logger, etcd *druidv1alpha1.Etcd, snapshot stsPodSnapshot, timeout time.Duration) *corev1.Pod {
	logger.Info("waiting for one pod to be replaced by an unschedulable pod at the new revision")
	var replacement *corev1.Pod
	g.Eventually(func() error {
		sts, pods, err := listStsPods(env, etcd)
		if err != nil {
			return err
		}
		if sts.Status.UpdateRevision == snapshot.updateRevision {
			return fmt.Errorf("statefulSet has no new updateRevision yet")
		}
		if len(pods) != len(snapshot.podUIDs) {
			return fmt.Errorf("expected %d pods, got %d", len(snapshot.podUIDs), len(pods))
		}
		var replaced []*corev1.Pod
		for i := range pods {
			pod := &pods[i]
			if pod.UID != snapshot.podUIDs[pod.Name] {
				replaced = append(replaced, pod)
			} else if pod.DeletionTimestamp != nil {
				return fmt.Errorf("original pod %s is terminating", pod.Name)
			}
		}
		if len(replaced) > 1 {
			return StopTrying(fmt.Sprintf("expected one replaced pod, got %d", len(replaced)))
		}
		if len(replaced) == 0 {
			return fmt.Errorf("no pod has been replaced yet")
		}
		pod := replaced[0]
		if pod.Labels[appsv1.StatefulSetRevisionLabel] != sts.Status.UpdateRevision {
			return fmt.Errorf("replacement %s is at revision %q, want %q", pod.Name, pod.Labels[appsv1.StatefulSetRevisionLabel], sts.Status.UpdateRevision)
		}
		if pod.Status.Phase != corev1.PodPending || pod.Spec.NodeName != "" || !isPodUnschedulable(pod) {
			return fmt.Errorf("replacement %s is not pending as unschedulable (phase %q, node %q)", pod.Name, pod.Status.Phase, pod.Spec.NodeName)
		}
		replacement = pod
		return nil
	}, timeout, 2*time.Second).Should(Succeed())
	logger.Info("replacement pod is pending", "pod", replacement.Name)
	return replacement
}

// isPodUnschedulable reports whether the scheduler has marked the pod Unschedulable.
func isPodUnschedulable(pod *corev1.Pod) bool {
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodScheduled {
			return cond.Status == corev1.ConditionFalse && cond.Reason == corev1.PodReasonUnschedulable
		}
	}
	return false
}

// isEtcdContainerReady reports whether the pod's etcd container is Ready, which is what the OnDelete controller gates on.
func isEtcdContainerReady(pod *corev1.Pod) bool {
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.Name == common.ContainerNameEtcd {
			return cs.Ready
		}
	}
	return false
}

// expectPodsUntouched asserts for the whole window that the named recorded pods keep their UID, are not terminating
// and have a Ready etcd container.
func expectPodsUntouched(g *WithT, env *testenv.TestEnvironment, logger logr.Logger, etcd *druidv1alpha1.Etcd, snapshot stsPodSnapshot, podNames []string, window time.Duration) {
	logger.Info("asserting that pods stay untouched", "pods", podNames, "window", window)
	g.Consistently(func() error {
		for _, name := range podNames {
			pod := &corev1.Pod{}
			if err := env.Client().Get(env.Context(), types.NamespacedName{Name: name, Namespace: etcd.Namespace}, pod); err != nil {
				return err
			}
			if pod.UID != snapshot.podUIDs[name] {
				return fmt.Errorf("pod %s was replaced", name)
			}
			if pod.DeletionTimestamp != nil {
				return fmt.Errorf("pod %s is terminating", name)
			}
			if !isEtcdContainerReady(pod) {
				return fmt.Errorf("etcd container of pod %s is not ready", name)
			}
		}
		return nil
	}, window, 2*time.Second).Should(Succeed())
}

// waitForEtcdContainerReady blocks until the etcd container of this exact pod instance is Ready.
func waitForEtcdContainerReady(g *WithT, env *testenv.TestEnvironment, logger logr.Logger, pod *corev1.Pod, timeout time.Duration) {
	logger.Info("waiting for etcd container to become ready", "pod", pod.Name)
	g.Eventually(func() error {
		current := &corev1.Pod{}
		if err := env.Client().Get(env.Context(), client.ObjectKeyFromObject(pod), current); err != nil {
			return err
		}
		if current.UID != pod.UID {
			return StopTrying(fmt.Sprintf("pod %s was replaced while waiting for it to become ready", pod.Name))
		}
		if !isEtcdContainerReady(current) {
			return fmt.Errorf("etcd container of pod %s is not ready", pod.Name)
		}
		return nil
	}, timeout, 2*time.Second).Should(Succeed())
	logger.Info("etcd container is ready", "pod", pod.Name)
}

// waitForAnyPodReplaced blocks until at least one of the named recorded pods has been recreated at the current update revision.
func waitForAnyPodReplaced(g *WithT, env *testenv.TestEnvironment, logger logr.Logger, etcd *druidv1alpha1.Etcd, snapshot stsPodSnapshot, podNames []string, timeout time.Duration) {
	logger.Info("waiting for one of the pods to be replaced", "pods", podNames)
	g.Eventually(func() error {
		sts, pods, err := listStsPods(env, etcd)
		if err != nil {
			return err
		}
		for i := range pods {
			pod := &pods[i]
			if slices.Contains(podNames, pod.Name) && pod.UID != snapshot.podUIDs[pod.Name] &&
				pod.Labels[appsv1.StatefulSetRevisionLabel] == sts.Status.UpdateRevision {
				logger.Info("pod was replaced", "pod", pod.Name)
				return nil
			}
		}
		return fmt.Errorf("none of %v has been recreated at revision %s yet", podNames, sts.Status.UpdateRevision)
	}, timeout, 2*time.Second).Should(Succeed())
}

// expectAllPodsReplaced asserts that every recorded pod was recreated at the new update revision with a Ready etcd container.
func expectAllPodsReplaced(g *WithT, env *testenv.TestEnvironment, etcd *druidv1alpha1.Etcd, snapshot stsPodSnapshot) {
	sts, pods, err := listStsPods(env, etcd)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(sts.Status.UpdateRevision).ToNot(Equal(snapshot.updateRevision))
	g.Expect(pods).To(HaveLen(len(snapshot.podUIDs)))
	for i := range pods {
		pod := &pods[i]
		g.Expect(snapshot.podUIDs).To(HaveKey(pod.Name))
		g.Expect(pod.UID).ToNot(Equal(snapshot.podUIDs[pod.Name]), "pod %s was not recreated", pod.Name)
		g.Expect(pod.Labels).To(HaveKeyWithValue(appsv1.StatefulSetRevisionLabel, sts.Status.UpdateRevision), "pod %s is not at the update revision", pod.Name)
		g.Expect(isEtcdContainerReady(pod)).To(BeTrue(), "etcd container of pod %s is not ready", pod.Name)
	}
}

// clusterChanges tracks the cluster-wide changes made by a serial test, so that undo can revert them even if the test fails.
type clusterChanges struct {
	env           *testenv.TestEnvironment
	logger        logr.Logger
	cordonedNodes []string
	druidReplicas *int32
}

func newClusterChanges(env *testenv.TestEnvironment, logger logr.Logger) *clusterChanges {
	return &clusterChanges{env: env, logger: logger}
}

// undo uncordons the nodes before restoring etcd-druid, because its pod cannot be scheduled on a cordoned node.
func (c *clusterChanges) undo(g *WithT) {
	c.uncordonNodes(g)
	c.restoreDruid(g)
}

// cordonNodes marks every schedulable node unschedulable.
func (c *clusterChanges) cordonNodes(g *WithT) {
	nodes := &corev1.NodeList{}
	g.Expect(c.env.Client().List(c.env.Context(), nodes)).To(Succeed())
	for i := range nodes.Items {
		node := &nodes.Items[i]
		if node.Spec.Unschedulable {
			continue
		}
		g.Expect(setNodeUnschedulable(c.env, node, true)).To(Succeed())
		c.cordonedNodes = append(c.cordonedNodes, node.Name)
	}
	g.Expect(c.cordonedNodes).ToNot(BeEmpty(), "found no schedulable node to cordon")
	c.logger.Info("cordoned nodes", "nodes", c.cordonedNodes)
}

// uncordonNodes reverts cordonNodes.
func (c *clusterChanges) uncordonNodes(g *WithT) {
	if len(c.cordonedNodes) == 0 {
		return
	}
	for _, name := range c.cordonedNodes {
		node := &corev1.Node{}
		g.Expect(c.env.Client().Get(c.env.Context(), types.NamespacedName{Name: name}, node)).To(Succeed())
		g.Expect(setNodeUnschedulable(c.env, node, false)).To(Succeed())
	}
	c.logger.Info("uncordoned nodes", "nodes", c.cordonedNodes)
	c.cordonedNodes = nil
}

func setNodeUnschedulable(env *testenv.TestEnvironment, node *corev1.Node, unschedulable bool) error {
	patch := client.MergeFrom(node.DeepCopy())
	node.Spec.Unschedulable = unschedulable
	return env.Client().Patch(env.Context(), node, patch)
}

// scaleDruidToZero scales the etcd-druid Deployment to zero and remembers its replicas for restoreDruid.
func (c *clusterChanges) scaleDruidToZero(g *WithT) {
	deploy, err := getDruidDeployment(c.env)
	g.Expect(err).ToNot(HaveOccurred())
	if c.druidReplicas == nil {
		c.druidReplicas = ptr.To(ptr.Deref(deploy.Spec.Replicas, 1))
	}
	c.logger.Info("scaling etcd-druid to zero", "replicas", *c.druidReplicas)
	g.Expect(scaleDeployment(c.env, deploy, 0)).To(Succeed())
}

// restoreDruid scales etcd-druid back to the replicas recorded by scaleDruidToZero and waits until they are available.
func (c *clusterChanges) restoreDruid(g *WithT) {
	if c.druidReplicas == nil {
		return
	}
	replicas := *c.druidReplicas
	c.logger.Info("restoring etcd-druid", "replicas", replicas)
	deploy, err := getDruidDeployment(c.env)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(scaleDeployment(c.env, deploy, replicas)).To(Succeed())
	g.Eventually(func() error {
		current, err := getDruidDeployment(c.env)
		if err != nil {
			return err
		}
		if current.Status.ObservedGeneration < current.Generation {
			return fmt.Errorf("deployment observedGeneration %d < generation %d", current.Status.ObservedGeneration, current.Generation)
		}
		if current.Status.AvailableReplicas != replicas {
			return fmt.Errorf("deployment has %d available replicas, want %d", current.Status.AvailableReplicas, replicas)
		}
		return nil
	}, timeoutDruidDeploymentRestart, 2*time.Second).Should(Succeed())
	c.druidReplicas = nil
	c.logger.Info("etcd-druid is available again")
}

func getDruidDeployment(env *testenv.TestEnvironment) (*appsv1.Deployment, error) {
	deploy := &appsv1.Deployment{}
	err := env.Client().Get(env.Context(), types.NamespacedName{Name: druidDeploymentName, Namespace: druidDeployNamespace}, deploy)
	return deploy, err
}

func scaleDeployment(env *testenv.TestEnvironment, deploy *appsv1.Deployment, replicas int32) error {
	patch := client.MergeFrom(deploy.DeepCopy())
	deploy.Spec.Replicas = ptr.To(replicas)
	return env.Client().Patch(env.Context(), deploy, patch)
}

// listDruidPods lists the etcd-druid pods, including terminating ones, through the Deployment's selector.
func listDruidPods(env *testenv.TestEnvironment) ([]corev1.Pod, error) {
	deploy, err := getDruidDeployment(env)
	if err != nil {
		return nil, err
	}
	pods := &corev1.PodList{}
	if err := env.Client().List(env.Context(), pods, client.InNamespace(druidDeployNamespace), client.MatchingLabels(deploy.Spec.Selector.MatchLabels)); err != nil {
		return nil, err
	}
	return pods.Items, nil
}

// waitForDruidPodsGone blocks until etcd-druid has no pods at all, since a terminating pod may still be reconciling.
func waitForDruidPodsGone(g *WithT, env *testenv.TestEnvironment, logger logr.Logger, timeout time.Duration) {
	logger.Info("waiting for etcd-druid to have no pods")
	g.Eventually(func() (int, error) {
		pods, err := listDruidPods(env)
		return len(pods), err
	}, timeout, 2*time.Second).Should(BeZero())
	logger.Info("etcd-druid has no pods")
}

// expectDruidStopped asserts that etcd-druid still has no pods.
func expectDruidStopped(g *WithT, env *testenv.TestEnvironment) {
	pods, err := listDruidPods(env)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(pods).To(BeEmpty(), "etcd-druid was expected to stay stopped")
}

// podEventRecorder records every pod state delivered by a watch, in the resourceVersion order the API server sends them.
type podEventRecorder struct {
	watcher watch.Interface
	stopped atomic.Bool
	done    chan struct{}
	pods    []corev1.Pod
	err     error
}

// startPodEventRecorder watches the Etcd's pods from the resourceVersion of a fresh List, so that no later event is missed.
func startPodEventRecorder(g *WithT, env *testenv.TestEnvironment, etcd *druidv1alpha1.Etcd) *podEventRecorder {
	sts, _, err := listStsPods(env, etcd)
	g.Expect(err).ToNot(HaveOccurred())
	watchClient := newWatchClient(g)
	listOpts := []client.ListOption{client.InNamespace(etcd.Namespace), client.MatchingLabels(sts.Spec.Selector.MatchLabels)}
	pods := &corev1.PodList{}
	g.Expect(watchClient.List(env.Context(), pods, listOpts...)).To(Succeed())
	watcher, err := watchClient.Watch(env.Context(), &corev1.PodList{}, append(listOpts, &client.ListOptions{Raw: &metav1.ListOptions{ResourceVersion: pods.ResourceVersion}})...)
	g.Expect(err).ToNot(HaveOccurred())

	r := &podEventRecorder{watcher: watcher, done: make(chan struct{})}
	go r.record()
	return r
}

// newWatchClient builds a watch-capable client from the e2e kubeconfig, since the test environment's client cannot watch.
func newWatchClient(g *WithT) client.WithWatch {
	kubeconfigPath, err := testutils.GetEnvOrError(e2eutils.EnvKubeconfigPath)
	g.Expect(err).ToNot(HaveOccurred())
	restConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	g.Expect(err).ToNot(HaveOccurred())
	watchClient, err := client.NewWithWatch(restConfig, client.Options{})
	g.Expect(err).ToNot(HaveOccurred())
	return watchClient
}

func (r *podEventRecorder) record() {
	defer close(r.done)
	for event := range r.watcher.ResultChan() {
		pod, ok := event.Object.(*corev1.Pod)
		if !ok {
			// A watch.Error event carries a *metav1.Status instead of a pod.
			r.err = fmt.Errorf("pod watch failed: %v", event.Object)
			return
		}
		r.pods = append(r.pods, *pod)
	}
	if !r.stopped.Load() {
		r.err = errors.New("pod watch closed before it was stopped")
	}
}

// stop ends the watch; it is safe to call more than once.
func (r *podEventRecorder) stop() {
	r.stopped.Store(true)
	r.watcher.Stop()
	<-r.done
}

// recordedPods stops the watch and returns the recorded pod states, failing if the watch ended early.
func (r *podEventRecorder) recordedPods(g *WithT) []corev1.Pod {
	r.stop()
	g.Expect(r.err).ToNot(HaveOccurred(), "pod events may be missing")
	return r.pods
}

// expectReplacementReadyBeforeNextDeletion asserts from the recorded pod events that the replacement's etcd container
// was Ready before any other recorded pod started terminating.
func expectReplacementReadyBeforeNextDeletion(g *WithT, events []corev1.Pod, snapshot stsPodSnapshot, replacement *corev1.Pod) {
	var otherUIDs []types.UID
	for _, name := range snapshot.podNamesExcept(replacement.Name) {
		otherUIDs = append(otherUIDs, snapshot.podUIDs[name])
	}
	readyAt := slices.IndexFunc(events, func(pod corev1.Pod) bool {
		return pod.UID == replacement.UID && isEtcdContainerReady(&pod)
	})
	terminatingAt := slices.IndexFunc(events, func(pod corev1.Pod) bool {
		return slices.Contains(otherUIDs, pod.UID) && pod.DeletionTimestamp != nil
	})
	g.Expect(readyAt).To(BeNumerically(">=", 0), "no pod event shows replacement %s with a Ready etcd container", replacement.Name)
	g.Expect(terminatingAt).To(BeNumerically(">=", 0), "no pod event shows another original pod terminating")
	g.Expect(readyAt).To(BeNumerically("<", terminatingAt), "another original pod started terminating before replacement %s was Ready", replacement.Name)
}
