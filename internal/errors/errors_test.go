// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package errors

import (
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/gomega"
)

func TestWrapError(t *testing.T) {
	err := WrapError(fmt.Errorf("testError"), "ERR_TEST", "testOp", "testMsg")

	g := NewWithT(t)
	druidErr, ok := err.(*DruidError)
	g.Expect(ok).To(BeTrue())
	g.Expect(string(druidErr.Code)).To(Equal("ERR_TEST"))
	g.Expect(druidErr.Operation).To(Equal("testOp"))
	g.Expect(druidErr.Message).To(Equal("testMsg"))
}

func TestIsRetriable(t *testing.T) {
	err := &DruidError{
		Code:       ErrRetriable,
		Cause:      nil,
		Operation:  "",
		Message:    "",
		ObservedAt: time.Now().UTC(),
	}

	g := NewWithT(t)
	g.Expect(IsRetriable(err)).To(BeTrue())

	err.Code = "ERR_TEST"
	g.Expect(IsRetriable(err)).To(BeFalse())
}

func TestMapToLastErrors(t *testing.T) {
	errs := []error{
		&DruidError{
			Code:       "ERR_TEST1",
			Cause:      fmt.Errorf("testError1"),
			Operation:  "testOp",
			Message:    "testMsg",
			ObservedAt: time.Now().UTC(),
		},
		&DruidError{
			Code:       "ERR_TEST2",
			Cause:      nil,
			Operation:  "testOp",
			Message:    "testMsg",
			ObservedAt: time.Now().UTC(),
		},
	}
	lastErrs := MapToLastErrors(errs)

	g := NewWithT(t)
	g.Expect(len(lastErrs)).To(Equal(2))
	g.Expect(string(lastErrs[0].Code)).To(Equal("ERR_TEST1"))
	g.Expect(lastErrs[0].Description).To(Equal("[Operation: testOp, Code: ERR_TEST1] message: testMsg, cause: testError1"))
	g.Expect(string(lastErrs[1].Code)).To(Equal("ERR_TEST2"))
	g.Expect(lastErrs[1].Description).To(Equal("[Operation: testOp, Code: ERR_TEST2] message: testMsg"))
}
