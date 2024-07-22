// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package errors

import (
	"errors"
	"fmt"
	"testing"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	. "github.com/onsi/gomega"
)

func TestWrapError(t *testing.T) {
	err := WrapError(fmt.Errorf("testError"), "ERR_TEST", "testOp", "testMsg")
	g := NewWithT(t)
	druidErr := &DruidError{}
	g.Expect(errors.As(err, &druidErr)).To(BeTrue())
	g.Expect(string(druidErr.Code)).To(Equal("ERR_TEST"))
	g.Expect(druidErr.Operation).To(Equal("testOp"))
	g.Expect(druidErr.Message).To(Equal("testMsg"))
}

func TestIsRequeueAfterError(t *testing.T) {
	testCases := []struct {
		name     string
		code     druidv1alpha1.ErrorCode
		expected bool
	}{
		{
			name:     "error has code ERR_REQUEUE_AFTER",
			code:     ErrRequeueAfter,
			expected: true,
		},
		{
			name:     "error has code ERR_TEST",
			code:     druidv1alpha1.ErrorCode("ERR_TEST"),
			expected: false,
		},
	}

	g := NewWithT(t)
	t.Parallel()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := &DruidError{
				Code:       tc.code,
				ObservedAt: time.Now().UTC(),
			}
			g.Expect(IsRequeueAfterError(err)).To(Equal(tc.expected))

		})
	}
}

func TestMapToLastErrors(t *testing.T) {
	errs := []error{
		&DruidError{
			Code:       druidv1alpha1.ErrorCode("ERR_TEST1"),
			Cause:      fmt.Errorf("testError1"),
			Operation:  "testOp",
			Message:    "testMsg",
			ObservedAt: time.Now().UTC(),
		},
		&DruidError{
			Code:       druidv1alpha1.ErrorCode("ERR_TEST2"),
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
