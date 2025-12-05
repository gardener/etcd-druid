// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	druidapicommon "github.com/gardener/etcd-druid/api/common"

	. "github.com/onsi/gomega"
)

// CheckLastOperation checks the LastOperation field in the task status with the expected values.
func CheckLastOperation(g *WithT, actualOp *druidapicommon.LastOperation, expectedLastOperation *druidapicommon.LastOperation) {
	if expectedLastOperation != nil {
		g.Expect(actualOp).ToNot(BeNil())
		g.Expect(actualOp.Type).To(Equal(expectedLastOperation.Type))
		g.Expect(actualOp.State).To(Equal(expectedLastOperation.State))
		if expectedLastOperation.Description != "" {
			g.Expect(actualOp.Description).To(Equal(expectedLastOperation.Description))
		}
	}
}

// CheckLastErrors checks the LastErrors field in the task status with the expected values.
func CheckLastErrors(g *WithT, lastErrors []druidapicommon.LastError, expectedLastErrors *druidapicommon.LastError) {
	if expectedLastErrors != nil {
		index := len(lastErrors) - 1
		g.Expect(lastErrors).ToNot(BeNil())
		g.Expect(lastErrors[index].Code).To(Equal(expectedLastErrors.Code))
	} else {
		g.Expect(lastErrors).To(BeNil())
	}
}
