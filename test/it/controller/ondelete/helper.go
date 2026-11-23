// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package ondelete

import (
	"context"
	"fmt"
	"maps"
	"strconv"
	"testing"
	"time"

	druidconfigv1alpha1 "github.com/gardener/etcd-druid/api/config/v1alpha1"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	etcdclientfake "github.com/gardener/etcd-druid/internal/client/etcd/fake"
	"github.com/gardener/etcd-druid/internal/common"
	"github.com/gardener/etcd-druid/internal/controller/ondelete"
	etcdmember "github.com/gardener/etcd-druid/internal/etcd"
	"github.com/gardener/etcd-druid/test/it/setup"
	testutils "github.com/gardener/etcd-druid/test/utils"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	. "github.com/onsi/gomega"
)

type reconcilerTestEnv struct {
	itTestEnv  setup.DruidTestEnvironment
	reconciler *ondelete.Reconciler
	etcdClient *etcdclientfake.Client
}

func initializeOnDeleteReconcilerTestEnv(t *testing.T, itTestEnv setup.DruidTestEnvironment) reconcilerTestEnv {
	g := NewWithT(t)
	var reconciler *ondelete.Reconciler
	// The reconciler is given a fake factory. It defaults to a healthy 3-member cluster (member names match the
	// pods the tests create) so the quorum gate is transparent and the end-to-end
	// wiring (watches, predicates, deletes) is what gets exercised. Tests that need
	// another cluster state call etcdClient.SetMembers before triggering a reconcile.
	etcdClient := etcdclientfake.NewClient(testutils.TestEtcdName, 3)
	g.Expect(itTestEnv.CreateManager(testutils.NewTestClientBuilder())).To(Succeed())
	itTestEnv.RegisterReconciler(func(mgr manager.Manager) {
		reconciler = ondelete.NewReconciler(mgr, druidconfigv1alpha1.OnDeleteControllerConfiguration{
			ConcurrentSyncs: ptr.To(3),
		}, &etcdclientfake.Factory{Client: etcdClient})
		g.Expect(reconciler.RegisterWithManager(mgr)).To(Succeed())
	})
	g.Expect(itTestEnv.StartManager()).To(Succeed())
	t.Log("successfully registered ondelete reconciler and started manager")
	return reconcilerTestEnv{itTestEnv: itTestEnv, reconciler: reconciler, etcdClient: etcdClient}
}

// createEtcdAndStatefulSet creates a minimal Etcd + StatefulSet pair for a test.
func createEtcdAndStatefulSet(ctx context.Context, t *testing.T, cl client.Client, namespace string, replicas int32) (*druidv1alpha1.Etcd, *appsv1.StatefulSet) {
	g := NewWithT(t)
	etcd := testutils.EtcdBuilderWithDefaults(testutils.TestEtcdName, namespace).WithReplicas(replicas).Build()
	g.Expect(cl.Create(ctx, etcd)).To(Succeed())

	labels := map[string]string{
		druidv1alpha1.LabelManagedByKey: druidv1alpha1.LabelManagedByValue,
		druidv1alpha1.LabelPartOfKey:    etcd.Name,
	}
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      etcd.Name,
			Namespace: namespace,
			Labels:    labels,
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion:         druidv1alpha1.SchemeGroupVersion.String(),
				Kind:               "Etcd",
				Name:               etcd.Name,
				UID:                etcd.UID,
				Controller:         ptr.To(true),
				BlockOwnerDeletion: ptr.To(true),
			}},
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:            ptr.To(replicas),
			Selector:            &metav1.LabelSelector{MatchLabels: labels},
			UpdateStrategy:      appsv1.StatefulSetUpdateStrategy{Type: appsv1.OnDeleteStatefulSetStrategyType},
			PodManagementPolicy: appsv1.ParallelPodManagement,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: common.ContainerNameEtcd, Image: "etcd-wrapper:latest"}},
				},
			},
			ServiceName: etcd.Name + "-peer",
		},
	}
	g.Expect(cl.Create(ctx, sts)).To(Succeed())
	return etcd, sts
}

// setStsUpdateRevision patches sts.Status.UpdateRevision to the given value.
func setStsUpdateRevision(ctx context.Context, t *testing.T, cl client.Client, sts *appsv1.StatefulSet, updateRevision string) {
	g := NewWithT(t)
	g.Eventually(func() error {
		current := &appsv1.StatefulSet{}
		if err := cl.Get(ctx, client.ObjectKeyFromObject(sts), current); err != nil {
			return err
		}
		patch := current.DeepCopy()
		patch.Status.ObservedGeneration = current.Generation
		patch.Status.UpdateRevision = updateRevision
		patch.Status.Replicas = *current.Spec.Replicas
		return cl.Status().Patch(ctx, patch, client.MergeFrom(current))
	}, 5*time.Second, 200*time.Millisecond).Should(Succeed())
}

// patchStsUpdateStrategy patches sts.Spec.UpdateStrategy.Type
func patchStsUpdateStrategy(ctx context.Context, t *testing.T, cl client.Client, sts *appsv1.StatefulSet, strategy appsv1.StatefulSetUpdateStrategyType) {
	g := NewWithT(t)
	g.Eventually(func() error {
		current := &appsv1.StatefulSet{}
		if err := cl.Get(ctx, client.ObjectKeyFromObject(sts), current); err != nil {
			return err
		}
		patch := current.DeepCopy()
		patch.Spec.UpdateStrategy = appsv1.StatefulSetUpdateStrategy{Type: strategy}
		return cl.Patch(ctx, patch, client.MergeFrom(current))
	}, 5*time.Second, 200*time.Millisecond).Should(Succeed())
}

// patchStsReadyReplicas bumps sts.Status.ReadyReplicas
func patchStsReadyReplicas(ctx context.Context, t *testing.T, cl client.Client, sts *appsv1.StatefulSet, ready int32) {
	g := NewWithT(t)
	g.Eventually(func() error {
		current := &appsv1.StatefulSet{}
		if err := cl.Get(ctx, client.ObjectKeyFromObject(sts), current); err != nil {
			return err
		}
		patch := current.DeepCopy()
		patch.Status.ReadyReplicas = ready
		return cl.Status().Patch(ctx, patch, client.MergeFrom(current))
	}, 5*time.Second, 200*time.Millisecond).Should(Succeed())
}

// createStsPods creates pods 0..count-1 of sts, all at the given revision and readiness.
func createStsPods(ctx context.Context, t *testing.T, cl client.Client, sts *appsv1.StatefulSet, count int, revision string, ready bool) []*corev1.Pod {
	pods := make([]*corev1.Pod, 0, count)
	for ordinal := range count {
		pods = append(pods, createStsPod(ctx, t, cl, sts, ordinal, revision, ready))
	}
	return pods
}

// createStsPodsWithRevisionAndReady creates one pod per entry in podSpecs, using
// the entry's index as the pod ordinal.
func createStsPodsWithRevisionAndReady(ctx context.Context, t *testing.T, cl client.Client, sts *appsv1.StatefulSet, podSpecs []podSpec) []*corev1.Pod {
	pods := make([]*corev1.Pod, 0, len(podSpecs))
	for ordinal, spec := range podSpecs {
		pods = append(pods, createStsPod(ctx, t, cl, sts, ordinal, spec.revision, spec.ready))
	}
	return pods
}

// createStsPod creates the pod with the given ordinal, owned by sts, at the given revision and readiness.
// Tests create pods before setStsUpdateRevision: until its status is patched a pod's readiness is
// unknown, which the controller may select for deletion.
func createStsPod(ctx context.Context, t *testing.T, cl client.Client, sts *appsv1.StatefulSet, ordinal int, revision string, ready bool) *corev1.Pod {
	g := NewWithT(t)
	podName := fmt.Sprintf("%s-%d", sts.Name, ordinal)
	labels := maps.Clone(sts.Spec.Selector.MatchLabels)
	labels[appsv1.StatefulSetPodNameLabel] = podName
	labels[appsv1.PodIndexLabel] = strconv.Itoa(ordinal)
	labels[appsv1.StatefulSetRevisionLabel] = revision
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            podName,
			Namespace:       sts.Namespace,
			Labels:          labels,
			OwnerReferences: []metav1.OwnerReference{stsOwnerReference(sts)},
		},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: common.ContainerNameEtcd, Image: "etcd-wrapper:latest"}}},
	}
	g.Expect(cl.Create(ctx, pod)).To(Succeed())
	setPodReady(ctx, t, cl, pod, ready)
	return pod
}

// stsOwnerReference builds a controller-owner reference to the given
// StatefulSet. The TypeMeta on the returned StatefulSet from cl.Create is
// stripped by the API server, so APIVersion/Kind are hardcoded here.
func stsOwnerReference(sts *appsv1.StatefulSet) metav1.OwnerReference {
	return metav1.OwnerReference{
		APIVersion:         appsv1.SchemeGroupVersion.String(),
		Kind:               "StatefulSet",
		Name:               sts.Name,
		UID:                sts.UID,
		Controller:         ptr.To(true),
		BlockOwnerDeletion: ptr.To(true),
	}
}

// setPodReady patches the readiness of the pod's etcd container, which is the
// readiness signal the controller reads.
func setPodReady(ctx context.Context, t *testing.T, cl client.Client, pod *corev1.Pod, ready bool) {
	g := NewWithT(t)
	g.Eventually(func() error {
		current := &corev1.Pod{}
		if err := cl.Get(ctx, client.ObjectKeyFromObject(pod), current); err != nil {
			return err
		}
		patch := current.DeepCopy()
		patch.Status.ContainerStatuses = []corev1.ContainerStatus{{
			Name:  common.ContainerNameEtcd,
			Ready: ready,
			State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
		}}
		return cl.Status().Patch(ctx, patch, client.MergeFrom(current))
	}, 5*time.Second, 200*time.Millisecond).Should(Succeed())
}

// liveMembers builds a member list of `replicas` voters named to match the test
// StatefulSet's pods (<TestEtcdName>-<ordinal>), with ordinal 0 as the leader and
// the health of each member taken from healthFor. It is used to drive the fake
// etcd client so the reconciler's quorum gate sees a specific cluster state.
func liveMembers(replicas int, healthFor func(int) etcdmember.MemberHealth) []etcdmember.Member {
	return liveMembersWithLeader(replicas, 0, healthFor)
}

// liveMembersWithLeader places the leader at leaderIndex ordinal.
func liveMembersWithLeader(replicas, leaderIndex int, healthFor func(int) etcdmember.MemberHealth) []etcdmember.Member {
	members := make([]etcdmember.Member, 0, replicas)
	for i := 0; i < replicas; i++ {
		role := etcdmember.MemberRoleMember
		if i == leaderIndex {
			role = etcdmember.MemberRoleLeader
		}
		members = append(members, etcdmember.Member{
			ID:     uint64(i) + 1, // #nosec G115 -- small loop index
			Name:   fmt.Sprintf("%s-%d", testutils.TestEtcdName, i),
			Role:   role,
			Health: healthFor(i),
		})
	}
	return members
}

// healthyExcept returns a health function that reports every member Healthy
// except those at the given indices, which report Unhealthy.
func healthyExcept(unhealthy ...int) func(int) etcdmember.MemberHealth {
	bad := make(map[int]struct{}, len(unhealthy))
	for _, i := range unhealthy {
		bad[i] = struct{}{}
	}
	return func(i int) etcdmember.MemberHealth {
		if _, ok := bad[i]; ok {
			return etcdmember.MemberHealthUnhealthy
		}
		return etcdmember.MemberHealthHealthy
	}
}

// consistentlyPodNotDeleted asserts that within the window the pod is neither
// terminating nor gone. Used to prove the reconciler is holding.
func consistentlyPodNotDeleted(ctx context.Context, t *testing.T, cl client.Client, key types.NamespacedName, window, poll time.Duration) {
	g := NewWithT(t)
	g.Consistently(func() bool {
		pod := &corev1.Pod{}
		err := cl.Get(ctx, key, pod)
		if err != nil {
			return false
		}
		return pod.DeletionTimestamp == nil
	}, window, poll).Should(BeTrue())
}

// deletedPodOrdinals returns the ordinals of the pods of sts that are gone or terminating.
func deletedPodOrdinals(ctx context.Context, cl client.Client, sts *appsv1.StatefulSet) []int {
	var ordinals []int
	for ordinal := range int(*sts.Spec.Replicas) {
		pod := &corev1.Pod{}
		err := cl.Get(ctx, stsPodKey(sts, ordinal), pod)
		if apierrorsNotFound(err) || (err == nil && pod.DeletionTimestamp != nil) {
			ordinals = append(ordinals, ordinal)
		}
	}
	return ordinals
}

// unavailablePodNames returns the names of the pods of sts that are gone,
// terminating, or whose etcd container is not Ready.
func unavailablePodNames(ctx context.Context, cl client.Client, sts *appsv1.StatefulSet) []string {
	var names []string
	for ordinal := range int(*sts.Spec.Replicas) {
		key := stsPodKey(sts, ordinal)
		pod := &corev1.Pod{}
		if err := cl.Get(ctx, key, pod); err != nil || pod.DeletionTimestamp != nil || !isEtcdContainerReady(pod) {
			names = append(names, key.Name)
		}
	}
	return names
}

func isEtcdContainerReady(pod *corev1.Pod) bool {
	for _, status := range pod.Status.ContainerStatuses {
		if status.Name == common.ContainerNameEtcd {
			return status.Ready
		}
	}
	return false
}

func stsPodKey(sts *appsv1.StatefulSet, ordinal int) types.NamespacedName {
	return types.NamespacedName{Name: fmt.Sprintf("%s-%d", sts.Name, ordinal), Namespace: sts.Namespace}
}

func apierrorsNotFound(err error) bool {
	return err != nil && client.IgnoreNotFound(err) == nil
}

// podSpec is a tiny record driving createStsPodsWithRevisionAndReady.
type podSpec struct {
	revision string
	ready    bool
}
