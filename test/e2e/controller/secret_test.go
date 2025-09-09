// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"fmt"
	"testing"
	"time"

	testutils "github.com/gardener/etcd-druid/test/utils"

	"github.com/go-logr/logr/testr"

	. "github.com/onsi/gomega"
)

var (
	timeout         = time.Minute * 2
	pollingInterval = time.Second * 2
)

// TestSecretFinalizers tests addition and removal of finalizer on referred secrets in Etcd resources.
func TestSecretFinalizers(t *testing.T) {
	t.Parallel()
	log := testr.NewWithOptions(t, testr.Options{LogTimestamp: true})
	g := NewWithT(t)
	for _, provider := range providers {
		tcName := fmt.Sprintf("secret-%s", provider)
		t.Run(tcName, func(t *testing.T) {
			t.Parallel()

			testNamespace := testutils.GenerateTestNamespaceName(t, fmt.Sprintf("%s%s", testNamespacePrefix, tcName), 4)
			logger := log.WithName(tcName).WithValues("etcdName", defaultEtcdName, "namespace", testNamespace)

			initializeTestCase(g, testEnv, logger, testNamespace, defaultEtcdName)

			logger.Info("running tests")
			etcdBuilder := testutils.EtcdBuilderWithoutDefaults(defaultEtcdName, testNamespace).
				WithReplicas(int32(0)).
				WithClientTLS().
				WithPeerTLS().
				WithDefaultBackup().
				WithBackupRestoreTLS().
				WithStorageProvider(provider, fmt.Sprintf("%s/%s", testNamespace, defaultEtcdName))
			etcd := etcdBuilder.Build()

			referencedSecrets := []string{
				etcd.Spec.Etcd.ClientUrlTLS.TLSCASecretRef.Name,
				etcd.Spec.Etcd.ClientUrlTLS.ServerTLSSecretRef.Name,
				etcd.Spec.Etcd.ClientUrlTLS.ClientTLSSecretRef.Name,
				etcd.Spec.Etcd.PeerUrlTLS.TLSCASecretRef.Name,
				etcd.Spec.Etcd.PeerUrlTLS.ServerTLSSecretRef.Name,
			}
			if etcd.Spec.Backup.Store != nil {
				referencedSecrets = append(referencedSecrets,
					etcd.Spec.Backup.Store.SecretRef.Name,
					etcd.Spec.Backup.TLS.TLSCASecretRef.Name,
					etcd.Spec.Backup.TLS.ServerTLSSecretRef.Name,
					etcd.Spec.Backup.TLS.ClientTLSSecretRef.Name,
				)
			}

			logger.Info("creating Etcd")
			testEnv.CreateAndCheckEtcd(g, etcd, timeoutEtcdReconciliation)
			logger.Info("successfully created Etcd")

			logger.Info("checking finalizers exist on referenced secrets")
			for _, secretName := range referencedSecrets {
				g.Eventually(func() error {
					return checkSecretFinalizer(testEnv, etcd.Namespace, secretName, true)
				}, timeout, pollingInterval).Should(BeNil())
			}

			logger.Info("deleting Etcd")
			etcd, err := testEnv.GetEtcd(etcd.Name, etcd.Namespace)
			g.Expect(err).ToNot(HaveOccurred())
			testEnv.DeleteAndCheckEtcd(g, logger, etcd, timeoutEtcdReconciliation)
			logger.Info("successfully deleted Etcd")

			logger.Info("checking finalizers are removed from referenced secrets")
			for _, secretName := range referencedSecrets {
				g.Eventually(func() error {
					return checkSecretFinalizer(testEnv, etcd.Namespace, secretName, false)
				}, timeout, pollingInterval).Should(BeNil())
			}

			logger.Info("finished running tests")

			if !retainTestArtifacts {
				cleanupTestArtifactsIfNecessary(testEnv, logger, g, testNamespace, etcd, timeout)
			}
		})
	}
}
