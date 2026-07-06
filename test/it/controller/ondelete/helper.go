// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package ondelete

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	druidconfigv1alpha1 "github.com/gardener/etcd-druid/api/config/v1alpha1"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"
	"github.com/gardener/etcd-druid/internal/controller/ondelete"
	"github.com/gardener/etcd-druid/test/it/setup"
	testutils "github.com/gardener/etcd-druid/test/utils"

	appsv1 "k8s.io/api/apps/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	. "github.com/onsi/gomega"
)

// reconcilerTestEnv bundles the test environment with the registered reconciler
// so scenarios can share the harness while isolating on per-test namespaces.
type reconcilerTestEnv struct {
	itTestEnv  setup.DruidTestEnvironment
	reconciler *ondelete.Reconciler
}

func initializeOnDeleteReconcilerTestEnv(t *testing.T, itTestEnv setup.DruidTestEnvironment) reconcilerTestEnv {
	g := NewWithT(t)
	var reconciler *ondelete.Reconciler
	g.Expect(itTestEnv.CreateManager(testutils.NewTestClientBuilder())).To(Succeed())
	itTestEnv.RegisterReconciler(func(mgr manager.Manager) {
		reconciler = ondelete.NewReconciler(mgr, druidconfigv1alpha1.OnDeleteControllerConfiguration{
			ConcurrentSyncs: ptr.To(3),
		})
		g.Expect(reconciler.RegisterWithManager(mgr)).To(Succeed())
	})
	g.Expect(itTestEnv.StartManager()).To(Succeed())
	t.Log("successfully registered ondelete reconciler and started manager")
	return reconcilerTestEnv{itTestEnv: itTestEnv, reconciler: reconciler}
}

// createEtcdAndStatefulSet creates a minimal Etcd + StatefulSet pair for a test.
// The StatefulSet is created with the OnDelete update strategy and a fabricated
// updateRevision on its status; pods are created separately via createStsPods.
// Envtest does not run KCM, so the StatefulSet controller does not create pods
// or maintain sts.Status; the test must set both explicitly.
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
// The status subresource is not populated by envtest.
func setStsUpdateRevision(ctx context.Context, t *testing.T, cl client.Client, sts *appsv1.StatefulSet, updateRevision string) {
	g := NewWithT(t)
	// The cached client's Get may lag behind our own Create; wait for the
	// informer to observe the STS before patching status.
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

// createStsPods creates `count` pods labelled to match the StatefulSet's
// selector and carrying the given controller-revision-hash. Each pod is set to
// Ready via a status patch, so isParticipating(pod) returns true.
func createStsPods(ctx context.Context, t *testing.T, cl client.Client, sts *appsv1.StatefulSet, count int32, revision string, ready bool) []*corev1.Pod {
	g := NewWithT(t)
	pods := make([]*corev1.Pod, 0, count)
	for i := int32(0); i < count; i++ {
		podName := fmt.Sprintf("%s-%d", sts.Name, i)
		labels := map[string]string{}
		for k, v := range sts.Spec.Selector.MatchLabels {
			labels[k] = v
		}
		labels[appsv1.StatefulSetPodNameLabel] = podName
		labels[appsv1.PodIndexLabel] = strconv.Itoa(int(i))
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
		pods = append(pods, pod)
	}
	return pods
}

// createStsPodsWithRevisionAndReady creates one pod per entry in podSpecs,
// keyed by ordinal, each with its own revision and Ready state. Used by tests
// that need heterogeneous pod state (e.g. p0 at new revision but not ready,
// p1/p2 at old revision and ready).
func createStsPodsWithRevisionAndReady(ctx context.Context, t *testing.T, cl client.Client, sts *appsv1.StatefulSet, baseLabels map[string]string, podSpecs map[int]podSpec) []*corev1.Pod {
	g := NewWithT(t)
	pods := make([]*corev1.Pod, 0, len(podSpecs))
	for ordinal, spec := range podSpecs {
		podName := fmt.Sprintf("%s-%d", sts.Name, ordinal)
		labels := map[string]string{}
		for k, v := range baseLabels {
			labels[k] = v
		}
		labels[appsv1.StatefulSetPodNameLabel] = podName
		labels[appsv1.PodIndexLabel] = strconv.Itoa(ordinal)
		labels[appsv1.StatefulSetRevisionLabel] = spec.revision
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
		setPodReady(ctx, t, cl, pod, spec.ready)
		pods = append(pods, pod)
	}
	return pods
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

// setPodReady patches the pod status to reflect the ready state of its etcd
// container. The reconciler's isParticipating() reads containerStatuses[etcd].Ready.
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

// createMemberLeases creates one lease per replica with the given roles.
// roleFor(i) is the role assigned to pod-i's lease.
func createMemberLeases(ctx context.Context, t *testing.T, cl client.Client, namespace string, sts *appsv1.StatefulSet, roleFor func(int) druidv1alpha1.EtcdRole) {
	g := NewWithT(t)
	replicas := int(*sts.Spec.Replicas)
	for i := 0; i < replicas; i++ {
		leaseName := fmt.Sprintf("%s-%d", sts.Name, i)
		lease := &coordinationv1.Lease{
			ObjectMeta: metav1.ObjectMeta{Name: leaseName, Namespace: namespace},
			Spec: coordinationv1.LeaseSpec{
				HolderIdentity: ptr.To(fmt.Sprintf("id-%d:cid:%s", i, roleFor(i))),
			},
		}
		g.Expect(cl.Create(ctx, lease)).To(Succeed())
	}
}

// followerFor returns EtcdRoleMember for every replica index — a helper for the
// common test scenario where any pod chosen for deletion should be a follower.
func followerFor(_ int) druidv1alpha1.EtcdRole { return druidv1alpha1.EtcdRoleMember }

// eventuallyPodDeleted waits until the pod either has a DeletionTimestamp set
// or is fully absent from the API server.
func eventuallyPodDeleted(ctx context.Context, t *testing.T, cl client.Client, key types.NamespacedName, timeout, poll time.Duration) {
	g := NewWithT(t)
	g.Eventually(func() bool {
		pod := &corev1.Pod{}
		err := cl.Get(ctx, key, pod)
		if apierrorsNotFound(err) {
			return true
		}
		if err != nil {
			return false
		}
		return pod.DeletionTimestamp != nil
	}, timeout, poll).Should(BeTrue())
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

// apiErrorsNotFound is a small adapter so eventuallyPodDeleted stays legible.
func apierrorsNotFound(err error) bool {
	return err != nil && client.IgnoreNotFound(err) == nil
}

// podSpec is a tiny record driving createStsPodsWithRevisionAndReady.
type podSpec struct {
	revision string
	ready    bool
}
