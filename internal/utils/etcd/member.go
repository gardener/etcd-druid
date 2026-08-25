// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package etcd

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
	// IsLearner is true if the member is a non-voting learner.
	IsLearner bool
}
