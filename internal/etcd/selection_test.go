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
			candidateID: 2, // survivors {1 healthy, 3 unknown} -> 1 of 2 healthy, quorum 2 -> blocked
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
			name: "5 voters, one Unhealthy, remove a healthy voter -> 3 of 4 survivors healthy, quorum 3, safe",
			members: []Member{
				{ID: 1, Name: "e-0", Role: MemberRoleLeader, Health: MemberHealthHealthy},
				healthyVoter(2, "e-1"), healthyVoter(3, "e-2"), healthyVoter(4, "e-3"),
				{ID: 5, Name: "e-4", Role: MemberRoleMember, Health: MemberHealthUnhealthy},
			},
			candidateID: 4,
			want:        true,
		},
		{
			name: "5 voters, two Unhealthy, remove a healthy voter -> 2 of 4 survivors healthy, quorum 3, blocked",
			members: []Member{
				{ID: 1, Name: "e-0", Role: MemberRoleLeader, Health: MemberHealthHealthy},
				healthyVoter(2, "e-1"), healthyVoter(3, "e-2"),
				{ID: 4, Name: "e-3", Role: MemberRoleMember, Health: MemberHealthUnhealthy},
				{ID: 5, Name: "e-4", Role: MemberRoleMember, Health: MemberHealthUnhealthy},
			},
			candidateID: 3,
			want:        false,
		},
		{
			name: "3 target + 3 source voters, two source members Unhealthy, remove a healthy source -> 3 of 5 healthy, quorum 3, safe",
			members: []Member{
				{ID: 1, Name: "t-0", Role: MemberRoleLeader, Health: MemberHealthHealthy},
				healthyVoter(2, "t-1"), healthyVoter(3, "t-2"), healthyVoter(4, "s-0"),
				{ID: 5, Name: "s-1", Role: MemberRoleMember, Health: MemberHealthUnhealthy},
				{ID: 6, Name: "s-2", Role: MemberRoleMember, Health: MemberHealthUnhealthy},
			},
			candidateID: 4,
			want:        true,
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

// TestMembersAllHealthy verifies that AllHealthy is true only when every member
// is healthy, and true for an empty list.
func TestMembersAllHealthy(t *testing.T) {
	unknown := Member{ID: 3, Name: "etcd-main-2", Role: MemberRoleMember, Health: MemberHealthUnknown}
	tests := []struct {
		name    string
		members Members
		want    bool
	}{
		{name: "empty list -> true", members: nil, want: true},
		{name: "all healthy -> true", members: Members{healthyVoter(1, "etcd-main-0"), healthyVoter(2, "etcd-main-1")}, want: true},
		{name: "one member not healthy -> false", members: Members{healthyVoter(1, "etcd-main-0"), unknown}, want: false},
	}
	t.Parallel()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g := NewWithT(t)
			g.Expect(tc.members.AllHealthy()).To(Equal(tc.want))
		})
	}
}

// TestMemberNamesHas verifies name lookups, including on a nil list.
func TestMemberNamesHas(t *testing.T) {
	g := NewWithT(t)
	names := MemberNames{"etcd-main-0", "etcd-main-1"}
	g.Expect(names.Has("etcd-main-1")).To(BeTrue())
	g.Expect(names.Has("etcd-main-2")).To(BeFalse())
	g.Expect(MemberNames(nil).Has("etcd-main-0")).To(BeFalse())
}

// TestMembersSplit verifies that Split separates the members named in the list
// from the rest and keeps the original order in both results.
func TestMembersSplit(t *testing.T) {
	members := Members{
		healthyVoter(1, "etcd-main-0"),
		healthyVoter(9, "etcd-source-1"),
		healthyVoter(2, "etcd-main-1"),
		healthyVoter(3, "etcd-main-2"),
	}
	tests := []struct {
		name    string
		names   MemberNames
		wantIn  []string
		wantOut []string
	}{
		{
			name:    "one member not named -> it is out",
			names:   MemberNames{"etcd-main-0", "etcd-main-1", "etcd-main-2"},
			wantIn:  []string{"etcd-main-0", "etcd-main-1", "etcd-main-2"},
			wantOut: []string{"etcd-source-1"},
		},
		{
			name:   "all members named -> none out",
			names:  MemberNames{"etcd-main-0", "etcd-main-1", "etcd-main-2", "etcd-source-1"},
			wantIn: []string{"etcd-main-0", "etcd-source-1", "etcd-main-1", "etcd-main-2"},
		},
		{
			name:    "no names -> all out",
			names:   nil,
			wantOut: []string{"etcd-main-0", "etcd-source-1", "etcd-main-1", "etcd-main-2"},
		},
	}
	t.Parallel()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g := NewWithT(t)
			in, out := members.Split(tc.names)
			g.Expect(memberNamesOf(in)).To(Equal(tc.wantIn))
			g.Expect(memberNamesOf(out)).To(Equal(tc.wantOut))
		})
	}
}

// memberNamesOf returns the names of ms in order, or nil when ms is empty.
func memberNamesOf(ms Members) []string {
	var names []string
	for _, m := range ms {
		names = append(names, m.Name)
	}
	return names
}

// TestSelectNextRemovalCandidate verifies that the next member to remove is
// taken from the surplus members in removal order, and that there is none when
// surplus is empty.
func TestSelectNextRemovalCandidate(t *testing.T) {
	leader := Member{ID: 1, Name: "etcd-main-0", Role: MemberRoleLeader, Health: MemberHealthHealthy}
	tests := []struct {
		name    string
		surplus Members
		wantID  uint64 // 0 means expect nil
	}{
		{name: "no surplus -> nil", surplus: nil, wantID: 0},
		{name: "single surplus voter selected", surplus: Members{healthyVoter(3, "etcd-main-2")}, wantID: 3},
		{
			name:    "multiple surplus: voter removed before leader",
			surplus: Members{leader, healthyVoter(3, "etcd-main-2")},
			wantID:  3,
		},
	}
	t.Parallel()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g := NewWithT(t)
			got := SelectNextRemovalCandidate(tc.surplus)
			if tc.wantID == 0 {
				g.Expect(got).To(BeNil())
				return
			}
			g.Expect(got).NotTo(BeNil())
			g.Expect(got.ID).To(Equal(tc.wantID))
		})
	}
}

// TestSelectLeaderTransferee verifies that a healthy voting member is chosen
// from the retained members, that last-preference members are used only when
// no other member qualifies, and that ties are broken by lowest ID.
func TestSelectLeaderTransferee(t *testing.T) {
	tests := []struct {
		name           string
		retained       Members
		lastPreference MemberNames
		wantID         uint64
		wantNil        bool
	}{
		{
			name:     "lowest-ID healthy voter is chosen",
			retained: Members{healthyVoter(3, "t-2"), healthyVoter(2, "t-1")},
			wantID:   2,
		},
		{
			name:           "other members win over a lower-ID last-preference member",
			retained:       Members{healthyVoter(1, "s-0"), healthyVoter(5, "t-0")},
			lastPreference: MemberNames{"s-0"},
			wantID:         5,
		},
		{
			name: "last-preference member is used when no other member is healthy",
			retained: Members{
				{ID: 1, Name: "t-0", Role: MemberRoleMember, Health: MemberHealthUnknown},
				healthyVoter(2, "s-0"),
			},
			lastPreference: MemberNames{"s-0"},
			wantID:         2,
		},
		{
			name: "unhealthy, learner and leader members are skipped",
			retained: Members{
				{ID: 1, Name: "t-0", Role: MemberRoleMember, Health: MemberHealthUnknown},
				{ID: 2, Name: "t-1", Role: MemberRoleLearner, Health: MemberHealthHealthy},
				{ID: 3, Name: "t-2", Role: MemberRoleLeader, Health: MemberHealthHealthy},
			},
			wantNil: true,
		},
		{
			name:    "no retained members -> nil",
			wantNil: true,
		},
	}
	t.Parallel()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g := NewWithT(t)
			got := SelectLeaderTransferee(tc.retained, tc.lastPreference)
			if tc.wantNil {
				g.Expect(got).To(BeNil())
				return
			}
			g.Expect(got).NotTo(BeNil())
			g.Expect(got.ID).To(Equal(tc.wantID))
		})
	}
}
