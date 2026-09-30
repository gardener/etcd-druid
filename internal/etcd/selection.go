// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package etcd

import "sort"

// QuorumSafeToRemove reports whether removing the member identified by
// candidateID is safe with respect to quorum, using the health embedded in each
// Member (populated from a live etcd Status probe). It takes no external health
// maps: all health data comes from members[i].Health.
//
// The rules are:
//   - A learner candidate is always safe to remove: learners do not participate
//     in quorum.
//   - A voter candidate is safe only when the healthy surviving voters (all
//     voters except the candidate) still form a quorum of the remaining voters,
//     i.e. at least ⌊(remaining voters)/2⌋ + 1. Unknown and Unhealthy members
//     are fail-closed and do not count as healthy.
//
// This mirrors the quorum check etcd applies to MemberRemove when
// strict-reconfig-check is enabled, so the controller holds with a clear reason
// instead of issuing a removal that would leave the cluster without quorum.
func QuorumSafeToRemove(members Members, candidateID uint64) bool {
	var candidate *Member
	for i := range members {
		if members[i].ID == candidateID {
			candidate = &members[i]
			break
		}
	}
	if candidate == nil {
		return false
	}
	if candidate.IsLearner() {
		return true
	}

	votersAfter := 0
	healthyVotersAfter := 0
	for _, m := range members {
		if m.ID == candidateID || !m.IsVoter() {
			continue
		}
		votersAfter++
		if m.IsHealthy() {
			healthyVotersAfter++
		}
	}

	// Removing the last voter would leave no cluster; treat as unsafe.
	if votersAfter == 0 {
		return false
	}

	quorumAfter := votersAfter/2 + 1
	return healthyVotersAfter >= quorumAfter
}

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

// OrderRemovalCandidates orders an already-selected candidate set into the
// sequence in which members should be removed: learners first, then non-leader
// voters, and the leader last. This ordering only sequences the removals within
// the selected set; it does not change which members are removed. Removing the
// leader last avoids an unnecessary leadership change mid-operation.
//
// The leader is derived from each Member's Role (MemberRoleLeader). Within a
// tier, unhealthy members are ordered before healthy ones so that a failing
// member is shed first, and members of equal health are ordered by member ID
// for determinism.
func OrderRemovalCandidates(candidates Members) Members {
	ordered := make(Members, len(candidates))
	copy(ordered, candidates)

	tier := func(m Member) int {
		switch {
		case m.IsLearner():
			return 0
		case m.Role == MemberRoleLeader:
			return 2
		default:
			return 1
		}
	}

	// unhealthyFirst returns 0 for unhealthy members and 1 for healthy ones, so
	// that within a tier the unhealthy members sort ahead of the healthy ones.
	unhealthyFirst := func(m Member) int {
		if m.Health == MemberHealthHealthy {
			return 1
		}
		return 0
	}

	sort.SliceStable(ordered, func(i, j int) bool {
		ti, tj := tier(ordered[i]), tier(ordered[j])
		if ti != tj {
			return ti < tj
		}
		hi, hj := unhealthyFirst(ordered[i]), unhealthyFirst(ordered[j])
		if hi != hj {
			return hi < hj
		}
		return ordered[i].ID < ordered[j].ID
	})
	return ordered
}
