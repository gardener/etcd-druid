// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package etcd

import (
	"testing"

	. "github.com/onsi/gomega"
)

// healthyVoter builds a healthy voting member.
func healthyVoter(id uint64, name string) Member {
	return Member{ID: id, Name: name, Role: MemberRoleMember, Health: MemberHealthHealthy}
}

func TestQuorumSafeToRemove(t *testing.T) {
	tests := []struct {
		name        string
		members     []Member
		candidateID uint64
		want        bool
	}{
		{
			name: "learner candidate is always removable, even if voters are unhealthy",
			members: []Member{
				healthyVoter(1, "e-0"),
				{ID: 2, Name: "e-1", Role: MemberRoleMember, Health: MemberHealthUnhealthy},
				{ID: 3, Name: "e-2", Role: MemberRoleMember, Health: MemberHealthUnknown},
				{ID: 9, Name: "e-3", Role: MemberRoleLearner, Health: MemberHealthHealthy},
			},
			candidateID: 9,
			want:        true,
		},
		{
			name: "3 voters all healthy, remove one -> 2 remain healthy, quorum 2, safe",
			members: []Member{
				{ID: 1, Name: "e-0", Role: MemberRoleLeader, Health: MemberHealthHealthy},
				healthyVoter(2, "e-1"), healthyVoter(3, "e-2"),
			},
			candidateID: 3,
			want:        true,
		},
		{
			name: "3 voters, one surviving voter Unknown -> blocked (fail-closed)",
			members: []Member{
				{ID: 1, Name: "e-0", Role: MemberRoleLeader, Health: MemberHealthHealthy},
				healthyVoter(2, "e-1"),
				{ID: 3, Name: "e-2", Role: MemberRoleMember, Health: MemberHealthUnknown},
			},
			candidateID: 2, // survivors {1 healthy, 3 unknown} -> not all healthy -> blocked
			want:        false,
		},
		{
			name: "3 voters, remove the Unknown member itself while others healthy -> allowed",
			members: []Member{
				{ID: 1, Name: "e-0", Role: MemberRoleLeader, Health: MemberHealthHealthy},
				healthyVoter(2, "e-1"),
				{ID: 3, Name: "e-2", Role: MemberRoleMember, Health: MemberHealthUnknown},
			},
			candidateID: 3, // survivors {1,2} both healthy -> quorum 2 -> safe
			want:        true,
		},
		{
			name: "5 voters, one Unhealthy, remove a healthy voter -> a survivor is unhealthy -> blocked",
			members: []Member{
				{ID: 1, Name: "e-0", Role: MemberRoleLeader, Health: MemberHealthHealthy},
				healthyVoter(2, "e-1"), healthyVoter(3, "e-2"), healthyVoter(4, "e-3"),
				{ID: 5, Name: "e-4", Role: MemberRoleMember, Health: MemberHealthUnhealthy},
			},
			candidateID: 4, // survivors include 5 (unhealthy) -> blocked
			want:        false,
		},
		{
			name: "5 voters all healthy, remove one voter -> 4 remain healthy, quorum 3, safe",
			members: []Member{
				{ID: 1, Name: "e-0", Role: MemberRoleLeader, Health: MemberHealthHealthy},
				healthyVoter(2, "e-1"), healthyVoter(3, "e-2"), healthyVoter(4, "e-3"), healthyVoter(5, "e-4"),
			},
			candidateID: 5,
			want:        true,
		},
		{
			name: "7 voters all healthy -> 6 remain, quorum 4, safe",
			members: []Member{
				{ID: 1, Name: "e-0", Role: MemberRoleLeader, Health: MemberHealthHealthy},
				healthyVoter(2, "e-1"), healthyVoter(3, "e-2"), healthyVoter(4, "e-3"),
				healthyVoter(5, "e-4"), healthyVoter(6, "e-5"), healthyVoter(7, "e-6"),
			},
			candidateID: 7,
			want:        true,
		},
		{
			name: "removing the last voter is unsafe",
			members: []Member{
				{ID: 1, Name: "e-0", Role: MemberRoleLeader, Health: MemberHealthHealthy},
			},
			candidateID: 1,
			want:        false,
		},
		{
			name: "unknown candidate ID -> fail closed",
			members: []Member{
				{ID: 1, Name: "e-0", Role: MemberRoleLeader, Health: MemberHealthHealthy},
				healthyVoter(2, "e-1"), healthyVoter(3, "e-2"),
			},
			candidateID: 99,
			want:        false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(QuorumSafeToRemove(tc.members, tc.candidateID)).To(Equal(tc.want))
		})
	}
}

func TestOrderRemovalCandidates(t *testing.T) {
	tests := []struct {
		name       string
		candidates []Member
		wantIDs    []uint64
	}{
		{
			name: "learners first, then voters by member ID, leader last",
			candidates: []Member{
				{ID: 1, Name: "etcd-main-0", Role: MemberRoleLeader, Health: MemberHealthHealthy},  // leader
				{ID: 2, Name: "etcd-main-3", Role: MemberRoleMember, Health: MemberHealthHealthy},  // healthyVoter
				{ID: 3, Name: "etcd-main-4", Role: MemberRoleLearner, Health: MemberHealthHealthy}, // learner
				{ID: 4, Name: "etcd-main-2", Role: MemberRoleMember, Health: MemberHealthHealthy},  // healthyVoter
			},
			// learner(3) first; voters by ascending ID: id2, id4; leader id1 last
			wantIDs: []uint64{3, 2, 4, 1},
		},
		{
			name: "no leader in set: voters ordered by member ID",
			candidates: []Member{
				healthyVoter(12, "etcd-main-2"),
				healthyVoter(10, "etcd-main-4"),
				healthyVoter(11, "etcd-main-3"),
			},
			wantIDs: []uint64{10, 11, 12},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			ordered := OrderRemovalCandidates(tc.candidates)
			gotIDs := make([]uint64, len(ordered))
			for i, m := range ordered {
				gotIDs[i] = m.ID
			}
			g.Expect(gotIDs).To(Equal(tc.wantIDs))
		})
	}
}

func TestAllMembersHealthy(t *testing.T) {
	members := []Member{
		{ID: 1, Name: "etcd-main-0", Role: MemberRoleLeader, Health: MemberHealthHealthy},
		healthyVoter(2, "etcd-main-1"),
		{ID: 3, Name: "etcd-main-2", Role: MemberRoleMember, Health: MemberHealthUnknown},
	}
	tests := []struct {
		name     string
		required map[string]bool
		want     bool
	}{
		{
			name:     "empty required -> false",
			required: map[string]bool{},
			want:     false,
		},
		{
			name:     "all required present and healthy",
			required: map[string]bool{"etcd-main-0": true, "etcd-main-1": true},
			want:     true,
		},
		{
			name:     "a required member is unhealthy",
			required: map[string]bool{"etcd-main-1": true, "etcd-main-2": true},
			want:     false,
		},
		{
			name:     "a required member is absent from the live list",
			required: map[string]bool{"etcd-main-0": true, "etcd-source-9": true},
			want:     false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(AllMembersHealthy(members, tc.required)).To(Equal(tc.want))
		})
	}
}

func TestSurplusMemberNames(t *testing.T) {
	members := []Member{
		healthyVoter(1, "etcd-main-0"),
		healthyVoter(2, "etcd-main-1"),
		healthyVoter(3, "etcd-main-2"),
		healthyVoter(9, "etcd-source-1"),
	}
	tests := []struct {
		name     string
		expected map[string]bool
		want     map[string]bool
	}{
		{
			name:     "one member not expected -> surplus",
			expected: map[string]bool{"etcd-main-0": true, "etcd-main-1": true, "etcd-main-2": true},
			want:     map[string]bool{"etcd-source-1": true},
		},
		{
			name:     "all members expected -> no surplus",
			expected: map[string]bool{"etcd-main-0": true, "etcd-main-1": true, "etcd-main-2": true, "etcd-source-1": true},
			want:     map[string]bool{},
		},
		{
			name:     "empty expected -> all surplus",
			expected: map[string]bool{},
			want:     map[string]bool{"etcd-main-0": true, "etcd-main-1": true, "etcd-main-2": true, "etcd-source-1": true},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			g.Expect(SurplusMemberNames(members, tc.expected)).To(Equal(tc.want))
		})
	}
}

func TestSelectNextRemovalCandidate(t *testing.T) {
	members := []Member{
		{ID: 1, Name: "etcd-main-0", Role: MemberRoleLeader, Health: MemberHealthHealthy},
		healthyVoter(2, "etcd-main-1"),
		healthyVoter(3, "etcd-main-2"),
	}
	tests := []struct {
		name    string
		surplus map[string]bool
		wantID  uint64 // 0 means expect nil
	}{
		{
			name:    "no surplus -> nil",
			surplus: map[string]bool{},
			wantID:  0,
		},
		{
			name:    "single surplus voter selected",
			surplus: map[string]bool{"etcd-main-2": true},
			wantID:  3,
		},
		{
			name:    "surplus name absent from live members -> nil",
			surplus: map[string]bool{"etcd-source-9": true},
			wantID:  0,
		},
		{
			name:    "multiple surplus: voter removed before leader",
			surplus: map[string]bool{"etcd-main-0": true, "etcd-main-2": true},
			wantID:  3, // etcd-main-2 (voter) removed before the leader etcd-main-0
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			got := SelectNextRemovalCandidate(members, tc.surplus)
			if tc.wantID == 0 {
				g.Expect(got).To(BeNil())
				return
			}
			g.Expect(got).NotTo(BeNil())
			g.Expect(got.ID).To(Equal(tc.wantID))
		})
	}
}
