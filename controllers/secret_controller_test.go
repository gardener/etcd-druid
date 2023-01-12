// Copyright (c) 2022 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package controllers

import (
	"context"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"

	. "github.com/gardener/gardener/pkg/utils/test/matchers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var _ = Describe("SecretController", func() {
	var (
		ctx = context.TODO()

		namespace *corev1.Namespace
		etcd      *druidv1alpha1.Etcd
	)

	BeforeEach(func() {
		namespace = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "secret-controller-tests",
			},
		}
		etcd = getEtcdWithTLS("etcd", namespace.Name)

		_, err := controllerutil.CreateOrUpdate(ctx, k8sClient, namespace, func() error { return nil })
		Expect(err).To(Not(HaveOccurred()))
	})

	It("should reconcile the finalizers for the referenced secrets", func() {
		getFinalizersForSecret := func(name string) func(g Gomega) []string {
			return func(g Gomega) []string {
				secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace.Name}}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(secret), secret)).To(Succeed())
				return secret.Finalizers
			}
		}

		By("creating new etcd with secret references")
		secretNames := []string{"client-url-ca-etcd", "client-url-etcd-client-tls", "client-url-etcd-server-tls", "peer-url-ca-etcd", "peer-url-etcd-server-tls", "etcd-backup"}
		Expect(createSecrets(k8sClient, namespace.Name, secretNames...)).To(BeEmpty())

		Expect(k8sClient.Create(ctx, etcd)).To(Succeed())

		By("verifying secret references got finalizer")
		for _, name := range secretNames {
			Eventually(getFinalizersForSecret(name)).Should(ConsistOf("druid.gardener.cloud/etcd-druid"), "for secret "+name)
		}

		By("updating existing etcd with new secret references")
		newSecretNames := []string{"client-url-ca-etcd2", "client-url-etcd-client-tls2", "client-url-etcd-server-tls2", "peer-url-ca-etcd2", "peer-url-etcd-server-tls2", "etcd-backup2"}
		Expect(createSecrets(k8sClient, namespace.Name, newSecretNames...)).To(BeEmpty())

		patch := client.MergeFrom(etcd.DeepCopy())
		etcd.Spec.Etcd.ClientUrlTLS.TLSCASecretRef.Name += "2"
		etcd.Spec.Etcd.ClientUrlTLS.ServerTLSSecretRef.Name += "2"
		etcd.Spec.Etcd.ClientUrlTLS.ClientTLSSecretRef.Name += "2"
		etcd.Spec.Etcd.PeerUrlTLS.TLSCASecretRef.Name += "2"
		etcd.Spec.Etcd.PeerUrlTLS.ServerTLSSecretRef.Name += "2"
		etcd.Spec.Backup.Store.SecretRef.Name += "2"
		Expect(k8sClient.Patch(ctx, etcd, patch)).To(Succeed())

		By("verifying new secret references got finalizer")
		for _, name := range newSecretNames {
			Eventually(getFinalizersForSecret(name)).Should(ConsistOf("druid.gardener.cloud/etcd-druid"), "for secret "+name)
		}

		By("verifying old secret references have no finalizer anymore")
		for _, name := range secretNames {
			Eventually(getFinalizersForSecret(name)).Should(BeEmpty(), "for secret "+name)
		}

		By("deleting etcd")
		Expect(k8sClient.Delete(ctx, etcd)).To(Succeed())

		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKeyFromObject(etcd), &druidv1alpha1.Etcd{})
		}, timeout, pollingInterval).Should(BeNotFoundError())

		By("verifying secret references have no finalizer anymore")
		for _, name := range newSecretNames {
			Eventually(getFinalizersForSecret(name)).Should(BeEmpty(), "for secret "+name)
		}
	})
})
