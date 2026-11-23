// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package ondelete

import (
	"cmp"
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
	cand := func(podName string, readiness etcdContainerReadiness, role memberRolePriority) podUpdateCandidate {
		return podUpdateCandidate{
			pod:          &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: podName}},
			readiness:    readiness,
			rolePriority: role,
		}
	}

	tests := []struct {
		name     string
		a, b     podUpdateCandidate
		wantSign int
	}{
		{
			name:     "not-ready leader sorts before ready follower",
			a:        cand("etcd-0", etcdContainerNotReady, memberRolePriorityLeader),
			b:        cand("etcd-1", etcdContainerReady, memberRolePriorityFollower),
			wantSign: -1,
		},
		{
			name:     "ready follower sorts after not-ready leader",
			a:        cand("etcd-1", etcdContainerReady, memberRolePriorityFollower),
			b:        cand("etcd-0", etcdContainerNotReady, memberRolePriorityLeader),
			wantSign: 1,
		},
		{
			name:     "among not-ready pods, higher ordinal sorts first",
			a:        cand("etcd-2", etcdContainerNotReady, memberRolePriorityFollower),
			b:        cand("etcd-0", etcdContainerNotReady, memberRolePriorityFollower),
			wantSign: -1,
		},
		{
			name:     "among not-ready pods, role is ignored: higher-ordinal leader sorts first",
			a:        cand("etcd-2", etcdContainerNotReady, memberRolePriorityLeader),
			b:        cand("etcd-0", etcdContainerNotReady, memberRolePriorityFollower),
			wantSign: -1,
		},
		{
			name:     "among ready pods, follower sorts before leader",
			a:        cand("etcd-0", etcdContainerReady, memberRolePriorityFollower),
			b:        cand("etcd-1", etcdContainerReady, memberRolePriorityLeader),
			wantSign: -1,
		},
		{
			name:     "among ready pods, leader sorts after follower",
			a:        cand("etcd-1", etcdContainerReady, memberRolePriorityLeader),
			b:        cand("etcd-0", etcdContainerReady, memberRolePriorityFollower),
			wantSign: 1,
		},
		{
			name:     "among ready pods, learner sorts before follower",
			a:        cand("etcd-0", etcdContainerReady, memberRolePriorityLearner),
			b:        cand("etcd-1", etcdContainerReady, memberRolePriorityFollower),
			wantSign: -1,
		},
		{
			name:     "among ready pods, follower sorts before unknown role",
			a:        cand("etcd-1", etcdContainerReady, memberRolePriorityFollower),
			b:        cand("etcd-2", etcdContainerReady, memberRolePriorityUnknown),
			wantSign: -1,
		},
		{
			name:     "among ready pods, unknown role sorts before leader",
			a:        cand("etcd-2", etcdContainerReady, memberRolePriorityUnknown),
			b:        cand("etcd-3", etcdContainerReady, memberRolePriorityLeader),
			wantSign: -1,
		},
		{
			name:     "unknown readiness sorts before ready at the same role",
			a:        cand("etcd-0", etcdContainerReadinessUnknown, memberRolePriorityFollower),
			b:        cand("etcd-1", etcdContainerReady, memberRolePriorityFollower),
			wantSign: -1,
		},
		{
			name:     "unknown readiness does not join the not-ready group: not-ready leader sorts first",
			a:        cand("etcd-0", etcdContainerNotReady, memberRolePriorityLeader),
			b:        cand("etcd-1", etcdContainerReadinessUnknown, memberRolePriorityFollower),
			wantSign: -1,
		},
		{
			name:     "same readiness and role, higher ordinal sorts first",
			a:        cand("etcd-2", etcdContainerReady, memberRolePriorityFollower),
			b:        cand("etcd-0", etcdContainerReady, memberRolePriorityFollower),
			wantSign: -1,
		},
		{
			name:     "equal candidates compare as 0",
			a:        cand("etcd-0", etcdContainerReady, memberRolePriorityFollower),
			b:        cand("etcd-0", etcdContainerReady, memberRolePriorityFollower),
			wantSign: 0,
		},
	}

	t.Parallel()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g := NewWithT(t)
			g.Expect(cmp.Compare(comparePodUpdateCandidates(tc.a, tc.b), 0)).To(Equal(tc.wantSign))
		})
	}
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
