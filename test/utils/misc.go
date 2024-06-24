// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"maps"
	"os"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	gomegatypes "github.com/onsi/gomega/types"
	"k8s.io/apimachinery/pkg/api/resource"
)

func MatchFinalizer(finalizer string) gomegatypes.GomegaMatcher {
	return MatchFields(IgnoreExtras, Fields{
		"ObjectMeta": MatchFields(IgnoreExtras, Fields{
			"Finalizers": MatchAllElements(stringIdentifier, Elements{
				finalizer: Equal(finalizer),
			}),
		}),
	})
}

func stringIdentifier(element interface{}) string {
	return element.(string)
}

func ParseQuantity(q string) resource.Quantity {
	val, _ := resource.ParseQuantity(q)
	return val
}

// SwitchDirectory sets the working directory and returns a function to revert to the previous one.
func SwitchDirectory(path string) func() {
	oldPath, err := os.Getwd()
	if err != nil {
		Expect(err).NotTo(HaveOccurred())
	}

	if err := os.Chdir(path); err != nil {
		Expect(err).NotTo(HaveOccurred())
	}

	return func() {
		if err := os.Chdir(oldPath); err != nil {
			Expect(err).NotTo(HaveOccurred())
		}
	}
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

// TypeDeref dereferences a pointer to a type if it is not nil, else it returns the default value.
func TypeDeref[T any](val *T, defaultVal T) T {
	if val != nil {
		return *val
	}
	return defaultVal
}
