// Copyright (c) 2023 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package rolebinding_test

import (
	"github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/pkg/component/etcd/rolebinding"
	"github.com/gardener/etcd-druid/pkg/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
)

var _ = Describe("RoleBindig", func() {
	var (
		etcd = &v1alpha1.Etcd{
			ObjectMeta: v1.ObjectMeta{
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
			Name:               utils.GetRoleBindingName(etcd),
			Namespace:          etcd.Namespace,
			Labels:             etcd.Spec.Labels,
			RoleName:           utils.GetRoleName(etcd),
			ServiceAccountName: utils.GetServiceAccountName(etcd),
			OwnerReferences: []v1.OwnerReference{
				{
					APIVersion:         v1alpha1.GroupVersion.String(),
					Kind:               etcd.Kind,
					Name:               etcd.Name,
					UID:                etcd.UID,
					Controller:         pointer.BoolPtr(true),
					BlockOwnerDeletion: pointer.BoolPtr(true),
				},
			},
		}
	)

	Context("Generate Values", func() {
		It("should generate correct values", func() {
			values := rolebinding.GenerateValues(etcd)
			Expect(values).To(Equal(expected))
		})
	})
})
