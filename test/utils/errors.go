package utils

import (
	"errors"

	druiderr "github.com/gardener/etcd-druid/internal/errors"
	. "github.com/onsi/gomega"
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
