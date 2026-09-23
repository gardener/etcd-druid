// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package etcd

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
