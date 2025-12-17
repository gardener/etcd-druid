// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"
	"os"
	"slices"
	"time"

	"github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"
	"github.com/gardener/etcd-druid/test/e2e/testenv"
	e2etestutils "github.com/gardener/etcd-druid/test/e2e/utils"
	testutils "github.com/gardener/etcd-druid/test/utils"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/gomega"
)

const (
	pkiResourcesDir              = "pki-resources"
	defaultBackupStoreSecretName = "etcd-backup"
)

func initializeTestCase(g *WithT, testEnv *testenv.TestEnvironment, logger logr.Logger, testNamespace, etcdName string) {
	createNamespace(g, testEnv, logger, testNamespace)
	etcdCertsDir, etcdPeerCertsDir, etcdbrCertsDir := generatePKIResources(g, logger, testNamespace, etcdName)
	createTLSSecrets(g, testEnv, logger, testNamespace, etcdCertsDir, etcdPeerCertsDir, etcdbrCertsDir)
	createBackupSecret(g, testEnv, logger, testNamespace)
}

func createNamespace(g *WithT, testEnv *testenv.TestEnvironment, logger logr.Logger, testNamespace string) {
	logger.Info("creating test namespace")
	g.Expect(testEnv.CreateTestNamespace(testNamespace)).To(Succeed())
	logger.Info("successfully created test namespace")
}

func generatePKIResources(g *WithT, logger logr.Logger, testNamespace, etcdName string) (string, string, string) {
	logger.Info("generating PKI resources")
	certDir := fmt.Sprintf("%s/%s", pkiResourcesDir, testNamespace)
	// certs for etcd server-client communication
	etcdCertsDir := fmt.Sprintf("%s/etcd", certDir)
	g.Expect(os.MkdirAll(etcdCertsDir, 0755)).To(Succeed()) // #nosec: G301 -- local directory creation for test purposes.
	g.Expect(e2etestutils.GeneratePKIResources(logger, etcdCertsDir, etcdName, testNamespace)).To(Succeed())
	// certs for etcd peer communication
	etcdPeerCertsDir := fmt.Sprintf("%s/etcd-peer", certDir)
	g.Expect(os.MkdirAll(etcdPeerCertsDir, 0755)).To(Succeed()) // #nosec: G301 -- local directory creation for test purposes.
	g.Expect(e2etestutils.GeneratePKIResources(logger, etcdPeerCertsDir, etcdName, testNamespace)).To(Succeed())
	// certs for etcd-backup-restore TLS
	etcdbrCertsDir := fmt.Sprintf("%s/etcd-backup-restore", certDir)
	g.Expect(os.MkdirAll(etcdbrCertsDir, 0755)).To(Succeed()) // #nosec: G301 -- local directory creation for test purposes.
	g.Expect(e2etestutils.GeneratePKIResources(logger, etcdbrCertsDir, etcdName, testNamespace)).To(Succeed())
	logger.Info("successfully generated PKI resources")
	return etcdCertsDir, etcdPeerCertsDir, etcdbrCertsDir
}

func createTLSSecrets(g *WithT, testEnv *testenv.TestEnvironment, logger logr.Logger, testNamespace string, etcdCertsDir string, etcdPeerCertsDir string, etcdbrCertsDir string) {
	logger.Info("creating TLS secrets")
	// TLS secrets for etcd server-client communication
	g.Expect(e2etestutils.CreateCASecret(testEnv.GetContext(), testEnv.GetClient(), testutils.ClientTLSCASecretName, testNamespace, etcdCertsDir)).To(Succeed())
	g.Expect(e2etestutils.CreateServerTLSSecret(testEnv.GetContext(), testEnv.GetClient(), testutils.ClientTLSServerCertSecretName, testNamespace, etcdCertsDir)).To(Succeed())
	g.Expect(e2etestutils.CreateClientTLSSecret(testEnv.GetContext(), testEnv.GetClient(), testutils.ClientTLSClientCertSecretName, testNamespace, etcdCertsDir)).To(Succeed())
	// TLS secrets for etcd peer communication
	g.Expect(e2etestutils.CreateCASecret(testEnv.GetContext(), testEnv.GetClient(), testutils.PeerTLSCASecretName, testNamespace, etcdPeerCertsDir)).To(Succeed())
	g.Expect(e2etestutils.CreateServerTLSSecret(testEnv.GetContext(), testEnv.GetClient(), testutils.PeerTLSServerCertSecretName, testNamespace, etcdPeerCertsDir)).To(Succeed())
	// TLS secrets for etcd-backup-restore TLS
	g.Expect(e2etestutils.CreateCASecret(testEnv.GetContext(), testEnv.GetClient(), testutils.BackupRestoreTLSCASecretName, testNamespace, etcdbrCertsDir)).To(Succeed())
	g.Expect(e2etestutils.CreateServerTLSSecret(testEnv.GetContext(), testEnv.GetClient(), testutils.BackupRestoreTLSServerCertSecretName, testNamespace, etcdbrCertsDir)).To(Succeed())
	g.Expect(e2etestutils.CreateClientTLSSecret(testEnv.GetContext(), testEnv.GetClient(), testutils.BackupRestoreTLSClientCertSecretName, testNamespace, etcdbrCertsDir)).To(Succeed())
	logger.Info("successfully created TLS secrets")
}

func getSecret(testEnv *testenv.TestEnvironment, namespace, secretName string) (*corev1.Secret, error) {
	secret := &corev1.Secret{}
	err := testEnv.GetClient().Get(testEnv.GetContext(), types.NamespacedName{Namespace: namespace, Name: secretName}, secret)
	if err != nil {
		return nil, fmt.Errorf("failed to get secret %s in namespace %s: %w", secretName, namespace, err)
	}
	return secret, nil
}

func checkSecretFinalizer(testEnv *testenv.TestEnvironment, namespace, secretName string, expectFinalizer bool) error {
	secret, err := getSecret(testEnv, namespace, secretName)
	if err != nil {
		return err
	}

	if expectFinalizer == slices.Contains(secret.ObjectMeta.Finalizers, common.FinalizerName) {
		return nil
	}
	return fmt.Errorf("expected finalizer %v on secret %s in namespace %s, but was not satisfied", common.FinalizerName, secretName, namespace)
}

// TODO: complete me: for peer CA rotation test
func generateCA(g *WithT, logger logr.Logger, testNamespace string) (string, error) {
	logger.Info("generating new CA")
	return "", nil
}

func createBackupSecret(g *WithT, testEnv *testenv.TestEnvironment, logger logr.Logger, namespace string) {
	logger.Info("creating backup secret")
	g.Expect(testutils.CreateBackupProviderLocalSecret(testEnv.GetContext(), testEnv.GetClient(), defaultBackupStoreSecretName, namespace)).To(Succeed())
	logger.Info("successfully created backup secret")
}

func updateEtcdTLSAndLabels(etcd *v1alpha1.Etcd, clientTLSEnabled, peerTLSEnabled, backupRestoreTLSEnabled bool, additionalLabels map[string]string) {
	etcd.Spec.Etcd.ClientUrlTLS = nil
	if clientTLSEnabled {
		etcd.Spec.Etcd.ClientUrlTLS = testutils.GetClientTLSConfig()
	}

	etcd.Spec.Etcd.PeerUrlTLS = nil
	if peerTLSEnabled {
		etcd.Spec.Etcd.PeerUrlTLS = testutils.GetPeerTLSConfig()
	}

	etcd.Spec.Backup.TLS = nil
	if backupRestoreTLSEnabled {
		etcd.Spec.Backup.TLS = testutils.GetBackupRestoreTLSConfig()
	}

	etcd.Spec.Labels = testutils.MergeMaps(etcd.Spec.Labels, additionalLabels)
}

func cleanupTestArtifactsIfNecessary(testEnv *testenv.TestEnvironment, logger logr.Logger, g *WithT, ns string, etcd *v1alpha1.Etcd, timeout time.Duration) {
	logger.Info(fmt.Sprintf("deleting namespace %s", ns))
	g.Expect(testEnv.DeleteTestNamespace(ns)).To(Succeed())
	logger.Info("successfully deleted namespace")
}

func startEtcdReconcileAnnotator(ctx context.Context, testEnv *testenv.TestEnvironment, logger logr.Logger) context.CancelFunc {
	annotatorCtx, cancel := context.WithCancel(ctx)
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-annotatorCtx.Done():
				return
			case <-ticker.C:
				etcdList, err := testEnv.ListEtcds("") // list Etcds across all namespaces
				if err != nil {
					logger.Error(err, "failed to list Etcd resources")
					continue
				}
				now := time.Now()
				for _, etcd := range etcdList.Items {
					created := etcd.ObjectMeta.CreationTimestamp.Time
					if now.Sub(created) < 30*time.Second {
						continue
					}
					if etcd.Status.ObservedGeneration == nil {
						logger.Info("Force-reconciling Etcd since it was not picked up for reconciliation by etcd controller", "name", etcd.Name, "namespace", etcd.Namespace)
						original := etcd.DeepCopy()
						if etcd.Annotations == nil {
							etcd.Annotations = map[string]string{}
						}
						etcd.Annotations[v1alpha1.DruidOperationAnnotation] = v1alpha1.DruidOperationReconcile
						patch := client.MergeFrom(original)
						err = testEnv.GetClient().Patch(annotatorCtx, &etcd, patch)
						if err != nil {
							logger.Error(err, "failed to patch Etcd annotation", "name", etcd.Name, "namespace", etcd.Namespace)
						} else {
							logger.Info("Added operation-reconcile annotation to trigger reconciliation", "name", etcd.Name, "namespace", etcd.Namespace)
						}
					}
				}
			}
		}
	}()
	return cancel
}
