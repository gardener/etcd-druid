// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package serviceaccount_test

import (
	"github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/pkg/component/etcd/serviceaccount"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("ServiceAccount", func() {
	var (
		etcd     *v1alpha1.Etcd
		expected *serviceaccount.Values
	)

	BeforeEach(func() {
		etcd = &v1alpha1.Etcd{
			TypeMeta: metav1.TypeMeta{
				APIVersion: v1alpha1.GroupVersion.String(),
				Kind:       "Etcd",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "etcd-name",
				Namespace: "etcd-namespace",
				UID:       "etcd-uid",
			},
			Spec: v1alpha1.EtcdSpec{
				Labels: map[string]string{
					"key1": "value1",
					"key2": "value2",
				},
			},
		}
		expected = &serviceaccount.Values{
			Name:      etcd.GetServiceAccountName(),
			Namespace: etcd.Namespace,
			Labels: map[string]string{
				"name":     "etcd",
				"instance": etcd.Name,
			},
			OwnerReference: etcd.GetAsOwnerReference(),

			DisableAutomount: true,
		}
	})
	Context("Generate Values", func() {
		It("should generate correct values with automount disabled", func() {
			values := serviceaccount.GenerateValues(etcd, true)
			Expect(values).To(Equal(expected))
		})

		It("should generate correct values with automount enabled", func() {
			values := serviceaccount.GenerateValues(etcd, false)
			expected.DisableAutomount = false
			Expect(values).To(Equal(expected))
		})
	})
})
