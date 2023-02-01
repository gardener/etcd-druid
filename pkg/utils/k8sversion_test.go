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
