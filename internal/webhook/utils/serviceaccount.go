// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import "strings"

const (
	// ServiceAccountUsernamePrefix is the service account username prefix
	ServiceAccountUsernamePrefix = "system:serviceaccount:"
	// ServiceAccountUsernameSeparator is the separator used in service account username.
	ServiceAccountUsernameSeparator = ":"
)

// ServiceAccountMatchesUsername checks whether the provided username matches the namespace and name
// Use this when checking a service account namespace and name against a known string.
func ServiceAccountMatchesUsername(namespace, name, username string) bool {
	if !strings.HasPrefix(username, ServiceAccountUsernamePrefix) {
		return false
	}
	username = username[len(ServiceAccountUsernamePrefix):]

	if !strings.HasPrefix(username, namespace) {
		return false
	}
	username = username[len(namespace):]

	if !strings.HasPrefix(username, ServiceAccountUsernameSeparator) {
		return false
	}
	username = username[len(ServiceAccountUsernameSeparator):]

	return username == name
}
