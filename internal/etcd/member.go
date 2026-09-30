// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package etcd

import "slices"

// MemberRole is the role an etcd member plays in the cluster.
type MemberRole string

const (
	// MemberRoleLearner is a non-voting learner that does not participate in quorum.
	MemberRoleLearner MemberRole = "Learner"
	// MemberRoleMember is a voting, non-leader member.
	MemberRoleMember MemberRole = "Member"
	// MemberRoleLeader is the current cluster leader (also a voting member).
	MemberRoleLeader MemberRole = "Leader"
)

// MemberHealth is the observed health of an etcd member, derived from a live
// Status probe. Only Healthy is treated as healthy; both Unhealthy and Unknown
// (probe error/timeout) are fail-closed as not healthy.
type MemberHealth string

const (
	// MemberHealthHealthy indicates the member's Status probe succeeded.
	MemberHealthHealthy MemberHealth = "Healthy"
	// MemberHealthUnhealthy indicates the member is known to be unhealthy.
	MemberHealthUnhealthy MemberHealth = "Unhealthy"
	// MemberHealthUnknown indicates the member's Status probe errored or timed
	// out. It is fail-closed: never treated as healthy.
	MemberHealthUnknown MemberHealth = "Unknown"
)

// Member is a minimal, client-library-agnostic view of an etcd cluster member.
// It lives in this pure package (free of any etcd client dependency) so that the
// membership selection logic and the client wrapper in internal/client/etcd can
// both refer to it without the selection logic pulling in the etcd client
// library.
type Member struct {
	// ID is the etcd member ID.
	ID uint64
	// Name is the etcd member name (matches the pod name).
	Name string
	// Role is the member's role (learner, voting member, or leader), derived
	// from the live MemberList and Status RPCs.
	Role MemberRole
	// Health is the member's observed health, derived from a live Status probe.
	Health MemberHealth
	// ClientURLs are the member's advertised client URLs from the MemberList RPC.
	ClientURLs []string
}

// IsLearner reports whether the member is a non-voting learner.
func (m Member) IsLearner() bool { return m.Role == MemberRoleLearner }

// IsVoter reports whether the member participates in quorum (a voting member or
// the leader).
func (m Member) IsVoter() bool {
	return m.Role == MemberRoleMember || m.Role == MemberRoleLeader
}

// IsHealthy reports whether the member is healthy. Only MemberHealthHealthy is
// considered healthy; Unhealthy and Unknown are not.
func (m Member) IsHealthy() bool { return m.Health == MemberHealthHealthy }

// MemberNames is a list of etcd member names.
type MemberNames []string

// Has reports whether name is in the list.
func (n MemberNames) Has(name string) bool { return slices.Contains(n, name) }

// Members is a list of etcd cluster members.
type Members []Member

// AllHealthy reports whether every member in ms is healthy. It returns true for
// an empty list.
func (ms Members) AllHealthy() bool {
	for _, m := range ms {
		if !m.IsHealthy() {
			return false
		}
	}
	return true
}

// Split returns the members whose name is in names and the members whose name
// is not. Both results keep the order of ms.
func (ms Members) Split(names MemberNames) (in, out Members) {
	for _, m := range ms {
		if names.Has(m.Name) {
			in = append(in, m)
		} else {
			out = append(out, m)
		}
	}
	return in, out
}
