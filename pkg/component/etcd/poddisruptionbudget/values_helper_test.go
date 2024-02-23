// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

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
