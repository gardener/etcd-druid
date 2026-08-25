// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package etcd

import "sort"

// QuorumSafeToRemove reports whether removing exactly one member is safe with
// respect to quorum, given the current members, the set of member IDs
// currently considered healthy by the Etcd status, and the set of all member
// IDs tracked by the Etcd status.
//
// etcd quorum is computed over voting members only (learners do not count).
// Removing a member is quorum-safe when, after the removal, a majority of the
// remaining voting members are still healthy:
//
//	healthyVotersAfter >= (votersAfter/2)+1
//
// Removing a learner never affects quorum, so it is always safe.
//
// Two independent signals determine whether a remaining voter is counted as
// healthy:
//  1. Unknown to this CR's status (foreign cluster member, e.g. source during
//     bootstrap members removal) — treated as healthy because we have no basis
//     to consider it unhealthy.
//  2. Present in healthyIDs — the CR status recorded it as Ready.
//
// etcd itself also enforces a quorum check on MemberRemove; this pre-check lets
// the controller avoid issuing a removal that etcd would reject and surface a
// clear reason instead.
func QuorumSafeToRemove(members []Member, candidate Member, healthyIDs, knownIDs map[uint64]bool) bool {
	if candidate.IsLearner {
		return true
	}

	votersAfter := 0
	healthyVotersAfter := 0
	for _, m := range members {
		if m.ID == candidate.ID || m.IsLearner {
			continue
		}
		votersAfter++
		// A member absent from knownIDs belongs to a foreign cluster; treat as
		// healthy. A known member is healthy if the CR status says so.
		if !knownIDs[m.ID] || healthyIDs[m.ID] {
			healthyVotersAfter++
		}
	}

	// Removing the last voter would leave no cluster; treat as unsafe here and
	// let the replicas -> 0 path (a Non-Goal of scale-in) handle teardown.
	if votersAfter == 0 {
		return false
	}

	quorumAfter := votersAfter/2 + 1
	return healthyVotersAfter >= quorumAfter
}

// OrderRemovalCandidates orders an already-selected candidate set into the
// sequence in which members should be removed: learners first, then non-leader
// voters, and the leader last. This ordering only sequences the removals within
// the selected set — it does not change which members are removed. Removing the
// leader last avoids an unnecessary leadership change mid-operation.
//
// leaderID is the current leader's member ID (0 if unknown). Within each tier,
// members are ordered by descending pod ordinal where derivable, so that higher
// ordinals — the ones a replicas-driven scale-in targets — are removed first;
// ties fall back to member ID for determinism.
func OrderRemovalCandidates(candidates []Member, leaderID uint64) []Member {
	ordered := make([]Member, len(candidates))
	copy(ordered, candidates)

	tier := func(m Member) int {
		switch {
		case m.IsLearner:
			return 0
		case m.ID == leaderID:
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
		// Within a tier, prefer removing higher pod ordinals first; fall back to
		// member ID when an ordinal cannot be derived from the name.
		oi, iok := podOrdinalFromName(ordered[i].Name)
		oj, jok := podOrdinalFromName(ordered[j].Name)
		if iok && jok && oi != oj {
			return oi > oj
		}
		return ordered[i].ID < ordered[j].ID
	})
	return ordered
}

// podOrdinalFromName extracts the trailing StatefulSet pod ordinal from a member
// name of the form "<prefix>-<ordinal>" (e.g. "etcd-main-2" -> 2). It returns
// (ordinal, true) on success and (0, false) if no trailing integer is present.
func podOrdinalFromName(name string) (int, bool) {
	lastDash := -1
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '-' {
			lastDash = i
			break
		}
	}
	if lastDash < 0 || lastDash == len(name)-1 {
		return 0, false
	}
	ordinal := 0
	for _, r := range name[lastDash+1:] {
		if r < '0' || r > '9' {
			return 0, false
		}
		ordinal = ordinal*10 + int(r-'0')
	}
	return ordinal, true
}
