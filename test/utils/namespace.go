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
