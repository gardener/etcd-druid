// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package etcd

import (
	"testing"

	. "github.com/onsi/gomega"
)

// voter builds a healthy voting member.
func voter(id uint64, name string) Member {
	return Member{ID: id, Name: name, Role: MemberRoleMember, Health: MemberHealthHealthy}
}

func TestQuorumSafeToRemove(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		members     []Member
		candidateID uint64
		want        bool
	}{
		{
			name: "learner candidate is always removable, even if voters are unhealthy",
			members: []Member{
				voter(1, "e-0"),
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
				voter(2, "e-1"), voter(3, "e-2"),
			},
			candidateID: 3,
			want:        true,
		},
		{
			name: "3 voters, one surviving voter Unknown -> blocked (fail-closed)",
			members: []Member{
				{ID: 1, Name: "e-0", Role: MemberRoleLeader, Health: MemberHealthHealthy},
				voter(2, "e-1"),
				{ID: 3, Name: "e-2", Role: MemberRoleMember, Health: MemberHealthUnknown},
			},
			candidateID: 2, // survivors {1 healthy, 3 unknown} -> not all healthy -> blocked
			want:        false,
		},
		{
			name: "3 voters, remove the Unknown member itself while others healthy -> allowed",
			members: []Member{
				{ID: 1, Name: "e-0", Role: MemberRoleLeader, Health: MemberHealthHealthy},
				voter(2, "e-1"),
				{ID: 3, Name: "e-2", Role: MemberRoleMember, Health: MemberHealthUnknown},
			},
			candidateID: 3, // survivors {1,2} both healthy -> quorum 2 -> safe
			want:        true,
		},
		{
			name: "5 voters, one Unhealthy, remove a healthy voter -> a survivor is unhealthy -> blocked",
			members: []Member{
				{ID: 1, Name: "e-0", Role: MemberRoleLeader, Health: MemberHealthHealthy},
				voter(2, "e-1"), voter(3, "e-2"), voter(4, "e-3"),
				{ID: 5, Name: "e-4", Role: MemberRoleMember, Health: MemberHealthUnhealthy},
			},
			candidateID: 4, // survivors include 5 (unhealthy) -> blocked
			want:        false,
		},
		{
			name: "5 voters all healthy, remove one voter -> 4 remain healthy, quorum 3, safe",
			members: []Member{
				{ID: 1, Name: "e-0", Role: MemberRoleLeader, Health: MemberHealthHealthy},
				voter(2, "e-1"), voter(3, "e-2"), voter(4, "e-3"), voter(5, "e-4"),
			},
			candidateID: 5,
			want:        true,
		},
		{
			name: "7 voters all healthy -> 6 remain, quorum 4, safe",
			members: []Member{
				{ID: 1, Name: "e-0", Role: MemberRoleLeader, Health: MemberHealthHealthy},
				voter(2, "e-1"), voter(3, "e-2"), voter(4, "e-3"),
				voter(5, "e-4"), voter(6, "e-5"), voter(7, "e-6"),
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
				voter(2, "e-1"), voter(3, "e-2"),
			},
			candidateID: 99,
			want:        false,
		},
		{
			name: "learner in survivor set is ignored: 3 voters + 1 learner, remove a voter -> 2 healthy voters remain, safe",
			members: []Member{
				{ID: 1, Name: "e-0", Role: MemberRoleLeader, Health: MemberHealthHealthy},
				voter(2, "e-1"), voter(3, "e-2"),
				{ID: 9, Name: "e-3", Role: MemberRoleLearner, Health: MemberHealthUnknown},
			},
			candidateID: 3, // survivors: voters {1,2} healthy; learner 9 not counted -> safe
			want:        true,
		},
		{
			name: "single voter plus a learner: removing the sole voter is unsafe",
			members: []Member{
				{ID: 1, Name: "e-0", Role: MemberRoleLeader, Health: MemberHealthHealthy},
				{ID: 9, Name: "e-1", Role: MemberRoleLearner, Health: MemberHealthHealthy},
			},
			candidateID: 1, // removing the only voter leaves zero voters -> unsafe
			want:        false,
		},
		{
			name: "leader as candidate: 3 voters healthy, remove the leader -> 2 healthy voters remain, safe",
			members: []Member{
				{ID: 1, Name: "e-0", Role: MemberRoleLeader, Health: MemberHealthHealthy},
				voter(2, "e-1"), voter(3, "e-2"),
			},
			candidateID: 1, // leader removal, survivors {2,3} healthy -> quorum 2 -> safe
			want:        true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g := NewWithT(t)
			g.Expect(QuorumSafeToRemove(tc.members, tc.candidateID)).To(Equal(tc.want))
		})
	}
}

func TestOrderRemovalCandidates(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		candidates []Member
		wantIDs    []uint64
	}{
		{
			name: "learners first, then voters by ascending ID, leader last",
			candidates: []Member{
				{ID: 1, Name: "etcd-main-0", Role: MemberRoleLeader, Health: MemberHealthHealthy},
				{ID: 2, Name: "etcd-main-3", Role: MemberRoleMember, Health: MemberHealthHealthy},
				{ID: 3, Name: "etcd-main-4", Role: MemberRoleLearner, Health: MemberHealthHealthy},
				{ID: 4, Name: "etcd-main-2", Role: MemberRoleMember, Health: MemberHealthHealthy},
			},
			// learner(ID=3) first; voters by ascending ID: 2, 4; leader(ID=1) last
			wantIDs: []uint64{3, 2, 4, 1},
		},
		{
			name: "no leader: voters ordered by ascending member ID",
			candidates: []Member{
				voter(12, "etcd-main-2"),
				voter(10, "etcd-main-4"),
				voter(11, "etcd-main-3"),
			},
			wantIDs: []uint64{10, 11, 12},
		},
		{
			name: "bootstrap source members ordered by ascending member ID",
			candidates: []Member{
				voter(30, "etcd-source-1"),
				voter(20, "etcd-source-0"),
			},
			wantIDs: []uint64{20, 30},
		},
		{
			name: "multiple learners ordered by ascending member ID within the learner tier",
			candidates: []Member{
				{ID: 30, Name: "learner-2", Role: MemberRoleLearner, Health: MemberHealthHealthy},
				{ID: 10, Name: "learner-0", Role: MemberRoleLearner, Health: MemberHealthHealthy},
				{ID: 20, Name: "learner-1", Role: MemberRoleLearner, Health: MemberHealthHealthy},
			},
			wantIDs: []uint64{10, 20, 30},
		},
		{
			name:       "empty candidate set",
			candidates: []Member{},
			wantIDs:    []uint64{},
		},
		{
			name:       "single candidate",
			candidates: []Member{voter(42, "etcd-main-3")},
			wantIDs:    []uint64{42},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
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
