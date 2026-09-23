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
//   - A voter candidate is safe only when every surviving voter (all voters
//     except the candidate) is Healthy. Unknown and Unhealthy are fail-closed as
//     not healthy, so a single non-healthy survivor blocks the removal.
//   - As a backstop, even after the health gate passes, the number of healthy
//     surviving voters must be at least quorum(remaining voters) =
//     ⌊(remaining voters)/2⌋ + 1.
//
// etcd itself also enforces a quorum check on MemberRemove; this pre-check lets
// the controller avoid issuing a removal that etcd would reject and surface a
// clear reason instead.
func QuorumSafeToRemove(members []Member, candidateID uint64) bool {
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

	// Health gate: every surviving voter must be healthy.
	if healthyVotersAfter != votersAfter {
		return false
	}

	// Quorum backstop: healthy survivors must still form a majority.
	quorumAfter := votersAfter/2 + 1
	return healthyVotersAfter >= quorumAfter
}

// AllMembersHealthy reports whether every name in required is present in members
// and healthy. It returns false when required is empty, since "all of nothing" is
// not a meaningful readiness signal for the callers that gate on it.
func AllMembersHealthy(members []Member, required map[string]bool) bool {
	if len(required) == 0 {
		return false
	}
	healthyByName := make(map[string]bool, len(members))
	for _, m := range members {
		if m.IsHealthy() {
			healthyByName[m.Name] = true
		}
	}
	for name := range required {
		if !healthyByName[name] {
			return false
		}
	}
	return true
}

// SurplusMemberNames returns the set of live member names that the desired state
// no longer wants, computed as (live members) minus expected. Using the live
// member list as the source of truth is more reliable than comparing StatefulSet
// replica counts alone: the anti-rejoin guard can leave the pod absent while the
// etcd member still exists in the cluster.
func SurplusMemberNames(members []Member, expected map[string]bool) map[string]bool {
	surplus := map[string]bool{}
	for _, m := range members {
		if !expected[m.Name] {
			surplus[m.Name] = true
		}
	}
	return surplus
}

// SelectNextRemovalCandidate returns the most suitable member to remove next from
// those whose name is in surplus, or nil when none of the live members is
// surplus. Ordering follows OrderRemovalCandidates (learners first, leader last).
func SelectNextRemovalCandidate(members []Member, surplus map[string]bool) *Member {
	candidates := make([]Member, 0, len(surplus))
	for _, m := range members {
		if surplus[m.Name] {
			candidates = append(candidates, m)
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	// OrderRemovalCandidates copies, so the returned pointer stays valid.
	ordered := OrderRemovalCandidates(candidates)
	return &ordered[0]
}

// OrderRemovalCandidates orders an already-selected candidate set into the
// sequence in which members should be removed: learners first, then non-leader
// voters, and the leader last. This ordering only sequences the removals within
// the selected set; it does not change which members are removed. Removing the
// leader last avoids an unnecessary leadership change mid-operation.
//
// The leader is derived from each Member's Role (MemberRoleLeader). Within a
// tier, members are ordered by member ID for determinism.
func OrderRemovalCandidates(candidates []Member) []Member {
	ordered := make([]Member, len(candidates))
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

	sort.SliceStable(ordered, func(i, j int) bool {
		ti, tj := tier(ordered[i]), tier(ordered[j])
		if ti != tj {
			return ti < tj
		}
		return ordered[i].ID < ordered[j].ID
	})
	return ordered
}
