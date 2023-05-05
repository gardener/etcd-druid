package etcd

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestEtcdController(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Etcd Controller Suite")
}
