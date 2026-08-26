// SPDX-FileCopyrightText: Copyright Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestControllerUtils(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Controller Utils suite")
}
