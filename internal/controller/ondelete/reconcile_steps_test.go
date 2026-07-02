// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package ondelete

import (
	"context"
	"testing"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/client/kubernetes"
	"github.com/gardener/etcd-druid/internal/common"
	testutils "github.com/gardener/etcd-druid/test/utils"

	"github.com/go-logr/logr"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/gomega"
)

const (
	testStsNamespace = "test-ns"
	testStsName      = "etcd-test"
	testRevOld       = "rev-1"
	testRevNew       = "rev-2"
)

func TestPartitionByRevision(t *testing.T) {
	g := NewWithT(t)
	t.Parallel()

	sts := stsFixture(testStsName, testStsNamespace, testRevNew, 3)
	current1 := makeStsPod(testStsName+"-0", testStsNamespace, testRevNew, true)
	current2 := makeStsPod(testStsName+"-1", testStsNamespace, testRevNew, true)
	outdated := makeStsPod(testStsName+"-2", testStsNamespace, testRevOld, false)
	terminatingOutdated := withDeletionTimestamp(makeStsPod(testStsName+"-3", testStsNamespace, testRevOld, false))
	terminatingCurrent := withDeletionTimestamp(makeStsPod(testStsName+"-4", testStsNamespace, testRevNew, true))

	pods := []corev1.Pod{*current1, *current2, *outdated, *terminatingOutdated, *terminatingCurrent}
	outdatedGot, currentGot := partitionByRevision(pods, sts)

	g.Expect(outdatedGot).To(HaveLen(1))
	g.Expect(outdatedGot[0].Name).To(Equal(outdated.Name))
	g.Expect(currentGot).To(HaveLen(2))
	g.Expect([]string{currentGot[0].Name, currentGot[1].Name}).To(ConsistOf(current1.Name, current2.Name))
}

func TestSelectNonParticipatingOutdated(t *testing.T) {
	tests := []struct {
		name         string
		buildPods    func() []corev1.Pod
		expectedName string // "" means expect nil
	}{
		{
			name: "all outdated pods participating -> nil",
			buildPods: func() []corev1.Pod {
				return []corev1.Pod{
					*makeStsPod("p-0", testStsNamespace, testRevOld, true),
					*makeStsPod("p-1", testStsNamespace, testRevOld, true),
				}
			},
			expectedName: "",
		},
		{
			name: "Dead beats Transient beats Alive-readiness-failing",
			buildPods: func() []corev1.Pod {
				return []corev1.Pod{
					*makeStsPod("p-alive", testStsNamespace, testRevOld, false), // Running + Ready=false
					*withContainerState(makeStsPod("p-transient", testStsNamespace, testRevOld, false),
						corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "ContainerCreating"}}),
					*withContainerState(makeStsPod("p-dead", testStsNamespace, testRevOld, false),
						corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}}),
				}
			},
			expectedName: "p-dead",
		},
		{
			name: "Transient chosen when Dead absent",
			buildPods: func() []corev1.Pod {
				return []corev1.Pod{
					*makeStsPod("p-alive", testStsNamespace, testRevOld, false),
					*withContainerState(makeStsPod("p-transient", testStsNamespace, testRevOld, false),
						corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "PodInitializing"}}),
				}
			},
			expectedName: "p-transient",
		},
		{
			name: "Alive-readiness-failing chosen last",
			buildPods: func() []corev1.Pod {
				return []corev1.Pod{
					*makeStsPod("p-alive", testStsNamespace, testRevOld, false), // Running + Ready=false
				}
			},
			expectedName: "p-alive",
		},
		{
			name: "Unknown-state pod is skipped",
			buildPods: func() []corev1.Pod {
				unknown := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "p-unknown", Namespace: testStsNamespace}}
				return []corev1.Pod{*unknown}
			},
			expectedName: "",
		},
	}

	g := NewWithT(t)
	t.Parallel()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			pods := tc.buildPods()
			got := selectNonParticipatingOutdated(pods)
			if tc.expectedName == "" {
				g.Expect(got).To(BeNil())
			} else {
				g.Expect(got).ToNot(BeNil())
				g.Expect(got.Name).To(Equal(tc.expectedName))
			}
		})
	}
}

func TestFirstNonParticipatingCurrent(t *testing.T) {
	g := NewWithT(t)
	t.Parallel()

	t.Run("all participating -> nil", func(t *testing.T) {
		pods := []corev1.Pod{
			*makeStsPod("p-0", testStsNamespace, testRevNew, true),
			*makeStsPod("p-1", testStsNamespace, testRevNew, true),
		}
		g.Expect(firstNonParticipatingCurrent(pods)).To(BeNil())
	})

	t.Run("returns the first non-participating pod", func(t *testing.T) {
		pods := []corev1.Pod{
			*makeStsPod("p-0", testStsNamespace, testRevNew, true),
			*makeStsPod("p-1", testStsNamespace, testRevNew, false),
			*makeStsPod("p-2", testStsNamespace, testRevNew, false),
		}
		got := firstNonParticipatingCurrent(pods)
		g.Expect(got).ToNot(BeNil())
		g.Expect(got.Name).To(Equal("p-1"))
	})
}

func TestSelectParticipatingOutdated(t *testing.T) {
	ctx := context.Background()

	t.Run("empty input -> nil, nil", func(t *testing.T) {
		g := NewWithT(t)
		r := &Reconciler{logger: logr.Discard()}
		etcd := &druidv1alpha1.Etcd{Spec: druidv1alpha1.EtcdSpec{Replicas: 3}}
		got, err := r.selectParticipatingOutdated(ctx, etcd, nil)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).To(BeNil())
	})

	t.Run("single-node shortcut returns the sole outdated pod", func(t *testing.T) {
		g := NewWithT(t)
		r := &Reconciler{logger: logr.Discard()}
		etcd := &druidv1alpha1.Etcd{Spec: druidv1alpha1.EtcdSpec{Replicas: 1}}
		pod := makeStsPod(testStsName+"-0", testStsNamespace, testRevOld, true)
		got, err := r.selectParticipatingOutdated(ctx, etcd, []corev1.Pod{*pod})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(got.Name).To(Equal(pod.Name))
	})

	t.Run("follower chosen before leader (multi-node)", func(t *testing.T) {
		g := NewWithT(t)
		etcd := &druidv1alpha1.Etcd{
			ObjectMeta: metav1.ObjectMeta{Name: testStsName, Namespace: testStsNamespace},
			Spec:       druidv1alpha1.EtcdSpec{Replicas: 3},
		}
		p0 := makeStsPod(testStsName+"-0", testStsNamespace, testRevOld, true)
		p1 := makeStsPod(testStsName+"-1", testStsNamespace, testRevOld, true)
		p2 := makeStsPod(testStsName+"-2", testStsNamespace, testRevOld, true)
		cl := testutils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).WithObjects(
			leaseFor(p0.Name, testStsNamespace, "id-0:cid:Member"),
			leaseFor(p1.Name, testStsNamespace, "id-1:cid:Leader"),
			leaseFor(p2.Name, testStsNamespace, "id-2:cid:Member"),
		).Build()
		r := &Reconciler{client: cl, logger: logr.Discard()}

		got, err := r.selectParticipatingOutdated(ctx, etcd, []corev1.Pod{*p1, *p0, *p2})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(got.Name).ToNot(Equal(p1.Name)) // must not be the leader
		g.Expect([]string{p0.Name, p2.Name}).To(ContainElement(got.Name))
	})

	t.Run("all leaders returned means we still return the leader", func(t *testing.T) {
		g := NewWithT(t)
		etcd := &druidv1alpha1.Etcd{
			ObjectMeta: metav1.ObjectMeta{Name: testStsName, Namespace: testStsNamespace},
			Spec:       druidv1alpha1.EtcdSpec{Replicas: 3},
		}
		p := makeStsPod(testStsName+"-0", testStsNamespace, testRevOld, true)
		cl := testutils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).WithObjects(
			leaseFor(p.Name, testStsNamespace, "id-0:cid:Leader"),
		).Build()
		r := &Reconciler{client: cl, logger: logr.Discard()}
		got, err := r.selectParticipatingOutdated(ctx, etcd, []corev1.Pod{*p})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(got.Name).To(Equal(p.Name))
	})

	t.Run("role lookup failure falls back to follower ordering", func(t *testing.T) {
		g := NewWithT(t)
		etcd := &druidv1alpha1.Etcd{
			ObjectMeta: metav1.ObjectMeta{Name: testStsName, Namespace: testStsNamespace},
			Spec:       druidv1alpha1.EtcdSpec{Replicas: 3},
		}
		p0 := makeStsPod(testStsName+"-0", testStsNamespace, testRevOld, true)
		p1 := makeStsPod(testStsName+"-1", testStsNamespace, testRevOld, true)
		// p0's lease has garbage; p1's lease says leader. p0 is treated as a follower and chosen.
		cl := testutils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).WithObjects(
			leaseFor(p0.Name, testStsNamespace, "garbage-no-colons"),
			leaseFor(p1.Name, testStsNamespace, "id-1:cid:Leader"),
		).Build()
		r := &Reconciler{client: cl, logger: logr.Discard()}
		got, err := r.selectParticipatingOutdated(ctx, etcd, []corev1.Pod{*p0, *p1})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(got.Name).To(Equal(p0.Name))
	})
}

func TestDeleteSelected(t *testing.T) {
	ctx := context.Background()
	g := NewWithT(t)

	t.Run("terminating pod is not re-deleted, but a requeue is returned", func(t *testing.T) {
		pod := withDeletionTimestamp(makeStsPod("p-0", testStsNamespace, testRevOld, false))
		cl := testutils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).WithObjects(pod).Build()
		r := &Reconciler{client: cl, logger: logr.Discard()}

		result, err := r.deleteSelected(ctx, logr.Discard(), pod, "Step2:test")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result.Requeue).To(BeTrue())
		g.Expect(cl.Get(ctx, client.ObjectKeyFromObject(pod), &corev1.Pod{})).To(Succeed())
	})

	t.Run("normal delete requeues", func(t *testing.T) {
		pod := makeStsPod("p-0", testStsNamespace, testRevOld, false)
		cl := testutils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).WithObjects(pod).Build()
		r := &Reconciler{client: cl, logger: logr.Discard()}

		result, err := r.deleteSelected(ctx, logr.Discard(), pod, "Step4:test")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result.Requeue).To(BeTrue())
		// Pod should be gone from the fake client.
		g.Expect(apierrors.IsNotFound(cl.Get(ctx, client.ObjectKeyFromObject(pod), &corev1.Pod{}))).To(BeTrue())
	})

	t.Run("pod-not-found on Delete is a benign race and requeues without error", func(t *testing.T) {
		pod := makeStsPod("p-ghost", testStsNamespace, testRevOld, false)
		cl := testutils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).Build() // pod not present
		r := &Reconciler{client: cl, logger: logr.Discard()}

		result, err := r.deleteSelected(ctx, logr.Discard(), pod, "Step2:test")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result.Requeue).To(BeTrue())
	})
}

func TestExecutePodUpdateProcedure(t *testing.T) {
	ctx := context.Background()

	etcd := &druidv1alpha1.Etcd{
		ObjectMeta: metav1.ObjectMeta{Name: testStsName, Namespace: testStsNamespace},
		Spec:       druidv1alpha1.EtcdSpec{Replicas: 3},
	}
	sts := stsFixture(testStsName, testStsNamespace, testRevNew, 3)

	t.Run("Step 1: all up to date -> no delete, no requeue", func(t *testing.T) {
		g := NewWithT(t)
		p0 := makeStsPod(testStsName+"-0", testStsNamespace, testRevNew, true)
		p1 := makeStsPod(testStsName+"-1", testStsNamespace, testRevNew, true)
		p2 := makeStsPod(testStsName+"-2", testStsNamespace, testRevNew, true)
		cl := testutils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).WithObjects(p0, p1, p2).Build()
		r := &Reconciler{client: cl, logger: logr.Discard()}

		result, err := r.executePodUpdateProcedure(ctx, logr.Discard(), sts, etcd, []corev1.Pod{*p0, *p1, *p2})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result.Requeue).To(BeFalse())
		// Verify no pods were deleted.
		for _, p := range []*corev1.Pod{p0, p1, p2} {
			g.Expect(cl.Get(ctx, client.ObjectKeyFromObject(p), &corev1.Pod{})).To(Succeed())
		}
	})

	t.Run("Step 2: deletes the Dead non-participating outdated pod first", func(t *testing.T) {
		g := NewWithT(t)
		p0 := makeStsPod(testStsName+"-0", testStsNamespace, testRevNew, true) // current, participating
		p1 := makeStsPod(testStsName+"-1", testStsNamespace, testRevOld, true) // outdated participating
		p2 := withContainerState(makeStsPod(testStsName+"-2", testStsNamespace, testRevOld, false),
			corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}}) // outdated dead
		cl := testutils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).WithObjects(p0, p1, p2).Build()
		r := &Reconciler{client: cl, logger: logr.Discard()}

		result, err := r.executePodUpdateProcedure(ctx, logr.Discard(), sts, etcd, []corev1.Pod{*p0, *p1, *p2})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result.Requeue).To(BeTrue())
		// Dead outdated pod should be gone.
		g.Expect(apierrors.IsNotFound(cl.Get(ctx, client.ObjectKeyFromObject(p2), &corev1.Pod{}))).To(BeTrue())
		// Others still present.
		g.Expect(cl.Get(ctx, client.ObjectKeyFromObject(p0), &corev1.Pod{})).To(Succeed())
		g.Expect(cl.Get(ctx, client.ObjectKeyFromObject(p1), &corev1.Pod{})).To(Succeed())
	})

	t.Run("Step 3 gate: waits when a current-revision pod is not participating", func(t *testing.T) {
		g := NewWithT(t)
		p0 := makeStsPod(testStsName+"-0", testStsNamespace, testRevNew, false) // current but NOT participating
		p1 := makeStsPod(testStsName+"-1", testStsNamespace, testRevOld, true)  // outdated participating
		cl := testutils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).WithObjects(p0, p1).Build()
		r := &Reconciler{client: cl, logger: logr.Discard()}

		result, err := r.executePodUpdateProcedure(ctx, logr.Discard(), sts, etcd, []corev1.Pod{*p0, *p1})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result.Requeue).To(BeTrue())
		// No pod deleted.
		g.Expect(cl.Get(ctx, client.ObjectKeyFromObject(p0), &corev1.Pod{})).To(Succeed())
		g.Expect(cl.Get(ctx, client.ObjectKeyFromObject(p1), &corev1.Pod{})).To(Succeed())
	})

	t.Run("Step 4: deletes a follower before the leader", func(t *testing.T) {
		g := NewWithT(t)
		p0 := makeStsPod(testStsName+"-0", testStsNamespace, testRevOld, true)
		p1 := makeStsPod(testStsName+"-1", testStsNamespace, testRevOld, true)
		p2 := makeStsPod(testStsName+"-2", testStsNamespace, testRevOld, true)
		cl := testutils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).WithObjects(
			p0, p1, p2,
			leaseFor(p0.Name, testStsNamespace, "id-0:cid:Member"),
			leaseFor(p1.Name, testStsNamespace, "id-1:cid:Leader"),
			leaseFor(p2.Name, testStsNamespace, "id-2:cid:Member"),
		).Build()
		r := &Reconciler{client: cl, logger: logr.Discard()}

		result, err := r.executePodUpdateProcedure(ctx, logr.Discard(), sts, etcd, []corev1.Pod{*p0, *p1, *p2})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result.Requeue).To(BeTrue())
		// Leader must still be present.
		g.Expect(cl.Get(ctx, client.ObjectKeyFromObject(p1), &corev1.Pod{})).To(Succeed())
		// Exactly one follower is gone.
		g0 := cl.Get(ctx, client.ObjectKeyFromObject(p0), &corev1.Pod{})
		g2 := cl.Get(ctx, client.ObjectKeyFromObject(p2), &corev1.Pod{})
		g.Expect(apierrors.IsNotFound(g0) || apierrors.IsNotFound(g2)).To(BeTrue())
		g.Expect(apierrors.IsNotFound(g0) && apierrors.IsNotFound(g2)).To(BeFalse())
	})

	t.Run("Single-node: sole outdated pod is deleted", func(t *testing.T) {
		g := NewWithT(t)
		singleEtcd := &druidv1alpha1.Etcd{
			ObjectMeta: metav1.ObjectMeta{Name: testStsName, Namespace: testStsNamespace},
			Spec:       druidv1alpha1.EtcdSpec{Replicas: 1},
		}
		singleSts := stsFixture(testStsName, testStsNamespace, testRevNew, 1)
		p := makeStsPod(testStsName+"-0", testStsNamespace, testRevOld, true)
		cl := testutils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).WithObjects(p).Build()
		r := &Reconciler{client: cl, logger: logr.Discard()}

		result, err := r.executePodUpdateProcedure(ctx, logr.Discard(), singleSts, singleEtcd, []corev1.Pod{*p})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result.Requeue).To(BeTrue())
		g.Expect(apierrors.IsNotFound(cl.Get(ctx, client.ObjectKeyFromObject(p), &corev1.Pod{}))).To(BeTrue())
	})

	t.Run("Terminating outdated pod does not count toward selection", func(t *testing.T) {
		g := NewWithT(t)
		// All pods at target revision except one that is already terminating (its
		// replacement will land at target revision on recreate). Nothing should be
		// deleted; the reconciler considers the rollout complete.
		p0 := makeStsPod(testStsName+"-0", testStsNamespace, testRevNew, true)
		p1 := makeStsPod(testStsName+"-1", testStsNamespace, testRevNew, true)
		p2 := withDeletionTimestamp(makeStsPod(testStsName+"-2", testStsNamespace, testRevOld, false))
		cl := testutils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).WithObjects(p0, p1, p2).Build()
		r := &Reconciler{client: cl, logger: logr.Discard()}

		result, err := r.executePodUpdateProcedure(ctx, logr.Discard(), sts, etcd, []corev1.Pod{*p0, *p1, *p2})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(result.Requeue).To(BeFalse())
	})

	t.Run("At most one Delete per invocation (Step 2)", func(t *testing.T) {
		g := NewWithT(t)
		p0 := withContainerState(makeStsPod(testStsName+"-0", testStsNamespace, testRevOld, false),
			corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}})
		p1 := withContainerState(makeStsPod(testStsName+"-1", testStsNamespace, testRevOld, false),
			corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}})
		cl := testutils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).WithObjects(p0, p1).Build()
		r := &Reconciler{client: cl, logger: logr.Discard()}

		_, err := r.executePodUpdateProcedure(ctx, logr.Discard(), sts, etcd, []corev1.Pod{*p0, *p1})
		g.Expect(err).ToNot(HaveOccurred())

		g0 := cl.Get(ctx, client.ObjectKeyFromObject(p0), &corev1.Pod{})
		g1 := cl.Get(ctx, client.ObjectKeyFromObject(p1), &corev1.Pod{})
		deleted := 0
		if apierrors.IsNotFound(g0) {
			deleted++
		}
		if apierrors.IsNotFound(g1) {
			deleted++
		}
		g.Expect(deleted).To(Equal(1))
	})
}

// leaseFor builds a member lease with the given HolderIdentity string.
func leaseFor(name, namespace, holderIdentity string) *coordinationv1.Lease {
	return &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec:       coordinationv1.LeaseSpec{HolderIdentity: ptr.To(holderIdentity)},
	}
}

// silence "imported and not used" if common ever unused after refactors
var _ = common.ContainerNameEtcd
