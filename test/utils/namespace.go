// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"fmt"
	"testing"

	"github.com/onsi/gomega"
)

func GenerateTestNamespaceName(t *testing.T, prefix string, suffixLength int) string {
	g := gomega.NewWithT(t)
	suffix := GenerateRandomAlphanumericString(g, suffixLength/2) // Each byte is represented by two hex characters
	return fmt.Sprintf("%s-%s", prefix, suffix)
}
