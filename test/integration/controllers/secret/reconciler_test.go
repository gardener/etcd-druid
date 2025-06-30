// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package secret

import (
	"context"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"
	testutils "github.com/gardener/etcd-druid/test/utils"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	timeout         = 1 * time.Minute
	pollingInterval = 2 * time.Second
)

var _ = Describe("Secret Controller", func() {
	var (
		ctx  = context.TODO()
		etcd *druidv1alpha1.Etcd
	)

	BeforeEach(func() {
		etcd = testutils.EtcdBuilderWithDefaults("etcd", namespace).
			WithClientTLS().
			WithPeerTLS().
			WithBackupRestoreTLS().
			Build()
	})

	It("should reconcile the finalizers for the referenced secrets", func() {
		getFinalizersForSecret := func(name string) func(g Gomega) []string {
			return func(g Gomega) []string {
				secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace}}
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(secret), secret)).To(Succeed())
				return secret.Finalizers
			}
		}

		By("creating new etcd with secret references")
		secretNames := []string{
			"client-url-ca-etcd",
			"client-url-etcd-client-tls",
			"client-url-etcd-server-tls",
			"peer-url-ca-etcd",
			"peer-url-etcd-server-tls",
			"ca-etcdbr",
			"etcdbr-server-tls",
			"etcdbr-client-tls",
			"etcd-backup",
		}
		err := testutils.CreateSecrets(ctx, k8sClient, namespace, secretNames...)
		Expect(err).ToNot(HaveOccurred())

		Expect(k8sClient.Create(ctx, etcd)).To(Succeed())

		By("verifying secret references got finalizer")
		for _, name := range secretNames {
			Eventually(getFinalizersForSecret(name)).Should(ConsistOf(common.FinalizerName), "for secret "+name)
		}

		By("updating existing etcd with new secret references")
		newSecretNames := []string{
			"client-url-ca-etcd2",
			"client-url-etcd-client-tls2",
			"client-url-etcd-server-tls2",
			"peer-url-ca-etcd2",
			"peer-url-etcd-server-tls2",
			"ca-etcdbr2",
			"etcdbr-server-tls2",
			"etcdbr-client-tls2",
			"etcd-backup2",
		}
		err = testutils.CreateSecrets(ctx, k8sClient, namespace, newSecretNames...)
		Expect(err).ToNot(HaveOccurred())

		patch := client.MergeFrom(etcd.DeepCopy())
		etcd.Spec.Etcd.ClientUrlTLS.TLSCASecretRef.Name += "2"
		etcd.Spec.Etcd.ClientUrlTLS.ServerTLSSecretRef.Name += "2"
		etcd.Spec.Etcd.ClientUrlTLS.ClientTLSSecretRef.Name += "2"
		etcd.Spec.Etcd.PeerUrlTLS.TLSCASecretRef.Name += "2"
		etcd.Spec.Etcd.PeerUrlTLS.ServerTLSSecretRef.Name += "2"
		etcd.Spec.Backup.TLS.TLSCASecretRef.Name += "2"
		etcd.Spec.Backup.TLS.ServerTLSSecretRef.Name += "2"
		etcd.Spec.Backup.TLS.ClientTLSSecretRef.Name += "2"
		etcd.Spec.Backup.Store.SecretRef.Name += "2"
		Expect(k8sClient.Patch(ctx, etcd, patch)).To(Succeed())

		By("verifying new secret references got finalizer")
		for _, name := range newSecretNames {
			Eventually(getFinalizersForSecret(name)).Should(ConsistOf(common.FinalizerName), "for secret "+name)
		}

		By("verifying old secret references have no finalizer anymore")
		for _, name := range secretNames {
			Eventually(getFinalizersForSecret(name)).Should(BeEmpty(), "for secret "+name)
		}

		By("deleting etcd")
		Expect(k8sClient.Delete(ctx, etcd)).To(Succeed())

		Eventually(func() error {
			return k8sClient.Get(ctx, client.ObjectKeyFromObject(etcd), &druidv1alpha1.Etcd{})
		}, timeout, pollingInterval).Should(testutils.BeNotFoundError())

		By("verifying secret references have no finalizer anymore")
		for _, name := range newSecretNames {
			Eventually(getFinalizersForSecret(name)).Should(BeEmpty(), "for secret "+name)
		}
	})
})
