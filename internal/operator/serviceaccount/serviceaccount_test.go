package serviceaccount

import (
	"testing"

	. "github.com/onsi/gomega"
)

func DummyTest(t *testing.T) {
	g := NewWithT(t)
	g.Expect("bingo").To(Equal("bingo"))
}
