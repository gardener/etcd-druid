// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"fmt"
	"testing"

	"github.com/onsi/gomega"
)

func GenerateTestNamespaceName(t *testing.T, prefix string) string {
	g := gomega.NewWithT(t)
	suffix := GenerateRandomAlphanumericString(g, 4)
	return fmt.Sprintf("%s-%s", prefix, suffix)
}
