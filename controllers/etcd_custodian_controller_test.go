// Copyright (c) 2021 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package controllers

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
