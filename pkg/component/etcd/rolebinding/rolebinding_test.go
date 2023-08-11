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

	"github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/pkg/client/kubernetes"
	"github.com/gardener/etcd-druid/pkg/component/etcd/rolebinding"

	"github.com/gardener/gardener/pkg/component"
	. "github.com/gardener/gardener/pkg/utils/test/matchers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("RoleBinding Component", func() {
	var (
		ctx                  context.Context
		c                    client.Client
		values               *rolebinding.Values
		roleBindingComponent component.Deployer
	)

	Context("#Deploy", func() {

		BeforeEach(func() {
			ctx = context.Background()
			c = fake.NewClientBuilder().WithScheme(kubernetes.Scheme).Build()
			values = getTestRoleBindingValues()
			roleBindingComponent = rolebinding.New(c, values)

			err := roleBindingComponent.Deploy(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			err := roleBindingComponent.Destroy(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create the RoleBinding with the expected values", func() {
			By("verifying that the RoleBinding is created on the K8s cluster as expected")
			created := &rbacv1.RoleBinding{}
			err := c.Get(ctx, getRoleBindingKeyFromValue(values), created)
			Expect(err).NotTo(HaveOccurred())
			verifyRoleBindingValues(created, values)
		})

		It("should update the RoleBinding with the expected values", func() {
			By("updating the RoleBinding")
			values.Labels["new"] = "label"
			err := roleBindingComponent.Deploy(ctx)
			Expect(err).NotTo(HaveOccurred())

			By("verifying that the RoleBinding is updated on the K8s cluster as expected")
			updated := &rbacv1.RoleBinding{}
			err = c.Get(ctx, getRoleBindingKeyFromValue(values), updated)
			Expect(err).NotTo(HaveOccurred())
			verifyRoleBindingValues(updated, values)
		})

		It("should not return an error when there is nothing to update the RoleBinding", func() {
			err := roleBindingComponent.Deploy(ctx)
			Expect(err).NotTo(HaveOccurred())
			updated := &rbacv1.RoleBinding{}
			err = c.Get(ctx, getRoleBindingKeyFromValue(values), updated)
			Expect(err).NotTo(HaveOccurred())
			verifyRoleBindingValues(updated, values)
		})

		It("should return an error when the update fails", func() {
			values.Name = ""
			err := roleBindingComponent.Deploy(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Required value: name is required"))
		})
	})

	Context("#Destroy", func() {

		BeforeEach(func() {
			ctx = context.Background()
			c = fake.NewClientBuilder().WithScheme(kubernetes.Scheme).Build()
			values = getTestRoleBindingValues()

			roleBindingComponent = rolebinding.New(c, values)
			err := roleBindingComponent.Deploy(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should delete the RoleBinding", func() {
			By("deleting the RoleBinding")
			err := roleBindingComponent.Destroy(ctx)
			Expect(err).NotTo(HaveOccurred())

			roleBinding := &rbacv1.RoleBinding{}
			By("verifying that the RoleBinding is deleted from the K8s cluster as expected")
			Expect(c.Get(ctx, getRoleBindingKeyFromValue(values), roleBinding)).To(BeNotFoundError())
		})

		It("should not return an error when there is nothing to delete", func() {
			By("deleting the RoleBinding")
			err := roleBindingComponent.Destroy(ctx)
			Expect(err).NotTo(HaveOccurred())

			By("verifying that attempting to delete the RoleBinding again returns no error")
			err = roleBindingComponent.Destroy(ctx)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})

func verifyRoleBindingValues(expected *rbacv1.RoleBinding, values *rolebinding.Values) {
	Expect(expected.Name).To(Equal(values.Name))
	Expect(expected.Labels).To(Equal(values.Labels))
	Expect(expected.Namespace).To(Equal(values.Namespace))
	Expect(expected.Namespace).To(Equal(values.Namespace))
	Expect(expected.OwnerReferences).To(Equal([]metav1.OwnerReference{values.OwnerReference}))
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
func getRoleBindingKeyFromValue(values *rolebinding.Values) types.NamespacedName {
	return client.ObjectKey{Name: values.Name, Namespace: values.Namespace}
}

func getTestRoleBindingValues() *rolebinding.Values {
	return &rolebinding.Values{
		Name:      "test-rolebinding",
		Namespace: "test-namespace",
		Labels: map[string]string{
			"foo": "bar",
		},
		RoleName:           "test-role",
		ServiceAccountName: "test-serviceaccount",
		OwnerReference: metav1.OwnerReference{
			APIVersion:         v1alpha1.GroupVersion.String(),
			Kind:               "etcd",
			Name:               "test-etcd",
			UID:                "123-456-789",
			Controller:         pointer.Bool(true),
			BlockOwnerDeletion: pointer.Bool(true),
		},
	}
}
