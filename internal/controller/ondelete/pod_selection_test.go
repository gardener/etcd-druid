// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package ondelete

import (
	"fmt"
	"testing"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	etcdmember "github.com/gardener/etcd-druid/internal/etcd"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	. "github.com/onsi/gomega"
)

// selectionFixture builds a minimal Etcd and a pod with the given name and
// readiness. The pod carries the STS revision label so the caller can control
// what selectNextPodForUpdate sees.
func selectionEtcd(name, namespace string) *druidv1alpha1.Etcd {
	return &druidv1alpha1.Etcd{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec:       druidv1alpha1.EtcdSpec{Replicas: 3},
	}
}

func selectionPod(name string, ready bool) corev1.Pod {
	readyStatus := corev1.ConditionFalse
	if ready {
		readyStatus = corev1.ConditionTrue
	}
	return corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{{
				Name:  "etcd",
				Ready: readyStatus == corev1.ConditionTrue,
			}},
		},
	}
}

func TestComparePodUpdateCandidates(t *testing.T) {
	t.Parallel()

	// Helper to build a candidate with explicit readiness and role.
	cand := func(podName string, readiness etcdContainerReadiness, role memberRolePriority) podUpdateCandidate {
		return podUpdateCandidate{
			pod:          &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: podName}},
			readiness:    readiness,
			rolePriority: role,
		}
	}

	t.Run("not-ready beats ready regardless of role: not-ready leader before ready follower", func(_ *testing.T) {
		g := NewWithT(t)
		notReadyLeader := cand("etcd-0", etcdContainerNotReady, memberRolePriorityLeader)
		readyFollower := cand("etcd-1", etcdContainerReady, memberRolePriorityFollower)
		g.Expect(comparePodUpdateCandidates(notReadyLeader, readyFollower)).To(BeNumerically("<", 0),
			"not-ready pod must sort before ready pod regardless of role")
	})

	t.Run("among not-ready pods: higher ordinal first (role irrelevant)", func(_ *testing.T) {
		g := NewWithT(t)
		notReady0 := cand("etcd-0", etcdContainerNotReady, memberRolePriorityFollower)
		notReady2 := cand("etcd-2", etcdContainerNotReady, memberRolePriorityFollower)
		// ordinal 2 > ordinal 0 so etcd-2 sorts first in the list
		g.Expect(comparePodUpdateCandidates(notReady2, notReady0)).To(BeNumerically("<", 0))
	})

	t.Run("among ready pods: follower before leader", func(_ *testing.T) {
		g := NewWithT(t)
		follower := cand("etcd-0", etcdContainerReady, memberRolePriorityFollower)
		leader := cand("etcd-1", etcdContainerReady, memberRolePriorityLeader)
		g.Expect(comparePodUpdateCandidates(follower, leader)).To(BeNumerically("<", 0))
	})

	t.Run("among ready pods: learner before follower before unknown-role before leader", func(_ *testing.T) {
		g := NewWithT(t)
		learner := cand("etcd-0", etcdContainerReady, memberRolePriorityLearner)
		follower := cand("etcd-1", etcdContainerReady, memberRolePriorityFollower)
		unknownRole := cand("etcd-2", etcdContainerReady, memberRolePriorityUnknown)
		leader := cand("etcd-3", etcdContainerReady, memberRolePriorityLeader)
		g.Expect(comparePodUpdateCandidates(learner, follower)).To(BeNumerically("<", 0))
		g.Expect(comparePodUpdateCandidates(follower, unknownRole)).To(BeNumerically("<", 0))
		g.Expect(comparePodUpdateCandidates(unknownRole, leader)).To(BeNumerically("<", 0))
	})

	t.Run("readiness-unknown groups with ready (conservative): unknown before ready at same role", func(_ *testing.T) {
		g := NewWithT(t)
		unknownReadiness := cand("etcd-0", etcdContainerReadinessUnknown, memberRolePriorityFollower)
		ready := cand("etcd-1", etcdContainerReady, memberRolePriorityFollower)
		// unknown-readiness sorts before ready at same role
		g.Expect(comparePodUpdateCandidates(unknownReadiness, ready)).To(BeNumerically("<", 0))
	})

	t.Run("readiness-unknown does NOT sort into the not-ready group: unknown-readiness follower after not-ready leader", func(_ *testing.T) {
		g := NewWithT(t)
		notReadyPod := cand("etcd-0", etcdContainerNotReady, memberRolePriorityLeader)
		unknownFollower := cand("etcd-1", etcdContainerReadinessUnknown, memberRolePriorityFollower)
		// not-ready wins regardless as unknown-readiness follower should not come before a not-ready pod
		g.Expect(comparePodUpdateCandidates(notReadyPod, unknownFollower)).To(BeNumerically("<", 0))
	})

	t.Run("ordinal tiebreak: among same readiness+role, higher ordinal first", func(_ *testing.T) {
		g := NewWithT(t)
		low := cand("etcd-0", etcdContainerReady, memberRolePriorityFollower)
		high := cand("etcd-2", etcdContainerReady, memberRolePriorityFollower)
		g.Expect(comparePodUpdateCandidates(high, low)).To(BeNumerically("<", 0))
	})

	t.Run("comparator is total: equal candidates return 0", func(_ *testing.T) {
		g := NewWithT(t)
		a := cand("etcd-0", etcdContainerReady, memberRolePriorityFollower)
		b := cand("etcd-0", etcdContainerReady, memberRolePriorityFollower)
		g.Expect(comparePodUpdateCandidates(a, b)).To(Equal(0))
	})
}

func TestSelectNextPodForUpdate(t *testing.T) {
	t.Parallel()
	etcd := selectionEtcd("etcd-test", "default")

	members := func(pods ...string) etcdmember.Members {
		ms := make(etcdmember.Members, 0, len(pods))
		for i, name := range pods {
			role := etcdmember.MemberRoleMember
			if i == 0 {
				role = etcdmember.MemberRoleLeader
			}
			ms = append(ms, etcdmember.Member{
				ID:     uint64(i) + 1,
				Name:   name,
				Role:   role,
				Health: etcdmember.MemberHealthHealthy,
			})
		}
		return ms
	}

	t.Run("nil input returns nil", func(_ *testing.T) {
		g := NewWithT(t)
		g.Expect(selectNextPodForUpdate(etcd, nil, nil)).To(BeNil())
	})

	t.Run("not-ready pod selected before ready pod regardless of member role", func(_ *testing.T) {
		g := NewWithT(t)
		notReady := selectionPod("etcd-test-0", false)
		ready := selectionPod("etcd-test-1", true)
		// member list makes etcd-test-0 the leader, etcd-test-1 the follower
		ms := members("etcd-test-0", "etcd-test-1")
		got := selectNextPodForUpdate(etcd, []corev1.Pod{notReady, ready}, ms)
		g.Expect(got.Name).To(Equal("etcd-test-0"), "not-ready leader must be chosen before ready follower")
	})

	t.Run("among ready pods, follower selected before leader", func(_ *testing.T) {
		g := NewWithT(t)
		follower := selectionPod("etcd-test-1", true)
		leader := selectionPod("etcd-test-0", true)
		ms := members("etcd-test-0", "etcd-test-1")
		got := selectNextPodForUpdate(etcd, []corev1.Pod{follower, leader}, ms)
		g.Expect(got.Name).To(Equal("etcd-test-1"), "follower must be chosen before leader")
	})

	t.Run("pod with no live member (e.g. bootstrapping) treated as unknown role, before leader", func(_ *testing.T) {
		g := NewWithT(t)
		unknownRolePod := selectionPod("etcd-test-2", true) // no entry in member list
		leader := selectionPod("etcd-test-0", true)
		ms := members("etcd-test-0") // only the leader in the list
		got := selectNextPodForUpdate(etcd, []corev1.Pod{unknownRolePod, leader}, ms)
		g.Expect(got.Name).To(Equal("etcd-test-2"), "unknown-role pod must be selected before leader")
	})

	t.Run("MemberNamePrefix: role resolved via prefix, not raw pod name", func(_ *testing.T) {
		g := NewWithT(t)
		// Etcd with a prefix: member names are "<prefix>-<podName>"
		prefixEtcd := &druidv1alpha1.Etcd{
			ObjectMeta: metav1.ObjectMeta{Name: "etcd-test", Namespace: "default"},
			Spec:       druidv1alpha1.EtcdSpec{Replicas: 2, MemberNamePrefix: ptr.To("pfx")},
		}
		follower := selectionPod("etcd-test-1", true)
		leader := selectionPod("etcd-test-0", true)
		// Member names carry the prefix
		ms := etcdmember.Members{
			{ID: 1, Name: "pfx-etcd-test-0", Role: etcdmember.MemberRoleLeader, Health: etcdmember.MemberHealthHealthy},
			{ID: 2, Name: "pfx-etcd-test-1", Role: etcdmember.MemberRoleMember, Health: etcdmember.MemberHealthHealthy},
		}
		got := selectNextPodForUpdate(prefixEtcd, []corev1.Pod{follower, leader}, ms)
		g.Expect(got.Name).To(Equal("etcd-test-1"), "follower must be chosen even with MemberNamePrefix")
	})

	t.Run("high-ordinal not-ready pod chosen before low-ordinal not-ready", func(_ *testing.T) {
		g := NewWithT(t)
		pods := make([]corev1.Pod, 3)
		ms := make(etcdmember.Members, 3)
		for i := range 3 {
			pods[i] = selectionPod(fmt.Sprintf("etcd-test-%d", i), false)
			ms[i] = etcdmember.Member{
				ID: uint64(i) + 1, Name: fmt.Sprintf("etcd-test-%d", i),
				Role: etcdmember.MemberRoleMember, Health: etcdmember.MemberHealthHealthy,
			}
		}
		got := selectNextPodForUpdate(etcd, pods, ms)
		g.Expect(got.Name).To(Equal("etcd-test-2"), "highest ordinal not-ready pod chosen first")
	})
}
