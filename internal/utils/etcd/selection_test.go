// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package etcd

import (
	"testing"

	. "github.com/onsi/gomega"
)

func healthyAll(members []Member) map[uint64]bool {
	h := make(map[uint64]bool, len(members))
	for _, m := range members {
		h[m.ID] = true
	}
	return h
}

func TestQuorumSafeToRemove(t *testing.T) {
	tests := []struct {
		name      string
		members   []Member
		candidate Member
		// unhealthy lists IDs that should be marked NOT healthy in the CR status.
		unhealthy []uint64
		// knownIDs overrides the set of IDs considered known to the Etcd status.
		// When nil, all member IDs are treated as known (normal scale-in scenario).
		knownIDs map[uint64]bool
		want     bool
	}{
		{
			name: "5 healthy voters, remove one -> 4 remain, quorum 3, safe",
			members: []Member{
				{ID: 1, Name: "e-0"}, {ID: 2, Name: "e-1"}, {ID: 3, Name: "e-2"},
				{ID: 4, Name: "e-3"}, {ID: 5, Name: "e-4"},
			},
			candidate: Member{ID: 5, Name: "e-4"},
			want:      true,
		},
		{
			name: "3 healthy voters, remove one -> 2 remain, quorum 2, safe",
			members: []Member{
				{ID: 1, Name: "e-0"}, {ID: 2, Name: "e-1"}, {ID: 3, Name: "e-2"},
			},
			candidate: Member{ID: 3, Name: "e-2"},
			want:      true,
		},
		{
			name: "3 voters but one already unhealthy, removing a healthy one breaks quorum",
			members: []Member{
				{ID: 1, Name: "e-0"}, {ID: 2, Name: "e-1"}, {ID: 3, Name: "e-2"},
			},
			candidate: Member{ID: 2, Name: "e-1"},
			unhealthy: []uint64{3}, // after removing 2: voters {1,3}, healthy {1} -> quorum 2, unsafe
			want:      false,
		},
		{
			name: "removing the already-unhealthy voter is safe (it was not in quorum)",
			members: []Member{
				{ID: 1, Name: "e-0"}, {ID: 2, Name: "e-1"}, {ID: 3, Name: "e-2"},
			},
			candidate: Member{ID: 3, Name: "e-2"},
			unhealthy: []uint64{3}, // after removing 3: voters {1,2}, healthy {1,2} -> quorum 2, safe
			want:      true,
		},
		{
			name: "removing a learner is always safe regardless of voter health",
			members: []Member{
				{ID: 1, Name: "e-0"}, {ID: 2, Name: "e-1"}, {ID: 3, Name: "e-2"},
				{ID: 9, Name: "e-3", IsLearner: true},
			},
			candidate: Member{ID: 9, Name: "e-3", IsLearner: true},
			unhealthy: []uint64{2, 3}, // voters unhealthy, but learner removal never touches quorum
			want:      true,
		},
		{
			name: "removing the last voter is unsafe",
			members: []Member{
				{ID: 1, Name: "e-0"},
			},
			candidate: Member{ID: 1, Name: "e-0"},
			want:      false,
		},
		{
			name: "bootstrap removal: source members not in status (unknown) treated as healthy",
			// 4 source voters (IDs 1-4, not tracked by target CR status) + 3 target voters (IDs 10-12, all known+healthy).
			// Removing source member 4: votersAfter=6, healthyVotersAfter=6 (3 unknown=healthy + 3 known healthy), quorum=4, safe.
			members: []Member{
				{ID: 1, Name: "src-0"}, {ID: 2, Name: "src-1"}, {ID: 3, Name: "src-2"}, {ID: 4, Name: "src-3"},
				{ID: 10, Name: "tgt-0"}, {ID: 11, Name: "tgt-1"}, {ID: 12, Name: "tgt-2"},
			},
			candidate: Member{ID: 4, Name: "src-3"},
			// Only target members are known to the Etcd status; source members are absent.
			knownIDs: map[uint64]bool{10: true, 11: true, 12: true},
			want:     true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			known := tc.knownIDs
			if known == nil {
				known = healthyAll(tc.members)
			}
			healthy := healthyAll(tc.members)
			for _, id := range tc.unhealthy {
				delete(healthy, id)
			}
			g.Expect(QuorumSafeToRemove(tc.members, tc.candidate, healthy, known)).To(Equal(tc.want))
		})
	}
}

func TestOrderRemovalCandidates(t *testing.T) {
	tests := []struct {
		name       string
		candidates []Member
		leaderID   uint64
		wantIDs    []uint64
	}{
		{
			name: "learners first, then voters by descending ordinal, leader last",
			candidates: []Member{
				{ID: 1, Name: "etcd-main-0"},                  // voter, leader
				{ID: 2, Name: "etcd-main-3"},                  // voter
				{ID: 3, Name: "etcd-main-4", IsLearner: true}, // learner
				{ID: 4, Name: "etcd-main-2"},                  // voter
			},
			leaderID: 1,
			// learner(3) first; voters by desc ordinal: 3 -> id2, 2 -> id4; leader id1 last
			wantIDs: []uint64{3, 2, 4, 1},
		},
		{
			name: "no leader in set: voters ordered by descending ordinal",
			candidates: []Member{
				{ID: 10, Name: "etcd-main-2"},
				{ID: 11, Name: "etcd-main-4"},
				{ID: 12, Name: "etcd-main-3"},
			},
			leaderID: 99, // not in set
			wantIDs:  []uint64{11, 12, 10},
		},
		{
			name: "names without derivable ordinal fall back to member ID order",
			candidates: []Member{
				{ID: 30, Name: "weird"},
				{ID: 20, Name: "alsoweird"},
			},
			leaderID: 0,
			wantIDs:  []uint64{20, 30},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			ordered := OrderRemovalCandidates(tc.candidates, tc.leaderID)
			gotIDs := make([]uint64, len(ordered))
			for i, m := range ordered {
				gotIDs[i] = m.ID
			}
			g.Expect(gotIDs).To(Equal(tc.wantIDs))
		})
	}
}

func TestPodOrdinalFromName(t *testing.T) {
	tests := []struct {
		name   string
		in     string
		want   int
		wantOK bool
	}{
		{name: "simple", in: "etcd-main-2", want: 2, wantOK: true},
		{name: "multi-digit", in: "etcd-main-13", want: 13, wantOK: true},
		{name: "prefix with dashes", in: "my-etcd-cluster-7", want: 7, wantOK: true},
		{name: "no trailing ordinal", in: "etcd-main-", want: 0, wantOK: false},
		{name: "non-numeric suffix", in: "etcd-main-abc", want: 0, wantOK: false},
		{name: "no dash", in: "etcd", want: 0, wantOK: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			got, ok := podOrdinalFromName(tc.in)
			g.Expect(ok).To(Equal(tc.wantOK))
			g.Expect(got).To(Equal(tc.want))
		})
	}
}
