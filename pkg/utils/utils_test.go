// Copyright 2023 SAP SE or an SAP affiliate company
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

package utils_test

import (
	. "github.com/gardener/etcd-druid/pkg/utils"
	. "github.com/onsi/ginkgo/v2"
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
		Context("Comparing between versions 1.21.0 and 1.21.1", func() {
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
