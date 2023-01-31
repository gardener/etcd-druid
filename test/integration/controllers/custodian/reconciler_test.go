package custodian

import (
	componentpdb "github.com/gardener/etcd-druid/pkg/component/etcd/poddisruptionbudget"
	testutils "github.com/gardener/etcd-druid/test/utils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Custodian Controller", func() {
	Context("minAvailable of PodDisruptionBudget", func() {
		When("having a single node cluster", func() {
			etcd := testutils.EtcdBuilderWithDefaults("test", "default").WithReadyStatus().Build()

			Expect(len(etcd.Status.Members)).To(BeEquivalentTo(1))
			Expect(*etcd.Status.ClusterSize).To(BeEquivalentTo(1))

			It("should be set to 0", func() {
				etcd.Spec.Replicas = 1
				Expect(componentpdb.CalculatePDBMinAvailable(etcd)).To(BeEquivalentTo(0))
				etcd.Spec.Replicas = 0
				Expect(componentpdb.CalculatePDBMinAvailable(etcd)).To(BeEquivalentTo(0))
			})
		})

		When("clusterSize is nil", func() {
			etcd := testutils.EtcdBuilderWithDefaults("test", "default").WithReplicas(3).WithReadyStatus().Build()
			etcd.Status.ClusterSize = nil

			It("should be set to quorum size", func() {
				Expect(componentpdb.CalculatePDBMinAvailable(etcd)).To(BeEquivalentTo(2))
			})
		})

		When("having a multi node cluster", func() {
			etcd := testutils.EtcdBuilderWithDefaults("test", "default").WithReplicas(5).WithReadyStatus().Build()

			Expect(len(etcd.Status.Members)).To(BeEquivalentTo(5))
			Expect(*etcd.Status.ClusterSize).To(BeEquivalentTo(5))

			It("should calculate the value correctly", func() {
				Expect(componentpdb.CalculatePDBMinAvailable(etcd)).To(BeEquivalentTo(3))
			})
		})
	})
})
