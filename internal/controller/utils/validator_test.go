// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Validator Tests", func() {

	DescribeTable("#ShouldBeOneOfAllowedValues tests",
		func(key string, allowedValues []string, valueToCheck string, expectError bool) {
			err := ShouldBeOneOfAllowedValues(key, allowedValues, valueToCheck)
			Expect(err != nil).To(Equal(expectError))
		},
		Entry("value is not present in the allowed values slice", "locknames", []string{"configmaps"}, "leases", true),
		Entry("values is present in the allowed values slice", "locknames", []string{"configmaps", "leases", "endpoints"}, "leases", false),
	)

	DescribeTable("#MustBeGreaterThan",
		func(key string, lowerBound int, value int, expectError bool) {
			err := MustBeGreaterThan(key, lowerBound, value)
			Expect(err != nil).To(Equal(expectError))
		},
		Entry("value is not greater than the lower bound", "workers", 0, -1, true),
		Entry("value is greater than the lower bound", "durationSeconds", 10, 20, false),
		Entry("value is equal to the lower bound", "threshold", 10, 10, true),
	)

	DescribeTable("#MustBeGreaterThanOrEqualTo",
		func(key string, lowerBound int, value int, expectError bool) {
			err := MustBeGreaterThanOrEqualTo(key, lowerBound, value)
			Expect(err != nil).To(Equal(expectError))
		},
		Entry("value is not greater than the lower bound", "workers", 0, -1, true),
		Entry("value is greater than the lower bound", "durationSeconds", 10, 20, false),
		Entry("value is equal to the lower bound", "threshold", 10, 10, false),
	)

})
