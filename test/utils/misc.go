// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	. "github.com/onsi/gomega"

	"os"

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
