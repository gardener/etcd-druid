// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package ondelete

import (
	"context"
	"fmt"
	"testing"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	etcdclientfake "github.com/gardener/etcd-druid/internal/client/etcd/fake"
	"github.com/gardener/etcd-druid/internal/client/kubernetes"
	ctrlutils "github.com/gardener/etcd-druid/internal/controller/utils"
	etcdmember "github.com/gardener/etcd-druid/internal/etcd"
	testutils "github.com/gardener/etcd-druid/test/utils"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/gomega"
)

const (
	testStsNamespace = "test-ns"
	testStsName      = "etcd-test"
	testRevOld       = "rev-1"
	testRevNew       = "rev-2"
)

func TestClassifyPods(t *testing.T) {
	g := NewWithT(t)
	t.Parallel()

	sts := stsFixture(testStsName, testStsNamespace, testRevNew, 3)
	current1 := makeStsPod(testStsName+"-0", testStsNamespace, testRevNew, true)
	current2 := makeStsPod(testStsName+"-1", testStsNamespace, testRevNew, true)
	outdated := makeStsPod(testStsName+"-2", testStsNamespace, testRevOld, false)
	terminatingOutdated := withDeletionTimestamp(makeStsPod(testStsName+"-3", testStsNamespace, testRevOld, false))
	terminatingCurrent := withDeletionTimestamp(makeStsPod(testStsName+"-4", testStsNamespace, testRevNew, true))

	pods := []corev1.Pod{*current1, *current2, *outdated, *terminatingOutdated, *terminatingCurrent}
	state := &podRolloutState{sts: sts, pods: pods}
	reconciler := &Reconciler{}
	result := reconciler.classifyPods(context.Background(), logr.Discard(), state)

	g.Expect(result).To(Equal(ctrlutils.ContinueReconcile()))
	g.Expect(namesOf(state.podsNotAtTargetRevision)).To(ConsistOf(outdated.Name, terminatingOutdated.Name))
	g.Expect(namesOf(state.podsAtTargetRevision)).To(ConsistOf(current1.Name, current2.Name, terminatingCurrent.Name))

	state.pods = nil
	result = reconciler.classifyPods(context.Background(), logr.Discard(), state)
	g.Expect(result).To(Equal(ctrlutils.ContinueReconcile()))
	g.Expect(state.podsNotAtTargetRevision).To(BeEmpty())
	g.Expect(state.podsAtTargetRevision).To(BeEmpty())
}

// namesOf returns the names of the given pods (test-side helper).
func namesOf(pods []corev1.Pod) []string {
	names := make([]string, 0, len(pods))
	for i := range pods {
		names = append(names, pods[i].Name)
	}
	return names
}

func TestRequeueIfPodsTerminatingOrMissing(t *testing.T) {
	tests := []struct {
		name               string
		desiredReplicas    *int32
		pods               []corev1.Pod
		expectEmpty        bool
		expectReasonSubstr string
	}{
		{
			name:            "settled pod set -> empty reason",
			desiredReplicas: ptr.To(int32(3)),
			pods: []corev1.Pod{
				*makeStsPod("p-0", testStsNamespace, testRevNew, true),
				*makeStsPod("p-1", testStsNamespace, testRevNew, true),
				*makeStsPod("p-2", testStsNamespace, testRevNew, true),
			},
			expectEmpty: true,
		},
		{
			name:            "any terminating pod -> reason names the terminating pod",
			desiredReplicas: ptr.To(int32(3)),
			pods: []corev1.Pod{
				*makeStsPod("p-0", testStsNamespace, testRevNew, true),
				*withDeletionTimestamp(makeStsPod("p-1", testStsNamespace, testRevOld, false)),
				*makeStsPod("p-2", testStsNamespace, testRevNew, true),
			},
			expectReasonSubstr: "p-1",
		},
		{
			name:            "pod count below desired -> reason mentions counts",
			desiredReplicas: ptr.To(int32(3)),
			pods: []corev1.Pod{
				*makeStsPod("p-0", testStsNamespace, testRevNew, true),
			},
			expectReasonSubstr: "below desired 3",
		},
		{
			name:            "hibernation (desired=0) with no pods -> empty reason",
			desiredReplicas: ptr.To(int32(0)),
			pods:            nil,
			expectEmpty:     true,
		},
		{
			name:            "nil spec.replicas is treated as 0 -> empty reason for empty pod set",
			desiredReplicas: nil,
			pods:            nil,
			expectEmpty:     true,
		},
		{
			name:            "terminating check runs before count check",
			desiredReplicas: ptr.To(int32(3)),
			pods: []corev1.Pod{
				*withDeletionTimestamp(makeStsPod("p-terminating", testStsNamespace, testRevOld, false)),
			},
			expectReasonSubstr: "p-terminating",
		},
	}

	t.Parallel()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g := NewWithT(t)
			sts := &appsv1.StatefulSet{Spec: appsv1.StatefulSetSpec{Replicas: tc.desiredReplicas}}
			state := &podRolloutState{sts: sts, pods: tc.pods}
			reconciler := &Reconciler{}
			result := reconciler.requeueIfPodsTerminatingOrMissing(context.Background(), logr.Discard(), state)
			if tc.expectEmpty {
				g.Expect(result).To(Equal(ctrlutils.ContinueReconcile()))
			} else {
				g.Expect(ctrlutils.ShortCircuitReconcileFlow(result)).To(BeTrue())
				g.Expect(result.GetResult().RequeueAfter).To(Equal(requeueAfterInFlight))
				g.Expect(result.GetDescription()).To(ContainSubstring(tc.expectReasonSubstr))
				g.Expect(result.HasErrors()).To(BeFalse())
			}
		})
	}
}

func TestUpdatedNonParticipatingPodNames(t *testing.T) {
	g := NewWithT(t)
	t.Parallel()

	t.Run("all participating -> empty", func(_ *testing.T) {
		pods := []corev1.Pod{
			*makeStsPod("p-0", testStsNamespace, testRevNew, true),
			*makeStsPod("p-1", testStsNamespace, testRevNew, true),
		}
		g.Expect(updatedNonParticipatingPodNames(pods)).To(BeEmpty())
	})

	t.Run("returns the single non-participating pod's name", func(_ *testing.T) {
		pods := []corev1.Pod{
			*makeStsPod("p-0", testStsNamespace, testRevNew, true),
			*makeStsPod("p-1", testStsNamespace, testRevNew, false),
			*makeStsPod("p-2", testStsNamespace, testRevNew, true),
		}
		g.Expect(updatedNonParticipatingPodNames(pods)).To(ConsistOf("p-1"))
	})

	t.Run("returns all non-participating pod names, preserving list order", func(_ *testing.T) {
		pods := []corev1.Pod{
			*makeStsPod("p-0", testStsNamespace, testRevNew, true),
			*makeStsPod("p-1", testStsNamespace, testRevNew, false),
			*makeStsPod("p-2", testStsNamespace, testRevNew, false),
			*makeStsPod("p-3", testStsNamespace, testRevNew, true),
			*makeStsPod("p-4", testStsNamespace, testRevNew, false),
		}
		g.Expect(updatedNonParticipatingPodNames(pods)).To(Equal([]string{"p-1", "p-2", "p-4"}))
	})
}

func TestDeleteSelected(t *testing.T) {
	ctx := context.Background()
	g := NewWithT(t)

	t.Run("terminating pod is not re-deleted, but a requeue is returned", func(_ *testing.T) {
		pod := withDeletionTimestamp(makeStsPod("p-0", testStsNamespace, testRevOld, false))
		cl := testutils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).WithObjects(pod).Build()
		r := &Reconciler{client: cl, logger: logr.Discard()}

		result := r.deleteSelectedPodAndWait(ctx, logr.Discard(), pod)
		g.Expect(result.HasErrors()).To(BeFalse())
		g.Expect(result.NeedsRequeue()).To(BeTrue())
		g.Expect(cl.Get(ctx, client.ObjectKeyFromObject(pod), &corev1.Pod{})).To(Succeed())
	})

	t.Run("normal delete requeues", func(_ *testing.T) {
		pod := makeStsPod("p-0", testStsNamespace, testRevOld, false)
		cl := testutils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).WithObjects(pod).Build()
		r := &Reconciler{client: cl, logger: logr.Discard()}

		result := r.deleteSelectedPodAndWait(ctx, logr.Discard(), pod)
		g.Expect(result.HasErrors()).To(BeFalse())
		g.Expect(result.NeedsRequeue()).To(BeTrue())
		// Pod should be gone from the fake client.
		g.Expect(apierrors.IsNotFound(cl.Get(ctx, client.ObjectKeyFromObject(pod), &corev1.Pod{}))).To(BeTrue())
	})

	t.Run("pod-not-found on Delete is a benign race and requeues without error", func(_ *testing.T) {
		pod := makeStsPod("p-ghost", testStsNamespace, testRevOld, false)
		cl := testutils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).Build() // pod not present
		r := &Reconciler{client: cl, logger: logr.Discard()}

		result := r.deleteSelectedPodAndWait(ctx, logr.Discard(), pod)
		g.Expect(result.HasErrors()).To(BeFalse())
		g.Expect(result.NeedsRequeue()).To(BeTrue())
	})
}

func TestDeleteNextOutdatedPod_NoCandidate(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	reconciler := &Reconciler{}
	state := &podRolloutState{etcd: &druidv1alpha1.Etcd{}}
	result := reconciler.deleteNextOutdatedPod(context.Background(), logr.Discard(), state)
	g.Expect(result).To(Equal(ctrlutils.ReconcileAfter(requeueAfterInFlight, "no pod update candidate selected")))
}

func TestReconcilePodRollout(t *testing.T) {
	ctx := context.Background()

	etcd := &druidv1alpha1.Etcd{
		ObjectMeta: metav1.ObjectMeta{Name: testStsName, Namespace: testStsNamespace},
		Spec:       druidv1alpha1.EtcdSpec{Replicas: 3},
	}
	sts := stsFixture(testStsName, testStsNamespace, testRevNew, 3)

	t.Run("does not delete or requeue when all pods are up to date", func(t *testing.T) {
		g := NewWithT(t)
		p0 := makeStsPod(testStsName+"-0", testStsNamespace, testRevNew, true)
		p1 := makeStsPod(testStsName+"-1", testStsNamespace, testRevNew, true)
		p2 := makeStsPod(testStsName+"-2", testStsNamespace, testRevNew, true)
		cl := testutils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).WithObjects(p0, p1, p2).Build()
		r := &Reconciler{client: cl, logger: logr.Discard(), clientFactory: healthyFactory(3)}

		result, err := r.reconcilePodRollout(ctx, logr.Discard(), etcd, sts)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(requeued(result)).To(BeFalse())
		for _, p := range []*corev1.Pod{p0, p1, p2} {
			g.Expect(cl.Get(ctx, client.ObjectKeyFromObject(p), &corev1.Pod{})).To(Succeed())
		}
	})

	t.Run("deletes a not-ready outdated pod before a ready one", func(t *testing.T) {
		g := NewWithT(t)
		// The not-ready pod is at the lower ordinal, so an ordinal-only pick would choose p2.
		p0 := makeStsPod(testStsName+"-0", testStsNamespace, testRevNew, true)
		p1 := makeStsPod(testStsName+"-1", testStsNamespace, testRevOld, false)
		p2 := makeStsPod(testStsName+"-2", testStsNamespace, testRevOld, true)
		cl := testutils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).WithObjects(p0, p1, p2).Build()
		r := &Reconciler{client: cl, logger: logr.Discard(), clientFactory: healthyFactory(3)}

		result, err := r.reconcilePodRollout(ctx, logr.Discard(), etcd, sts)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(requeued(result)).To(BeTrue())
		g.Expect(apierrors.IsNotFound(cl.Get(ctx, client.ObjectKeyFromObject(p1), &corev1.Pod{}))).To(BeTrue())
		g.Expect(cl.Get(ctx, client.ObjectKeyFromObject(p0), &corev1.Pod{})).To(Succeed())
		g.Expect(cl.Get(ctx, client.ObjectKeyFromObject(p2), &corev1.Pod{})).To(Succeed())
	})

	t.Run("does not delete a participating outdated pod while an updated pod is unready", func(t *testing.T) {
		g := NewWithT(t)
		p0 := makeStsPod(testStsName+"-0", testStsNamespace, testRevNew, false)
		p1 := makeStsPod(testStsName+"-1", testStsNamespace, testRevOld, true)
		p2 := makeStsPod(testStsName+"-2", testStsNamespace, testRevOld, true)
		cl := testutils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).WithObjects(p0, p1, p2).Build()
		r := &Reconciler{client: cl, logger: logr.Discard(), clientFactory: healthyFactory(3)}

		result, err := r.reconcilePodRollout(ctx, logr.Discard(), etcd, sts)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(requeued(result)).To(BeTrue())
		for _, p := range []*corev1.Pod{p0, p1, p2} {
			g.Expect(cl.Get(ctx, client.ObjectKeyFromObject(p), &corev1.Pod{})).To(Succeed())
		}
	})

	t.Run("deletes a follower before the leader", func(t *testing.T) {
		g := NewWithT(t)
		p0 := makeStsPod(testStsName+"-0", testStsNamespace, testRevOld, true)
		p1 := makeStsPod(testStsName+"-1", testStsNamespace, testRevOld, true)
		p2 := makeStsPod(testStsName+"-2", testStsNamespace, testRevOld, true)
		cl := testutils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).WithObjects(p0, p1, p2).Build()
		// The leader is at the highest ordinal, which an ordinal-only pick would choose first.
		r := &Reconciler{client: cl, logger: logr.Discard(), clientFactory: membersFactory(healthyMembersWithLeader(3, 2)...)}

		result, err := r.reconcilePodRollout(ctx, logr.Discard(), etcd, sts)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(requeued(result)).To(BeTrue())
		g.Expect(cl.Get(ctx, client.ObjectKeyFromObject(p2), &corev1.Pod{})).To(Succeed())
		g.Expect(deletedPodCount(ctx, cl, p0, p1)).To(Equal(1))
	})

	t.Run("single-member cluster deletes its sole outdated pod", func(t *testing.T) {
		g := NewWithT(t)
		singleEtcd := &druidv1alpha1.Etcd{
			ObjectMeta: metav1.ObjectMeta{Name: testStsName, Namespace: testStsNamespace},
			Spec:       druidv1alpha1.EtcdSpec{Replicas: 1},
		}
		singleSts := stsFixture(testStsName, testStsNamespace, testRevNew, 1)
		p := makeStsPod(testStsName+"-0", testStsNamespace, testRevOld, true)
		cl := testutils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).WithObjects(p).Build()
		r := &Reconciler{client: cl, logger: logr.Discard(), clientFactory: healthyFactory(1)}

		result, err := r.reconcilePodRollout(ctx, logr.Discard(), singleEtcd, singleSts)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(requeued(result)).To(BeTrue())
		g.Expect(apierrors.IsNotFound(cl.Get(ctx, client.ObjectKeyFromObject(p), &corev1.Pod{}))).To(BeTrue())
	})

	t.Run("does not delete while a previously deleted pod is still terminating", func(t *testing.T) {
		g := NewWithT(t)
		p0 := makeStsPod(testStsName+"-0", testStsNamespace, testRevNew, true)
		p1 := makeStsPod(testStsName+"-1", testStsNamespace, testRevNew, true)
		p2 := withDeletionTimestamp(makeStsPod(testStsName+"-2", testStsNamespace, testRevOld, false))
		cl := testutils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).WithObjects(p0, p1, p2).Build()
		r := &Reconciler{client: cl, logger: logr.Discard(), clientFactory: healthyFactory(3)}

		result, err := r.reconcilePodRollout(ctx, logr.Discard(), etcd, sts)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(requeued(result)).To(BeTrue())
		g.Expect(cl.Get(ctx, client.ObjectKeyFromObject(p0), &corev1.Pod{})).To(Succeed())
		g.Expect(cl.Get(ctx, client.ObjectKeyFromObject(p1), &corev1.Pod{})).To(Succeed())
	})

	t.Run("does not delete a second pod while the first delete of a fresh rollout is terminating", func(t *testing.T) {
		g := NewWithT(t)
		// No pod is at the target revision yet, so the rejoin gate is vacuous and
		// only the terminating check prevents a back-to-back delete.
		p0 := withDeletionTimestamp(makeStsPod(testStsName+"-0", testStsNamespace, testRevOld, true))
		p1 := makeStsPod(testStsName+"-1", testStsNamespace, testRevOld, true)
		p2 := makeStsPod(testStsName+"-2", testStsNamespace, testRevOld, true)
		cl := testutils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).WithObjects(p0, p1, p2).Build()
		r := &Reconciler{client: cl, logger: logr.Discard(), clientFactory: healthyFactory(3)}

		result, err := r.reconcilePodRollout(ctx, logr.Discard(), etcd, sts)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(requeued(result)).To(BeTrue())
		g.Expect(cl.Get(ctx, client.ObjectKeyFromObject(p1), &corev1.Pod{})).To(Succeed())
		g.Expect(cl.Get(ctx, client.ObjectKeyFromObject(p2), &corev1.Pod{})).To(Succeed())
	})

	t.Run("deletes a not-ready outdated pod even while an updated pod is unready", func(t *testing.T) {
		g := NewWithT(t)
		// A not-ready outdated pod contributes nothing to quorum, so the rejoin gate does not apply to it.
		p0 := makeStsPod(testStsName+"-0", testStsNamespace, testRevNew, false)
		p1 := makeStsPod(testStsName+"-1", testStsNamespace, testRevOld, true)
		p2 := makeStsPod(testStsName+"-2", testStsNamespace, testRevOld, false)
		cl := testutils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).WithObjects(p0, p1, p2).Build()
		r := &Reconciler{client: cl, logger: logr.Discard(), clientFactory: healthyFactory(3)}

		result, err := r.reconcilePodRollout(ctx, logr.Discard(), etcd, sts)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(requeued(result)).To(BeTrue())
		g.Expect(apierrors.IsNotFound(cl.Get(ctx, client.ObjectKeyFromObject(p2), &corev1.Pod{}))).To(BeTrue())
		g.Expect(cl.Get(ctx, client.ObjectKeyFromObject(p0), &corev1.Pod{})).To(Succeed())
		g.Expect(cl.Get(ctx, client.ObjectKeyFromObject(p1), &corev1.Pod{})).To(Succeed())
	})

	t.Run("deletes at most one pod per reconcile", func(t *testing.T) {
		g := NewWithT(t)
		p0 := makeStsPod(testStsName+"-0", testStsNamespace, testRevOld, false)
		p1 := makeStsPod(testStsName+"-1", testStsNamespace, testRevOld, false)
		p2 := makeStsPod(testStsName+"-2", testStsNamespace, testRevOld, false)
		cl := testutils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).WithObjects(p0, p1, p2).Build()
		r := &Reconciler{client: cl, logger: logr.Discard(), clientFactory: healthyFactory(3)}

		_, err := r.reconcilePodRollout(ctx, logr.Discard(), etcd, sts)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(deletedPodCount(ctx, cl, p0, p1, p2)).To(Equal(1))
	})

	t.Run("does not delete while the pod count is below desired (scale-up in progress)", func(t *testing.T) {
		g := NewWithT(t)
		// Only p0 exists so far; deleting it before p1 and p2 are created would drop participation to zero.
		p0 := makeStsPod(testStsName+"-0", testStsNamespace, testRevOld, true)
		cl := testutils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).WithObjects(p0).Build()
		r := &Reconciler{client: cl, logger: logr.Discard(), clientFactory: healthyFactory(3)}

		result, err := r.reconcilePodRollout(ctx, logr.Discard(), etcd, sts)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(requeued(result)).To(BeTrue())
		g.Expect(cl.Get(ctx, client.ObjectKeyFromObject(p0), &corev1.Pod{})).To(Succeed())
	})

	t.Run("deletes exactly one follower per reconcile in a five-member cluster", func(t *testing.T) {
		g := NewWithT(t)
		fiveEtcd := &druidv1alpha1.Etcd{
			ObjectMeta: metav1.ObjectMeta{Name: testStsName, Namespace: testStsNamespace},
			Spec:       druidv1alpha1.EtcdSpec{Replicas: 5},
		}
		fiveSts := stsFixture(testStsName, testStsNamespace, testRevNew, 5)
		pods := make([]*corev1.Pod, 0, 5)
		objs := make([]client.Object, 0, 5)
		for i := range 5 {
			p := makeStsPod(fmt.Sprintf("%s-%d", testStsName, i), testStsNamespace, testRevOld, true)
			pods = append(pods, p)
			objs = append(objs, p)
		}
		cl := testutils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).WithObjects(objs...).Build()
		// The leader is at the highest ordinal, which an ordinal-only pick would choose first.
		r := &Reconciler{client: cl, logger: logr.Discard(), clientFactory: membersFactory(healthyMembersWithLeader(5, 4)...)}

		result, err := r.reconcilePodRollout(ctx, logr.Discard(), fiveEtcd, fiveSts)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(requeued(result)).To(BeTrue())
		g.Expect(cl.Get(ctx, client.ObjectKeyFromObject(pods[4]), &corev1.Pod{})).To(Succeed())
		g.Expect(deletedPodCount(ctx, cl, pods[:4]...)).To(Equal(1))
	})

	t.Run("does not delete when restarting the selected member would break quorum", func(t *testing.T) {
		g := NewWithT(t)
		// member-0 is updated but Unhealthy live. Restarting either outdated
		// member leaves 1 of 3 voters healthy, below the quorum of 2.
		p0 := makeStsPod(testStsName+"-0", testStsNamespace, testRevNew, true)
		p1 := makeStsPod(testStsName+"-1", testStsNamespace, testRevOld, true)
		p2 := makeStsPod(testStsName+"-2", testStsNamespace, testRevOld, true)
		cl := testutils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).WithObjects(p0, p1, p2).Build()
		r := &Reconciler{client: cl, logger: logr.Discard(), clientFactory: membersFactory(
			etcdmember.Member{ID: 1, Name: testStsName + "-0", Role: etcdmember.MemberRoleLeader, Health: etcdmember.MemberHealthUnhealthy},
			etcdmember.Member{ID: 2, Name: testStsName + "-1", Role: etcdmember.MemberRoleMember, Health: etcdmember.MemberHealthHealthy},
			etcdmember.Member{ID: 3, Name: testStsName + "-2", Role: etcdmember.MemberRoleMember, Health: etcdmember.MemberHealthHealthy},
		)}

		result, err := r.reconcilePodRollout(ctx, logr.Discard(), etcd, sts)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(requeued(result)).To(BeTrue())
		for _, p := range []*corev1.Pod{p0, p1, p2} {
			g.Expect(cl.Get(ctx, client.ObjectKeyFromObject(p), &corev1.Pod{})).To(Succeed())
		}
	})

	t.Run("does not delete when an updated pod is Ready in the cache but its member is not healthy (4 members)", func(t *testing.T) {
		g := NewWithT(t)
		// Restarting etcd-test-2 leaves 2 of 4 voters healthy, below the quorum of 3.
		// A removal-based check (3 remaining voters, quorum 2) would have allowed it.
		fourEtcd := &druidv1alpha1.Etcd{
			ObjectMeta: metav1.ObjectMeta{Name: testStsName, Namespace: testStsNamespace},
			Spec:       druidv1alpha1.EtcdSpec{Replicas: 4},
		}
		fourSts := stsFixture(testStsName, testStsNamespace, testRevNew, 4)
		p0 := makeStsPod(testStsName+"-0", testStsNamespace, testRevOld, true)
		p1 := makeStsPod(testStsName+"-1", testStsNamespace, testRevOld, true)
		p2 := makeStsPod(testStsName+"-2", testStsNamespace, testRevOld, true)
		p3 := makeStsPod(testStsName+"-3", testStsNamespace, testRevNew, true)
		cl := testutils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).WithObjects(p0, p1, p2, p3).Build()
		r := &Reconciler{client: cl, logger: logr.Discard(), clientFactory: membersFactory(
			etcdmember.Member{ID: 1, Name: testStsName + "-0", Role: etcdmember.MemberRoleLeader, Health: etcdmember.MemberHealthHealthy},
			etcdmember.Member{ID: 2, Name: testStsName + "-1", Role: etcdmember.MemberRoleMember, Health: etcdmember.MemberHealthHealthy},
			etcdmember.Member{ID: 3, Name: testStsName + "-2", Role: etcdmember.MemberRoleMember, Health: etcdmember.MemberHealthHealthy},
			etcdmember.Member{ID: 4, Name: testStsName + "-3", Role: etcdmember.MemberRoleMember, Health: etcdmember.MemberHealthUnknown},
		)}

		result, err := r.reconcilePodRollout(ctx, logr.Discard(), fourEtcd, fourSts)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(requeued(result)).To(BeTrue())
		for _, p := range []*corev1.Pod{p0, p1, p2, p3} {
			g.Expect(cl.Get(ctx, client.ObjectKeyFromObject(p), &corev1.Pod{})).To(Succeed())
		}
	})

	t.Run("maps a pod to its prefixed etcd member for the quorum check", func(t *testing.T) {
		g := NewWithT(t)
		// The quorum check must resolve pod -> member via GetMemberName, else a prefixed cluster finds no member and stalls forever.
		prefixedEtcd := &druidv1alpha1.Etcd{
			ObjectMeta: metav1.ObjectMeta{Name: testStsName, Namespace: testStsNamespace},
			Spec:       druidv1alpha1.EtcdSpec{Replicas: 3, MemberNamePrefix: ptr.To("pfx")},
		}
		p0 := makeStsPod(testStsName+"-0", testStsNamespace, testRevOld, true)
		p1 := makeStsPod(testStsName+"-1", testStsNamespace, testRevNew, true)
		p2 := makeStsPod(testStsName+"-2", testStsNamespace, testRevNew, true)
		cl := testutils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).WithObjects(p0, p1, p2).Build()
		memberName := druidv1alpha1.GetMemberName(prefixedEtcd.Spec.MemberNamePrefix, p0.Name)
		r := &Reconciler{client: cl, logger: logr.Discard(), clientFactory: membersFactory(
			etcdmember.Member{ID: 1, Name: memberName, Role: etcdmember.MemberRoleMember, Health: etcdmember.MemberHealthHealthy},
			etcdmember.Member{ID: 2, Name: druidv1alpha1.GetMemberName(prefixedEtcd.Spec.MemberNamePrefix, p1.Name), Role: etcdmember.MemberRoleLeader, Health: etcdmember.MemberHealthHealthy},
			etcdmember.Member{ID: 3, Name: druidv1alpha1.GetMemberName(prefixedEtcd.Spec.MemberNamePrefix, p2.Name), Role: etcdmember.MemberRoleMember, Health: etcdmember.MemberHealthHealthy},
		)}

		result, err := r.reconcilePodRollout(ctx, logr.Discard(), prefixedEtcd, sts)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(requeued(result)).To(BeTrue())
		g.Expect(apierrors.IsNotFound(cl.Get(ctx, client.ObjectKeyFromObject(p0), &corev1.Pod{}))).To(BeTrue())
	})

	t.Run("does not delete a pod that has no live etcd member", func(t *testing.T) {
		g := NewWithT(t)
		// The selected pod has no matching live member (e.g. mid-join). Fail closed.
		p0 := makeStsPod(testStsName+"-0", testStsNamespace, testRevOld, true)
		p1 := makeStsPod(testStsName+"-1", testStsNamespace, testRevNew, true)
		p2 := makeStsPod(testStsName+"-2", testStsNamespace, testRevNew, true)
		cl := testutils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).WithObjects(p0, p1, p2).Build()
		r := &Reconciler{client: cl, logger: logr.Discard(), clientFactory: membersFactory(
			etcdmember.Member{ID: 2, Name: testStsName + "-1", Role: etcdmember.MemberRoleLeader, Health: etcdmember.MemberHealthHealthy},
			etcdmember.Member{ID: 3, Name: testStsName + "-2", Role: etcdmember.MemberRoleMember, Health: etcdmember.MemberHealthHealthy},
		)}

		result, err := r.reconcilePodRollout(ctx, logr.Discard(), etcd, sts)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(requeued(result)).To(BeTrue())
		g.Expect(cl.Get(ctx, client.ObjectKeyFromObject(p0), &corev1.Pod{})).To(Succeed())
	})

	t.Run("does not delete when the member list fails", func(t *testing.T) {
		g := NewWithT(t)
		p0 := makeStsPod(testStsName+"-0", testStsNamespace, testRevOld, true)
		p1 := makeStsPod(testStsName+"-1", testStsNamespace, testRevNew, true)
		p2 := makeStsPod(testStsName+"-2", testStsNamespace, testRevNew, true)
		cl := testutils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).WithObjects(p0, p1, p2).Build()
		listErrFactory := &etcdclientfake.Factory{Client: &etcdclientfake.Client{ListErr: fmt.Errorf("boom")}}
		r := &Reconciler{client: cl, logger: logr.Discard(), clientFactory: listErrFactory}

		result, err := r.reconcilePodRollout(ctx, logr.Discard(), etcd, sts)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(requeued(result)).To(BeTrue())
		g.Expect(cl.Get(ctx, client.ObjectKeyFromObject(p0), &corev1.Pod{})).To(Succeed())
	})

	t.Run("does not delete when the etcd client cannot be created", func(t *testing.T) {
		g := NewWithT(t)
		p0 := makeStsPod(testStsName+"-0", testStsNamespace, testRevOld, true)
		p1 := makeStsPod(testStsName+"-1", testStsNamespace, testRevNew, true)
		p2 := makeStsPod(testStsName+"-2", testStsNamespace, testRevNew, true)
		cl := testutils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).WithObjects(p0, p1, p2).Build()
		dialErrFactory := &etcdclientfake.Factory{CreateErr: fmt.Errorf("cannot dial")}
		r := &Reconciler{client: cl, logger: logr.Discard(), clientFactory: dialErrFactory}

		result, err := r.reconcilePodRollout(ctx, logr.Discard(), etcd, sts)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(requeued(result)).To(BeTrue())
		g.Expect(cl.Get(ctx, client.ObjectKeyFromObject(p0), &corev1.Pod{})).To(Succeed())
	})
}

// requeued reports whether the given result will cause controller-runtime to
// re-run Reconcile.
func requeued(result ctrl.Result) bool {
	return result.RequeueAfter > 0
}

// deletedPodCount returns how many of the given pods no longer exist in cl.
func deletedPodCount(ctx context.Context, cl client.Client, pods ...*corev1.Pod) int {
	deleted := 0
	for _, p := range pods {
		if apierrors.IsNotFound(cl.Get(ctx, client.ObjectKeyFromObject(p), &corev1.Pod{})) {
			deleted++
		}
	}
	return deleted
}

// healthyFactory returns a fake etcd client factory reporting a healthy cluster
// of `replicas` voting members named to match the test StatefulSet's pods
// (<testStsName>-<ordinal>), so the quorum gate is transparent to the happy path.
func healthyFactory(replicas int) *etcdclientfake.Factory {
	return &etcdclientfake.Factory{Client: etcdclientfake.NewClient(testStsName, replicas)}
}

// membersFactory returns a fake etcd client factory reporting exactly the given
// members, for tests that control member health or the leader's position.
func membersFactory(members ...etcdmember.Member) *etcdclientfake.Factory {
	return &etcdclientfake.Factory{Client: &etcdclientfake.Client{Members: members}}
}

// healthyMembersWithLeader returns `replicas` healthy voters named to match the
// test StatefulSet's pods, with the leader at leaderOrdinal.
func healthyMembersWithLeader(replicas, leaderOrdinal int) []etcdmember.Member {
	members := make([]etcdmember.Member, 0, replicas)
	for i := range replicas {
		role := etcdmember.MemberRoleMember
		if i == leaderOrdinal {
			role = etcdmember.MemberRoleLeader
		}
		members = append(members, etcdmember.Member{
			ID:     uint64(i) + 1,
			Name:   fmt.Sprintf("%s-%d", testStsName, i),
			Role:   role,
			Health: etcdmember.MemberHealthHealthy,
		})
	}
	return members
}
