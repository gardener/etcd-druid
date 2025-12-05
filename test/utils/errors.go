// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"errors"

	druiderr "github.com/gardener/etcd-druid/internal/errors"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	. "github.com/onsi/gomega"
)

var (
	// TestInternalErr is a generic test internal server error used to construct TestAPIInternalErr.
	TestInternalErr = errors.New("fake get internal error")
	// TestAPIInternalErr is an API internal server error meant to be used to mimic HTTP response code 500 for tests.
	TestAPIInternalErr = apierrors.NewInternalError(TestInternalErr)
)

// CheckDruidError checks that an actual error is a DruidError and further checks its underline cause, error code and operation.
func CheckDruidError(g *WithT, expectedError *druiderr.DruidError, actualError error) {
	g.Expect(actualError).To(HaveOccurred())
	var druidErr *druiderr.DruidError
	g.Expect(errors.As(actualError, &druidErr)).To(BeTrue())
	g.Expect(druidErr.Code).To(Equal(expectedError.Code))
	g.Expect(errors.Is(druidErr.Cause, expectedError.Cause)).To(BeTrue())
	g.Expect(druidErr.Operation).To(Equal(expectedError.Operation))
}

// CheckDruidErrorList checks if the actual errors match the expected Druid errors by invoking CheckDruidError for each error.
func CheckDruidErrorList(g *WithT, actual, expected []error) {
	g.Expect(actual).To(HaveLen(len(expected)))
	for i, err := range actual {
		expectedErr := expected[i]
		var druidErr *druiderr.DruidError
		var expectedDruidErr *druiderr.DruidError
		if errors.As(err, &druidErr) && errors.As(expectedErr, &expectedDruidErr) {
			CheckDruidError(g, expectedDruidErr, err)
		}
	}
}
