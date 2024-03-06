// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package rolebinding_test

import (
	"github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/pkg/component/etcd/rolebinding"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("RoleBinding", func() {
	var (
		etcd = &v1alpha1.Etcd{
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
		expected = &rolebinding.Values{
			Name:      etcd.GetRoleBindingName(),
			Namespace: etcd.Namespace,
			Labels: map[string]string{
				"name":     "etcd",
				"instance": etcd.Name,
			},
			RoleName:           etcd.GetRoleName(),
			ServiceAccountName: etcd.GetServiceAccountName(),
			OwnerReference:     etcd.GetAsOwnerReference(),
		}
	)

	Context("Generate Values", func() {
		It("should generate correct values", func() {
			values := rolebinding.GenerateValues(etcd)
			Expect(values).To(Equal(expected))
		})
	})
})
