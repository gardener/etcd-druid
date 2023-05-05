package etcd

import (
	"github.com/gardener/etcd-druid/test/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("EtcdController", func() {
	const (
		testEtcdName  = "etcd-test"
		testNamespace = "test-ns"
	)
	var (
		etcd = utils.EtcdBuilderWithDefaults(testEtcdName, testNamespace).Build()
	)

	DescribeTable("etcdInBootstrap tests",
		func(specReplicas, statusReplicas int, expectedResult bool) {
			etcd.Spec.Replicas = int32(specReplicas)
			etcd.Status.Replicas = int32(statusReplicas)
			Expect(clusterInBootstrap(etcd)).To(Equal(expectedResult))
		},
		Entry("check bootstrap of a single node cluster", 1, 0, true),
		Entry("check non-bootstrap case for a single node cluster", 1, 1, false),
		Entry("check bootstrap for a multi-node cluster", 3, 0, true),
		Entry("check non-bootstrap case for multi-node cluster", 3, 2, false),
	)
})
