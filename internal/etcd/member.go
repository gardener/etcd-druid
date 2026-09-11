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

// MemberHealth is the observed health of an etcd member derived from a live
// Status probe. Unknown (probe error/timeout) is fail-closed: never treated as healthy.
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

// Member is a minimal view of an etcd cluster member.
type Member struct {
	ID     uint64
	Name   string
	Role   MemberRole
	Health MemberHealth
}

// IsLearner reports whether the member is a non-voting learner.
func (m Member) IsLearner() bool { return m.Role == MemberRoleLearner }

// IsVoter reports whether the member participates in quorum.
func (m Member) IsVoter() bool {
	return m.Role == MemberRoleMember || m.Role == MemberRoleLeader
}

// IsHealthy reports whether the member is healthy.
func (m Member) IsHealthy() bool { return m.Health == MemberHealthHealthy }
