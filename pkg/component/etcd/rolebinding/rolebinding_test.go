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
	"context"

	"github.com/gardener/etcd-druid/pkg/client/kubernetes"
	"github.com/gardener/etcd-druid/pkg/component/etcd/rolebinding"

	"github.com/gardener/gardener/pkg/operation/botanist/component"
	. "github.com/gardener/gardener/pkg/utils/test/matchers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/types"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("RoleBinding Component", func() {
	var (
		ctx                  context.Context
		c                    client.Client
		value                *rolebinding.Values
		roleBindingComponent component.Deployer
	)

	BeforeEach(func() {
		ctx = context.TODO()
		c = fake.NewClientBuilder().WithScheme(kubernetes.Scheme).Build()
		value = getTestValue()
		roleBindingComponent = rolebinding.New(c, value)
	})

	Describe("#Deploy", func() {
		AfterEach(func() {
			Expect(roleBindingComponent.Destroy(ctx)).NotTo(HaveOccurred())
		})

		It("should create and update the RoleBinding with the expected values", func() {
			By("creating a RoleBinding")
			err := roleBindingComponent.Deploy(ctx)
			Expect(err).NotTo(HaveOccurred())

			By("verifying that the RoleBinding is created on the K8s cluster as expected")
			created := &rbacv1.RoleBinding{}
			err = c.Get(ctx, getRoleBindingKeyFromValue(value), created)
			Expect(err).NotTo(HaveOccurred())
			verifyRoleBindingValues(created, value)

			By("updating the RoleBinding")
			err = roleBindingComponent.Deploy(ctx)
			Expect(err).NotTo(HaveOccurred())

			By("verifying that the RoleBinding is updated on the K8s cluster as expected")
			updated := &rbacv1.RoleBinding{}
			err = c.Get(ctx, getRoleBindingKeyFromValue(value), updated)
			Expect(err).NotTo(HaveOccurred())
			verifyRoleBindingValues(updated, value)

			By("returning nil when there is nothing to update the RoleBinding")
			err = roleBindingComponent.Deploy(ctx)
			Expect(err).NotTo(HaveOccurred())
			updated = &rbacv1.RoleBinding{}
			err = c.Get(ctx, getRoleBindingKeyFromValue(value), updated)
			Expect(err).NotTo(HaveOccurred())
			verifyRoleBindingValues(updated, value)

			By("returning an error when the update fails")
			value.Name = ""
			err = roleBindingComponent.Deploy(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Required value: name is required"))
		})
	})

	Describe("#Destroy", func() {
		BeforeEach(func() {
			Expect(roleBindingComponent.Deploy(ctx)).NotTo(HaveOccurred())
		})

		It("should delete the RoleBinding", func() {
			roleBinding := &rbacv1.RoleBinding{}
			By("checking that the RoleBinding exists before deleting it")
			Expect(c.Get(ctx, getRoleBindingKeyFromValue(value), roleBinding)).NotTo(HaveOccurred())

			By("deleting the RoleBinding")
			err := roleBindingComponent.Destroy(ctx)
			Expect(err).NotTo(HaveOccurred())

			By("verifying that the RoleBinding is deleted from the K8s cluster as expected")
			Expect(c.Get(ctx, getRoleBindingKeyFromValue(value), roleBinding)).To(BeNotFoundError())

			By("returning nil when there is nothing to delete")
			err = roleBindingComponent.Destroy(ctx)
			Expect(err).To(BeNil())
		})
	})
})

func verifyRoleBindingValues(expected *rbacv1.RoleBinding, values *rolebinding.Values) {
	Expect(expected.Name).To(Equal(values.Name))
	Expect(expected.Labels).To(Equal(values.Labels))
	Expect(expected.Namespace).To(Equal(values.Namespace))
	Expect(expected.Namespace).To(Equal(values.Namespace))
	Expect(expected.Subjects).To(Equal([]rbacv1.Subject{
		{
			Kind:      "ServiceAccount",
			Name:      values.ServiceAccountName,
			Namespace: values.Namespace,
		},
	}))
	Expect(expected.RoleRef).To(Equal(rbacv1.RoleRef{
		APIGroup: "rbac.authorization.k8s.io",
		Kind:     "Role",
		Name:     values.RoleName,
	}))
}
func getRoleBindingKeyFromValue(value *rolebinding.Values) types.NamespacedName {
	return client.ObjectKey{Name: value.Name, Namespace: value.Namespace}
}

func getTestValue() *rolebinding.Values {
	return &rolebinding.Values{
		Name:      "test-rolebinding",
		Namespace: "test-namespace",
		Labels: map[string]string{
			"foo": "bar",
		},
	}
}
