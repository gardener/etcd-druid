// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package etcd

import (
	"cmp"
	"slices"
)

// QuorumSafeToRemove reports whether removing the member identified by
// candidateID is safe with respect to quorum. Health is read from each
// Member's embedded Health field, populated from a live etcd Status probe.
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
func QuorumSafeToRemove(members []Member, candidateID uint64) bool {
	var candidate *Member
	for i := range members {
		if members[i].ID == candidateID {
			candidate = &members[i]
			break
		}
	}
	// Fail closed -- unknown candidate ID means nothing to remove from our view.
	if candidate == nil {
		return false
	}
	if candidate.IsLearner() {
		return true
	}

	votersAfter := 0
	healthyVotersAfter := 0
	for _, m := range members {
		// Exclude the candidate from the survivor set regardless of its health.
		if m.ID == candidateID || !m.IsVoter() {
			continue
		}
		votersAfter++
		if m.IsHealthy() {
			healthyVotersAfter++
		}
	}

	// Removing the last voter would leave no cluster; treat as unsafe here and
	// let the replicas -> 0 path (a Non-Goal of scale-in) handle teardown.
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

// OrderRemovalCandidates orders an already-selected candidate set into the
// sequence in which members should be removed: learners first, then non-leader
// voters, and the leader last. Within each tier members are ordered by ascending
// member ID for determinism. Removing the leader last avoids an unnecessary
// leadership change mid-operation. The within-tier ordering is cosmetic --
// quorum safety is checked before every individual removal, and all surplus
// members are removed before the StatefulSet replicas field is updated.
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

	slices.SortFunc(ordered, func(a, b Member) int {
		if c := cmp.Compare(tier(a), tier(b)); c != 0 {
			return c
		}
		return cmp.Compare(a.ID, b.ID)
	})
	return ordered
}
