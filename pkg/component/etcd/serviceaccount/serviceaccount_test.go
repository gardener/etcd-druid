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

package serviceaccount

import (
	"context"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/pkg/client/kubernetes"
	gardenercomponent "github.com/gardener/gardener/pkg/operation/botanist/component"

	. "github.com/gardener/gardener/pkg/utils/test/matchers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("ServiceAccount Component", Ordered, func() {
	var (
		saComponent gardenercomponent.Deployer
		ctx         = context.TODO()
		c           = fake.NewClientBuilder().WithScheme(kubernetes.Scheme).Build()
		values      *Values
	)

	Context("#Deploy", func() {

		BeforeEach(func() {
			values = &Values{
				Name:      "test-service-account",
				Namespace: "test-namespace",
				Labels: map[string]string{
					"foo": "bar",
				},
				DisableAutomount: true,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion:         druidv1alpha1.GroupVersion.String(),
						Kind:               "Etcd",
						Name:               "test-etcd",
						UID:                "123-456-789",
						Controller:         pointer.Bool(true),
						BlockOwnerDeletion: pointer.Bool(true),
					},
				},
			}
			saComponent = New(c, values)
			err := saComponent.Deploy(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			err := saComponent.Destroy(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should create the ServiceAccount with the expected values", func() {
			By("verifying that the ServiceAccount is created on the K8s cluster as expected")
			created := &corev1.ServiceAccount{}
			err := c.Get(ctx, getServiceAccountKeyFromValue(values), created)
			Expect(err).NotTo(HaveOccurred())
			verifyServicAccountValues(created, values)
		})

		It("should update the ServiceAccount with the expected values", func() {
			By("updating the ServiceAccount")
			values.Labels["new"] = "label"
			err := saComponent.Deploy(ctx)
			Expect(err).NotTo(HaveOccurred())

			By("verifying that the ServiceAccount is updated on the K8s cluster as expected")
			updated := &corev1.ServiceAccount{}
			err = c.Get(ctx, getServiceAccountKeyFromValue(values), updated)
			Expect(err).NotTo(HaveOccurred())
			verifyServicAccountValues(updated, values)
		})

		It("should not return an error when there is nothing to update the ServiceAccount", func() {
			err := saComponent.Deploy(ctx)
			Expect(err).NotTo(HaveOccurred())
			updated := &corev1.ServiceAccount{}
			err = c.Get(ctx, getServiceAccountKeyFromValue(values), updated)
			Expect(err).NotTo(HaveOccurred())
			verifyServicAccountValues(updated, values)
		})

		It("should return an error when the update fails", func() {
			values.Name = ""
			err := saComponent.Deploy(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Required value: name is required"))
		})
	})

	Context("#Destroy", func() {
		BeforeEach(func() {
			values = &Values{
				Name:      "test-service-account",
				Namespace: "test-namespace",
				Labels: map[string]string{
					"foo": "bar",
				},
				DisableAutomount: true,
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion:         druidv1alpha1.GroupVersion.String(),
						Kind:               "Etcd",
						Name:               "test-etcd",
						UID:                "123-456-789",
						Controller:         pointer.Bool(true),
						BlockOwnerDeletion: pointer.Bool(true),
					},
				},
			}
			saComponent = New(c, values)
			err := saComponent.Deploy(ctx)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should delete the ServiceAccount", func() {
			By("deleting the ServiceAccount")
			err := saComponent.Destroy(ctx)
			Expect(err).NotTo(HaveOccurred())

			By("verifying that the ServiceAccount is deleted from the K8s cluster as expected")
			sa := &corev1.ServiceAccount{}
			Expect(c.Get(ctx, getServiceAccountKeyFromValue(values), sa)).To(BeNotFoundError())
		})

		It("should not return an error when there is nothing to delete", func() {
			By("deleting the ServiceAccount")
			err := saComponent.Destroy(ctx)
			Expect(err).NotTo(HaveOccurred())

			By("verifying that attempting to delete the ServiceAccount again returns no error")
			err = saComponent.Destroy(ctx)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})

func getServiceAccountKeyFromValue(value *Values) types.NamespacedName {
	return client.ObjectKey{Name: value.Name, Namespace: value.Namespace}
}

func verifyServicAccountValues(expected *corev1.ServiceAccount, value *Values) {
	Expect(expected.Name).To(Equal(value.Name))
	Expect(expected.Labels).Should(Equal(value.Labels))
	Expect(expected.Namespace).To(Equal(value.Namespace))
	Expect(expected.OwnerReferences).To(Equal(value.OwnerReferences))
	Expect(expected.AutomountServiceAccountToken).To(Equal(pointer.Bool(!value.DisableAutomount)))
}
