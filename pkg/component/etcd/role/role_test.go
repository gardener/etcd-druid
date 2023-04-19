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

package role_test

import (
	"context"

	"github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/pkg/client/kubernetes"
	"github.com/gardener/etcd-druid/pkg/component/etcd/role"

	"github.com/gardener/gardener/pkg/operation/botanist/component"
	. "github.com/gardener/gardener/pkg/utils/test/matchers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"

	testutils "github.com/gardener/etcd-druid/test/utils"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("Role Component", Ordered, func() {
	var (
		ctx           context.Context
		c             client.Client
		values        *role.Values
		roleComponent component.Deployer
	)

	BeforeEach(func() {
		ctx = context.Background()
		c = fake.NewClientBuilder().WithScheme(kubernetes.Scheme).Build()
		values = getTestRoleValues()
		roleComponent = role.New(c, values)
	})

	Describe("#Deploy", func() {
		It("should create the Role with the expected values", func() {
			By("creating a Role")
			err := roleComponent.Deploy(ctx)
			Expect(err).NotTo(HaveOccurred())

			By("verifying that the Role is created on the K8s cluster as expected")
			created := &rbacv1.Role{}
			err = c.Get(ctx, getRoleKeyFromValue(values), created)
			Expect(err).NotTo(HaveOccurred())
			verifyRoleValues(created, values)
		})
		It("should update the Role with the expected values", func() {
			By("updating the Role")
			values.Labels["new"] = "label"
			err := roleComponent.Deploy(ctx)
			Expect(err).NotTo(HaveOccurred())

			By("verifying that the Role is updated on the K8s cluster as expected")
			updated := &rbacv1.Role{}
			err = c.Get(ctx, getRoleKeyFromValue(values), updated)
			Expect(err).NotTo(HaveOccurred())
			verifyRoleValues(updated, values)
		})
		It("should not return an error when there is nothing to update the Role", func() {
			err := roleComponent.Deploy(ctx)
			Expect(err).NotTo(HaveOccurred())
			updated := &rbacv1.Role{}
			err = c.Get(ctx, getRoleKeyFromValue(values), updated)
			Expect(err).NotTo(HaveOccurred())
			verifyRoleValues(updated, values)
		})
		It("should return an error when the update fails", func() {
			values.Name = ""
			err := roleComponent.Deploy(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Required value: name is required"))
		})
	})

	Describe("#Destroy", func() {
		It("should delete the Role", func() {
			By("deleting the Role")
			err := roleComponent.Destroy(ctx)
			Expect(err).NotTo(HaveOccurred())

			By("verifying that the Role is deleted from the K8s cluster as expected")
			role := &rbacv1.Role{}
			Expect(c.Get(ctx, getRoleKeyFromValue(values), role)).To(BeNotFoundError())
		})
		It("should not return an error when there is nothing to delete", func() {
			err := roleComponent.Destroy(ctx)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})

func verifyRoleValues(expected *rbacv1.Role, values *role.Values) {
	Expect(expected.Name).To(Equal(values.Name))
	Expect(expected.Labels).To(Equal(values.Labels))
	Expect(expected.Namespace).To(Equal(values.Namespace))
	Expect(expected.OwnerReferences).To(Equal([]metav1.OwnerReference{*values.OwnerReference}))
	Expect(expected.Rules).To(MatchAllElements(testutils.RuleIterator, Elements{
		"coordination.k8s.io": MatchFields(IgnoreExtras, Fields{
			"APIGroups": MatchAllElements(testutils.StringArrayIterator, Elements{
				"coordination.k8s.io": Equal("coordination.k8s.io"),
			}),
			"Resources": MatchAllElements(testutils.StringArrayIterator, Elements{
				"leases": Equal("leases"),
			}),
			"Verbs": MatchAllElements(testutils.StringArrayIterator, Elements{
				"list":   Equal("list"),
				"get":    Equal("get"),
				"update": Equal("update"),
				"patch":  Equal("patch"),
				"watch":  Equal("watch"),
			}),
		}),
		"apps": MatchFields(IgnoreExtras, Fields{
			"APIGroups": MatchAllElements(testutils.StringArrayIterator, Elements{
				"apps": Equal("apps"),
			}),
			"Resources": MatchAllElements(testutils.StringArrayIterator, Elements{
				"statefulsets": Equal("statefulsets"),
			}),
			"Verbs": MatchAllElements(testutils.StringArrayIterator, Elements{
				"list":   Equal("list"),
				"get":    Equal("get"),
				"update": Equal("update"),
				"patch":  Equal("patch"),
				"watch":  Equal("watch"),
			}),
		}),
		"": MatchFields(IgnoreExtras, Fields{
			"APIGroups": MatchAllElements(testutils.StringArrayIterator, Elements{
				"": Equal(""),
			}),
			"Resources": MatchAllElements(testutils.StringArrayIterator, Elements{
				"pods": Equal("pods"),
			}),
			"Verbs": MatchAllElements(testutils.StringArrayIterator, Elements{
				"list":  Equal("list"),
				"get":   Equal("get"),
				"watch": Equal("watch"),
			}),
		}),
	}))
}
func getRoleKeyFromValue(values *role.Values) types.NamespacedName {
	return client.ObjectKey{Name: values.Name, Namespace: values.Namespace}
}

func getTestRoleValues() *role.Values {
	return &role.Values{
		Name:      "test-role",
		Namespace: "test-namespace",
		Labels: map[string]string{
			"foo": "bar",
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{"coordination.k8s.io"},
				Resources: []string{"leases"},
				Verbs:     []string{"get", "list", "patch", "update", "watch"},
			},
			{
				APIGroups: []string{"apps"},
				Resources: []string{"statefulsets"},
				Verbs:     []string{"get", "list", "patch", "update", "watch"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     []string{"get", "list", "watch"},
			},
		},
		OwnerReference: &metav1.OwnerReference{
			APIVersion:         v1alpha1.GroupVersion.String(),
			Kind:               "etcd",
			Name:               "test-etcd",
			UID:                "123-456-789",
			Controller:         pointer.Bool(true),
			BlockOwnerDeletion: pointer.Bool(true),
		},
	}
}
