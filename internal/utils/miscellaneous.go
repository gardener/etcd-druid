// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"fmt"
	"maps"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// MergeMaps merges the contents of maps. All maps will be processed in the order
// in which they are sent. For overlapping keys across source maps, value in the merged map
// for this key will be from the last occurrence of the key-value.
func MergeMaps[K comparable, V any](sourceMaps ...map[K]V) map[K]V {
	if sourceMaps == nil {
		return nil
	}
	merged := make(map[K]V)
	for _, m := range sourceMaps {
		maps.Copy(merged, m)
	}
	return merged
}

func nameAndNamespace(namespaceOrName string, nameOpt ...string) (namespace, name string) {
	if len(nameOpt) > 1 {
		panic(fmt.Sprintf("more than name/namespace for key specified: %s/%v", namespaceOrName, nameOpt))
	}
	if len(nameOpt) == 0 {
		name = namespaceOrName
		return
	}
	namespace = namespaceOrName
	name = nameOpt[0]
	return
}

// Key creates a new client.ObjectKey from the given parameters.
// There are only two ways to call this function:
//   - If only namespaceOrName is set, then a client.ObjectKey with name set to namespaceOrName is returned.
//   - If namespaceOrName and one nameOpt is given, then a client.ObjectKey with namespace set to namespaceOrName
//     and name set to nameOpt[0] is returned.
//
// For all other cases, this method panics.
func Key(namespaceOrName string, nameOpt ...string) client.ObjectKey {
	namespace, name := nameAndNamespace(namespaceOrName, nameOpt...)
	return client.ObjectKey{Namespace: namespace, Name: name}
}

// TypeDeref dereferences a pointer to a type if it is not nil, else it returns the default value.
func TypeDeref[T any](val *T, defaultVal T) T {
	if val != nil {
		return *val
	}
	return defaultVal
}

// IsEmptyString returns true if the string is empty or contains only whitespace characters.
func IsEmptyString(s string) bool {
	return len(strings.TrimSpace(s)) == 0
}

// IfConditionOr implements a simple ternary operator, if the passed condition is true then trueVal is returned else falseVal is returned.
func IfConditionOr[T any](condition bool, trueVal, falseVal T) T {
	if condition {
		return trueVal
	}
	return falseVal
}

// PointerOf returns a pointer to the given value.
func PointerOf[T any](val T) *T {
	return &val
}
