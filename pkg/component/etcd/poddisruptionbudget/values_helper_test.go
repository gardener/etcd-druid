// Copyright (c) 2022 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package poddisruptionbudget_test

import (
	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	. "github.com/gardener/etcd-druid/pkg/component/etcd/poddisruptionbudget"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("PodDisruptionBudget", func() {
	var (
		etcd *druidv1alpha1.Etcd
	)

	BeforeEach(func() {
		etcd = &druidv1alpha1.Etcd{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "etcd",
				Namespace: "default",
			},
			Spec: druidv1alpha1.EtcdSpec{
				Replicas: 3,
			},
		}
	})
	Describe("#CalculatePDBMinAvailable", func() {
		Context("With etcd replicas less than 2", func() {
			It("Should return MinAvailable 0 if replicas is set to 1", func() {
				etcd.Spec.Replicas = 1
				Expect(CalculatePDBMinAvailable(etcd)).To(Equal(0))
			})
		})
		Context("With etcd replicas more than 2", func() {
			It("Should return MinAvailable equal to quorum size of cluster", func() {
				etcd.Spec.Replicas = 5
				Expect(CalculatePDBMinAvailable(etcd)).To(Equal(3))
			})
			It("Should return MinAvailable equal to quorum size of cluster", func() {
				etcd.Spec.Replicas = 6
				Expect(CalculatePDBMinAvailable(etcd)).To(Equal(4))
			})
		})
	})
})
