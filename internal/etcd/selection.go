// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package etcd

import "sort"

// SelectNextRemovalCandidate returns the surplus member to remove next, or nil
// when surplus is empty. Ordering follows OrderRemovalCandidates (learners
// first, leader last).
func SelectNextRemovalCandidate(surplus Members) *Member {
	if len(surplus) == 0 {
		return nil
	}
	// OrderRemovalCandidates copies, so the returned pointer stays valid.
	ordered := OrderRemovalCandidates(surplus)
	return &ordered[0]
}

// SelectLeaderTransferee returns a healthy voting member from retained to take
// over leadership from a leader that is about to be removed, or nil when there
// is none. Members in lastPreference are chosen only when no other retained
// member qualifies. Among equally preferred members, the lowest member ID wins
// so the choice is deterministic.
func SelectLeaderTransferee(retained Members, lastPreference MemberNames) *Member {
	var preferred, fallback *Member
	for i := range retained {
		m := &retained[i]
		if m.Role != MemberRoleMember || !m.IsHealthy() {
			continue
		}
		if lastPreference.Has(m.Name) {
			if fallback == nil || m.ID < fallback.ID {
				fallback = m
			}
			continue
		}
		if preferred == nil || m.ID < preferred.ID {
			preferred = m
		}
	}
	if preferred != nil {
		return preferred
	}
	return fallback
}

// OrderRemovalCandidates returns candidates in the order they should be removed:
// learners, then unhealthy voters, then healthy voters, and the leader last.
// Removing the leader last avoids a leadership change mid-operation, and
// removing unhealthy voters first sheds a failing member early. Members of the
// same kind are ordered by member ID for determinism.
//
// It only orders the given set; it does not decide which members are removed.
func OrderRemovalCandidates(candidates Members) Members {
	ordered := make(Members, len(candidates))
	copy(ordered, candidates)

	tier := func(m Member) int {
		switch {
		case m.IsLearner():
			return 0
		case m.Role == MemberRoleLeader:
			return 3
		case m.Health != MemberHealthHealthy:
			return 1
		default:
			return 2
		}
	}

	sort.SliceStable(ordered, func(i, j int) bool {
		ti, tj := tier(ordered[i]), tier(ordered[j])
		if ti != tj {
			return ti < tj
		}
		return ordered[i].ID < ordered[j].ID
	})
	return ordered
}
