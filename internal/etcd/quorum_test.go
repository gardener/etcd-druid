// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package etcd

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestCanRemoveMemberWithoutLosingQuorum(t *testing.T) {
	tests := []struct {
		name        string
		members     []Member
		candidateID uint64
		want        bool
		wantErr     bool
	}{
		{
			name: "learner candidate is always removable when quorum is healthy",
			members: []Member{
				{ID: 1, Name: "e-0", Role: MemberRoleLeader, Health: MemberHealthHealthy},
				healthyVoter(2, "e-1"), healthyVoter(3, "e-2"),
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
			name: "current quorum already unhealthy (only 1 of 3 healthy) -> no removal is safe",
			members: []Member{
				{ID: 1, Name: "e-0", Role: MemberRoleLeader, Health: MemberHealthHealthy},
				{ID: 2, Name: "e-1", Role: MemberRoleMember, Health: MemberHealthUnhealthy},
				{ID: 3, Name: "e-2", Role: MemberRoleMember, Health: MemberHealthUnknown},
			},
			candidateID: 1,
			want:        false,
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
			name: "removing the last voter is unsafe",
			members: []Member{
				{ID: 1, Name: "e-0", Role: MemberRoleLeader, Health: MemberHealthHealthy},
			},
			candidateID: 1,
			want:        false,
		},
		{
			name: "unknown candidate ID -> error, not a quorum verdict",
			members: []Member{
				{ID: 1, Name: "e-0", Role: MemberRoleLeader, Health: MemberHealthHealthy},
				healthyVoter(2, "e-1"), healthyVoter(3, "e-2"),
			},
			candidateID: 99,
			want:        false,
			wantErr:     true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			got, err := CanRemoveMemberWithoutLosingQuorum(tc.members, tc.candidateID)
			if tc.wantErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
			g.Expect(got).To(Equal(tc.want))
		})
	}
}
