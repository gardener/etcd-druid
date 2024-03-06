// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package serviceaccount

import (
	"context"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/pkg/client/kubernetes"
	gardenercomponent "github.com/gardener/gardener/pkg/component"

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
		ctx         context.Context
		c           client.Client
		values      *Values
	)

	Context("#Deploy", func() {

		BeforeEach(func() {
			ctx = context.Background()
			c = fake.NewClientBuilder().WithScheme(kubernetes.Scheme).Build()

			values = &Values{
				Name:      "test-service-account",
				Namespace: "test-namespace",
				Labels: map[string]string{
					"foo": "bar",
				},
				DisableAutomount: true,
				OwnerReference: metav1.OwnerReference{
					APIVersion:         druidv1alpha1.GroupVersion.String(),
					Kind:               "Etcd",
					Name:               "test-etcd",
					UID:                "123-456-789",
					Controller:         pointer.Bool(true),
					BlockOwnerDeletion: pointer.Bool(true),
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
			ctx = context.Background()
			c = fake.NewClientBuilder().WithScheme(kubernetes.Scheme).Build()

			values = &Values{
				Name:      "test-service-account",
				Namespace: "test-namespace",
				Labels: map[string]string{
					"foo": "bar",
				},
				DisableAutomount: true,
				OwnerReference: metav1.OwnerReference{
					APIVersion:         druidv1alpha1.GroupVersion.String(),
					Kind:               "Etcd",
					Name:               "test-etcd",
					UID:                "123-456-789",
					Controller:         pointer.Bool(true),
					BlockOwnerDeletion: pointer.Bool(true),
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

func verifyServicAccountValues(expected *corev1.ServiceAccount, values *Values) {
	Expect(expected.Name).To(Equal(values.Name))
	Expect(expected.Labels).Should(Equal(values.Labels))
	Expect(expected.Namespace).To(Equal(values.Namespace))
	Expect(expected.OwnerReferences).To(Equal([]metav1.OwnerReference{values.OwnerReference}))
	Expect(expected.AutomountServiceAccountToken).To(Equal(pointer.Bool(!values.DisableAutomount)))
}
