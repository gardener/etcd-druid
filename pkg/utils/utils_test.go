package utils_test

import (
	. "github.com/gardener/etcd-druid/pkg/utils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Utils", func() {
	Describe("#CompareVersions", func() {
		Context("Comparing between versions 1.20.0 and 1.25.0", func() {
			It("Should return true if the constraint is 1.20.0 < 1.25.0", func() {
				result, err := CompareVersions("1.20.0", "<", "1.25.0")
				Expect(err).To(BeNil())
				Expect(result).To(BeTrue())
			})
			It("Should return true if the constraint is 1.25.0 > 1.20.0", func() {
				result, err := CompareVersions("1.25.0", ">", "1.20.0")
				Expect(err).To(BeNil())
				Expect(result).To(BeTrue())
			})
			It("Should return false if the constraint is 1.20.0 > 1.25.0", func() {
				result, err := CompareVersions("1.20.0", ">", "1.25.0")
				Expect(err).To(BeNil())
				Expect(result).To(BeFalse())
			})
			It("Should return false if the constraint is 1.20.0 = 1.25.0", func() {
				result, err := CompareVersions("1.20.0", "=", "1.25.0")
				Expect(err).To(BeNil())
				Expect(result).To(BeFalse())
			})
		})
		Context("Comparing between versions 1.20.0-dev and 1.25.0-dev", func() {
			It("Should return true if the constraint is 1.20.0-dev < 1.25.0-dev", func() {
				result, err := CompareVersions("1.20.0-dev", "<", "1.25.0-dev")
				Expect(err).To(BeNil())
				Expect(result).To(BeTrue())
			})
		})
	})
	Describe("#CheckVersionMeetsConstraint", func() {
		Context("COmparing between versions 1.21.0 and 1.21.1", func() {
			It("Should return false if the constraint is 1.21.0 > 1.21.1", func() {
				result, err := CheckVersionMeetsConstraint("1.21.0", "> 1.21.1")
				Expect(err).To(BeNil())
				Expect(result).To(BeFalse())
			})
			It("Should return true if the constraint is 1.21.0 < 1.21.1", func() {
				result, err := CheckVersionMeetsConstraint("1.21.0", "< 1.21.1")
				Expect(err).To(BeNil())
				Expect(result).To(BeTrue())
			})
			It("Should return false if the constraint is 1.21.0 = 1.21.1", func() {
				result, err := CheckVersionMeetsConstraint("1.21.0", "= 1.21.1")
				Expect(err).To(BeNil())
				Expect(result).To(BeFalse())
			})
		})
	})
})
