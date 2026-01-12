// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"crypto/rand"
	"encoding/hex"
	"maps"

	gomegatypes "github.com/onsi/gomega/types"
	"k8s.io/apimachinery/pkg/api/resource"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
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

func stringIdentifier(element any) string {
	return element.(string)
}

func ParseQuantity(q string) resource.Quantity {
	val, _ := resource.ParseQuantity(q)
	return val
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

// GenerateRandomAlphanumericString generates a random alphanumeric string of the given length.
func GenerateRandomAlphanumericString(g *WithT, length int) string {
	b := make([]byte, length)
	_, err := rand.Read(b)
	g.Expect(err).ToNot(HaveOccurred())
	return hex.EncodeToString(b)
}
