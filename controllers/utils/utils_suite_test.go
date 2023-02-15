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
