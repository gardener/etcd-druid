// Copyright (c) 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package utils

import (
	"fmt"
	"maps"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// MergeStringMaps merges the content of the newMaps with the oldMap. If a key already exists then
// it gets overwritten by the last value with the same key.
func MergeStringMaps(oldMap map[string]string, newMaps ...map[string]string) map[string]string {
	var out map[string]string

	if oldMap != nil {
		out = make(map[string]string)
	}
	for k, v := range oldMap {
		out[k] = v
	}

	for _, newMap := range newMaps {
		if newMap != nil && out == nil {
			out = make(map[string]string)
		}

		for k, v := range newMap {
			out[k] = v
		}
	}

	return out
}

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

func IsEmptyString(s string) bool {
	if len(strings.TrimSpace(s)) == 0 {
		return true
	}
	return false
}

// IfConditionOr implements a simple ternary operator, if the passed condition is true then trueVal is returned else falseVal is returned.
func IfConditionOr[T any](condition bool, trueVal, falseVal T) T {
	if condition {
		return trueVal
	}
	return falseVal
}

// IsNilOrEmptyStringPtr returns true if the string pointer is nil or the return value of IsEmptyString(s).
func IsNilOrEmptyStringPtr(s *string) bool {
	if s == nil {
		return true
	}
	return IsEmptyString(*s)
}
