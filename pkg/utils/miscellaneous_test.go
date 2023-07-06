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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Tests for miscellaneous utility functions", func() {
	Describe("#ComputeScheduleDuration", func() {
		It("should compute schedule duration as 24h when schedule set to 00:00 everyday", func() {
			schedule := "0 0 * * *"
			duration, err := ComputeScheduleDuration(schedule)
			Expect(err).To(Not(HaveOccurred()))
			Expect(duration).To(Equal(24 * time.Hour))
		})

		It("should compute schedule duration as 24h when schedule set to 15:30 everyday", func() {
			schedule := "30 15 * * *"
			duration, err := ComputeScheduleDuration(schedule)
			Expect(err).To(Not(HaveOccurred()))
			Expect(duration).To(Equal(24 * time.Hour))
		})

		It("should compute schedule duration as 12h when schedule set to 03:30 and 15:30 everyday", func() {
			schedule := "30 3,15 * * *"
			duration, err := ComputeScheduleDuration(schedule)
			Expect(err).To(Not(HaveOccurred()))
			Expect(duration).To(Equal(12 * time.Hour))
		})

		It("should compute schedule duration as 7d when schedule set to 15:30 on every Tuesday", func() {
			schedule := "30 15 * * 2"
			duration, err := ComputeScheduleDuration(schedule)
			Expect(err).To(Not(HaveOccurred()))
			Expect(duration).To(Equal(7 * 24 * time.Hour))
		})
	})
})
