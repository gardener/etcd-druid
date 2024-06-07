// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package version

var (
	// These variables are typically populated using -ldflags settings when building the binary

	// Version stores the etcd-druid binary version.
	Version string
	// GitSHA stores the etcd-druid binary code commit SHA on git.
	GitSHA string
)
