// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"fmt"
	"strings"
	"testing"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/test/e2e/testenv"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
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
)

// skipIfOnDeleteGateDisabled skips the test unless the deployed operator has the QuorumAwareUpdates feature gate enabled.
func skipIfOnDeleteGateDisabled(t *testing.T, env *testenv.TestEnvironment) {
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
			return
		}
	}
	t.Skipf("skipping: %s feature gate is not enabled; run 'make ci-e2e-kind-ondelete' to enable", quorumAwareUpdatesGateName)
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
		sts := &appsv1.StatefulSet{}
		if err := env.Client().Get(env.Context(), types.NamespacedName{Name: etcd.Name, Namespace: etcd.Namespace}, sts); err != nil {
			return err
		}
		if sts.Status.UpdateRevision == "" {
			return fmt.Errorf("statefulSet has no updateRevision yet")
		}
		if sts.Spec.Selector == nil || sts.Spec.Replicas == nil {
			return fmt.Errorf("statefulSet has nil selector or replicas")
		}
		pods := &corev1.PodList{}
		if err := env.Client().List(env.Context(), pods, client.InNamespace(etcd.Namespace), client.MatchingLabels(sts.Spec.Selector.MatchLabels)); err != nil {
			return err
		}
		if len(pods.Items) != int(*sts.Spec.Replicas) {
			return fmt.Errorf("expected %d pods, got %d", *sts.Spec.Replicas, len(pods.Items))
		}
		for i := range pods.Items {
			p := &pods.Items[i]
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
		fetched := &druidv1alpha1.Etcd{}
		if err := env.Client().Get(env.Context(), types.NamespacedName{Name: etcd.Name, Namespace: etcd.Namespace}, fetched); err != nil {
			return err
		}
		if fetched.Status.Ready == nil || !*fetched.Status.Ready {
			return fmt.Errorf("etcd.status.ready is not true")
		}
		for _, cond := range fetched.Status.Conditions {
			if cond.Type == druidv1alpha1.ConditionTypeAllMembersUpdated {
				if cond.Status != druidv1alpha1.ConditionTrue {
					return fmt.Errorf("AllMembersUpdated condition is %s, want True", cond.Status)
				}
				return nil
			}
		}
		return fmt.Errorf("AllMembersUpdated condition not found")
	}, timeout, 5*time.Second).Should(Succeed())
	logger.Info("post-rollout conditions verified: AllMembersUpdated=True, etcd.status.ready=true")
}

// skipIfOnDeleteGateEnabled skips the test if the QuorumAwareUpdates gate is enabled.
// Used for the gate-off scenario that verifies RollingUpdate behaviour is preserved.
func skipIfOnDeleteGateEnabled(t *testing.T, env *testenv.TestEnvironment) {
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
			t.Skipf("skipping gate-off scenario: %s feature gate is enabled", quorumAwareUpdatesGateName)
		}
	}
}

// waitForRolloutInProgress blocks until the pod rollout has visibly started.
// It covers the common observable mid-rollout states:
//   - a pod is terminating
//   - pod count is below desired i.e a pod was deleted and not yet recreated
//   - a partial revision mix of versions exists.
func waitForRolloutInProgress(g *WithT, env *testenv.TestEnvironment, logger logr.Logger, etcd *druidv1alpha1.Etcd, timeout time.Duration) {
	logger.Info("waiting for rollout to be observably in progress")
	g.Eventually(func() bool {
		sts := &appsv1.StatefulSet{}
		if err := env.Client().Get(env.Context(), types.NamespacedName{Name: etcd.Name, Namespace: etcd.Namespace}, sts); err != nil {
			return false
		}
		if sts.Spec.Selector == nil || sts.Status.UpdateRevision == "" {
			return false
		}
		pods := &corev1.PodList{}
		if err := env.Client().List(env.Context(), pods, client.InNamespace(etcd.Namespace), client.MatchingLabels(sts.Spec.Selector.MatchLabels)); err != nil {
			return false
		}
		// Case 1: a pod is mid-termination.
		for i := range pods.Items {
			if pods.Items[i].DeletionTimestamp != nil {
				return true
			}
		}
		// Case 2: pod count is below desired (deleted but not yet recreated).
		if sts.Spec.Replicas != nil && len(pods.Items) < int(*sts.Spec.Replicas) {
			return true
		}
		// Case 3: revision mix
		updated, outdated := 0, 0
		for i := range pods.Items {
			if pods.Items[i].Labels[appsv1.StatefulSetRevisionLabel] == sts.Status.UpdateRevision {
				updated++
			} else {
				outdated++
			}
		}
		return updated > 0 && outdated > 0
	}, timeout, 2*time.Second).Should(BeTrue(), "timed out waiting for rollout to start; no mid-rollout state observed")
	logger.Info("confirmed: rollout is in progress")
}

// restartDruidOperator triggers a rollout restart of the etcd-druid deployment and waits for the new pod to be Ready.
// Used by the mid-rollout-restart scenario to check the ondelete reconciler resumes correctly on operator restart.
func restartDruidOperator(g *WithT, env *testenv.TestEnvironment, logger logr.Logger, timeout time.Duration) {
	ctx := env.Context()
	cl := env.Client()

	druidDeployment := &appsv1.Deployment{}
	g.Expect(cl.Get(ctx, types.NamespacedName{Name: druidDeploymentName, Namespace: druidDeployNamespace}, druidDeployment)).To(Succeed())

	patch := druidDeployment.DeepCopy()
	if patch.Spec.Template.Annotations == nil {
		patch.Spec.Template.Annotations = map[string]string{}
	}
	patch.Spec.Template.Annotations["ondelete-e2e/restart-at"] = time.Now().Format(time.RFC3339Nano)
	g.Expect(cl.Patch(ctx, patch, client.MergeFrom(druidDeployment))).To(Succeed())
	logger.Info("triggered operator rollout restart")

	// Wait for the deployment to reach the new generation with all replicas ready.
	g.Eventually(func() error {
		fresh := &appsv1.Deployment{}
		if err := cl.Get(ctx, types.NamespacedName{Name: druidDeploymentName, Namespace: druidDeployNamespace}, fresh); err != nil {
			return err
		}
		if fresh.Status.ObservedGeneration < fresh.Generation {
			return fmt.Errorf("deployment observedGeneration %d < generation %d", fresh.Status.ObservedGeneration, fresh.Generation)
		}
		if fresh.Status.UpdatedReplicas < *fresh.Spec.Replicas {
			return fmt.Errorf("updatedReplicas %d < desired %d", fresh.Status.UpdatedReplicas, *fresh.Spec.Replicas)
		}
		if fresh.Status.ReadyReplicas < *fresh.Spec.Replicas {
			return fmt.Errorf("readyReplicas %d < desired %d", fresh.Status.ReadyReplicas, *fresh.Spec.Replicas)
		}
		return nil
	}, timeout, 3*time.Second).Should(Succeed())
	logger.Info("operator rollout restart complete")
}
