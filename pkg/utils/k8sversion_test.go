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

var _ = Describe("K8S Version tests", func() {

	DescribeTable("compare versions",
		func(v1, op, v2 string, expectedResult, errorExpected bool) {
			comparisonResult, err := CompareVersions(v1, op, v2)
			Expect(comparisonResult).To(Equal(expectedResult))
			Expect(err != nil).To(Equal(errorExpected))
		},
		Entry("check valid versions having 'v' with greater than equals operator", "v1.22.3", ">=", "1.22.1", true, false),
		Entry("check valid versions with less than operator", "1.3.4", "<", "1.2.3", false, false),
		Entry("check with versions having '-' and 'v'", "v1.2.3-SNAPSHOT", "<=", "v1.3.1", true, false),
		Entry("check with version having '-', 'v' and '+'", "1.3.4", ">", "v1.2.2-alpha+3456a", true, false),
		Entry("check with invalid version", "vw1.22", ">", "1.22", false, true),
	)

})
