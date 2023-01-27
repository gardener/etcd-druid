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

package secret

import (
	"context"
	"fmt"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/pkg/client/kubernetes"
	"github.com/gardener/etcd-druid/pkg/common"
	"github.com/go-logr/logr"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	. "github.com/gardener/gardener/pkg/utils/test/matchers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("SecretController", func() {
	var (
		secretName    = "test-secret"
		testNamespace = "test-namespace"
	)

	Describe("isFinalizerNeeded", func() {
		var (
			etcdList druidv1alpha1.EtcdList
		)

		BeforeEach(func() {
			etcdList = druidv1alpha1.EtcdList{}
			for i := 0; i < 3; i++ {
				etcdList.Items = append(etcdList.Items, druidv1alpha1.Etcd{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("etcd-%d", i),
						Namespace: testNamespace,
					},
				})
			}
		})

		It("should return false if secret is not referred to by any Etcd object", func() {
			Expect(isFinalizerNeeded(secretName, &etcdList)).To(BeFalse())
		})

		It("should return true if secret is referred to in an Etcd object's Spec.Etcd.ClientUrlTLS section", func() {
			etcdList.Items[0].Spec.Etcd.ClientUrlTLS = &druidv1alpha1.TLSConfig{
				ServerTLSSecretRef: v1.SecretReference{
					Name:      secretName,
					Namespace: testNamespace,
				},
			}
			Expect(isFinalizerNeeded(secretName, &etcdList)).To(BeTrue())
		})

		It("should return true if secret is referred to in an Etcd object's Spec.Etcd.PeerUrlTLS section", func() {
			etcdList.Items[1].Spec.Etcd.PeerUrlTLS = &druidv1alpha1.TLSConfig{
				ServerTLSSecretRef: v1.SecretReference{
					Name:      secretName,
					Namespace: testNamespace,
				},
			}
			Expect(isFinalizerNeeded(secretName, &etcdList)).To(BeTrue())
		})

		It("should return true if secret is referred to in an Etcd object's Spec.Backup.Store section", func() {
			etcdList.Items[2].Spec.Backup.Store = &druidv1alpha1.StoreSpec{
				SecretRef: &v1.SecretReference{
					Name:      secretName,
					Namespace: testNamespace,
				},
			}
			Expect(isFinalizerNeeded(secretName, &etcdList)).To(BeTrue())
		})
	})

	Describe("addFinalizer", func() {
		var (
			secret     *corev1.Secret
			ctx        context.Context
			logger     = logr.Discard()
			fakeClient = fakeclient.NewClientBuilder().WithScheme(kubernetes.Scheme).Build()
		)

		BeforeEach(func() {
			ctx = context.Background()
			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: testNamespace,
				},
			}

			By("Create Secret")
			Expect(fakeClient.Create(ctx, secret)).To(Succeed())

			By("Ensure Secret is created")
			Eventually(func() error {
				return fakeClient.Get(ctx, client.ObjectKeyFromObject(secret), secret)
			}).Should(Succeed())
		})

		AfterEach(func() {
			ctx = context.Background()

			By("Remove any existing finalizers on Secret")
			patch := client.MergeFrom(secret.DeepCopy())
			secret.Finalizers = []string{}
			Expect(fakeClient.Patch(ctx, secret, patch)).To(Succeed())

			By("Delete Secret")
			Expect(fakeClient.Delete(ctx, secret)).To(Succeed())

			By("Ensure Secret is deleted")
			Eventually(func() error {
				return fakeClient.Get(ctx, client.ObjectKeyFromObject(secret), secret)
			}).Should(BeNotFoundError())
		})

		It("should return nil if secret already has finalizer", func() {
			patch := client.MergeFrom(secret.DeepCopy())
			secret.Finalizers = []string{common.FinalizerName}
			Expect(fakeClient.Patch(ctx, secret, patch)).To(Succeed())
			Expect(addFinalizer(ctx, logger, fakeClient, secret)).To(BeNil())
		})

		It("should add finalizer on secret if finalizer does not exist", func() {
			Expect(addFinalizer(ctx, logger, fakeClient, secret)).To(Succeed())
			finalizers := sets.NewString(secret.Finalizers...)
			Expect(finalizers.Has(common.FinalizerName)).To(BeTrue())
		})
	})

	Describe("removeFinalizer", func() {
		var (
			secret     *corev1.Secret
			ctx        context.Context
			logger     = logr.Discard()
			fakeClient = fakeclient.NewClientBuilder().WithScheme(kubernetes.Scheme).Build()
		)

		BeforeEach(func() {
			ctx = context.Background()
			secret = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: testNamespace,
					Finalizers: []string{
						common.FinalizerName,
					},
				},
			}

			By("Create Secret")
			Expect(fakeClient.Create(ctx, secret)).To(Succeed())

			By("Ensure Secret is created")
			Eventually(func() error {
				return fakeClient.Get(ctx, client.ObjectKeyFromObject(secret), secret)
			}).Should(Succeed())
		})

		AfterEach(func() {
			ctx = context.Background()

			By("Remove any existing finalizers on Secret")
			patch := client.MergeFrom(secret.DeepCopy())
			secret.Finalizers = []string{}
			Expect(fakeClient.Patch(ctx, secret, patch)).To(Succeed())

			By("Delete Secret")
			Expect(fakeClient.Delete(ctx, secret)).To(Succeed())

			By("Ensure Secret is deleted")
			Eventually(func() error {
				return fakeClient.Get(ctx, client.ObjectKeyFromObject(secret), secret)
			}).Should(BeNotFoundError())
		})

		It("should return nil if secret does not have finalizer", func() {
			patch := client.MergeFrom(secret.DeepCopy())
			secret.Finalizers = []string{}
			Expect(fakeClient.Patch(ctx, secret, patch)).To(Succeed())
			Expect(removeFinalizer(ctx, logger, fakeClient, secret)).To(BeNil())
		})

		It("should remove finalizer from secret if finalizer exists", func() {
			Expect(removeFinalizer(ctx, logger, fakeClient, secret)).To(Succeed())
			finalizers := sets.NewString(secret.Finalizers...)
			Expect(finalizers.Has(common.FinalizerName)).To(BeFalse())
		})
	})
})
