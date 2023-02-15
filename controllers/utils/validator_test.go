// Copyright 2023 SAP SE or an SAP affiliate company
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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

})
