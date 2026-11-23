// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package etcd

import "fmt"

// CanRemoveMemberWithoutLosingQuorum reports whether removing the member
// identified by candidateID keeps a healthy voting quorum, using the health
// embedded in each Member (populated from a live etcd Status probe).
func CanRemoveMemberWithoutLosingQuorum(members Members, candidateID uint64) (bool, error) {
	candidate, found := memberByID(members, candidateID)
	if !found {
		return false, fmt.Errorf("member %x not found in membership", candidateID)
	}
	if candidate.IsLearner() {
		return true, nil
	}

	remainingVoters := 0
	healthyRemainingVoters := 0
	for _, m := range members {
		if m.ID == candidateID || !m.IsVoter() {
			continue
		}
		remainingVoters++
		if m.IsHealthy() {
			healthyRemainingVoters++
		}
	}
	if remainingVoters == 0 {
		// Removing the last voter would leave no cluster.
		return false, nil
	}
	return healthyRemainingVoters >= remainingVoters/2+1, nil
}

// CanRestartMemberWithoutLosingQuorum reports whether taking the member identified
// by candidateID down keeps a healthy voting quorum. A restarted member stays
// registered, so quorum is counted over all voters; scale-in permanently removes
// the member and uses CanRemoveMemberWithoutLosingQuorum instead.
func CanRestartMemberWithoutLosingQuorum(members Members, candidateID uint64) (bool, error) {
	candidate, found := memberByID(members, candidateID)
	if !found {
		return false, fmt.Errorf("member %x not found in membership", candidateID)
	}
	if candidate.IsLearner() {
		return true, nil
	}

	voters := 0
	healthySurvivingVoters := 0
	for _, m := range members {
		if !m.IsVoter() {
			continue
		}
		voters++
		if m.ID != candidateID && m.IsHealthy() {
			healthySurvivingVoters++
		}
	}
	return healthySurvivingVoters >= voters/2+1, nil
}

// memberByID returns the member with the given ID and whether it was found.
func memberByID(members Members, id uint64) (Member, bool) {
	for _, m := range members {
		if m.ID == id {
			return m, true
		}
	}
	return Member{}, false
}
