// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Tests for miscellaneous utility functions", func() {
	Describe("#ComputeScheduleInterval", func() {
		It("should compute schedule duration as 24h when schedule set to 00:00 everyday", func() {
			schedule := "0 0 * * *"
			duration, err := ComputeScheduleInterval(schedule)
			Expect(err).To(Not(HaveOccurred()))
			Expect(duration).To(Equal(24 * time.Hour))
		})

		It("should compute schedule duration as 24h when schedule set to 15:30 everyday", func() {
			schedule := "30 15 * * *"
			duration, err := ComputeScheduleInterval(schedule)
			Expect(err).To(Not(HaveOccurred()))
			Expect(duration).To(Equal(24 * time.Hour))
		})

		It("should compute schedule duration as 12h when schedule set to 03:30 and 15:30 everyday", func() {
			schedule := "30 3,15 * * *"
			duration, err := ComputeScheduleInterval(schedule)
			Expect(err).To(Not(HaveOccurred()))
			Expect(duration).To(Equal(12 * time.Hour))
		})

		It("should compute schedule duration as 7d when schedule set to 15:30 on every Tuesday", func() {
			schedule := "30 15 * * 2"
			duration, err := ComputeScheduleInterval(schedule)
			Expect(err).To(Not(HaveOccurred()))
			Expect(duration).To(Equal(7 * 24 * time.Hour))
		})
	})
})
