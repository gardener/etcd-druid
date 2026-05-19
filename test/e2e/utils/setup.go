// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"fmt"
	"os"
	"strings"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/store"
	"github.com/gardener/etcd-druid/test/e2e/testenv"
	testutils "github.com/gardener/etcd-druid/test/utils"

	"github.com/go-logr/logr"
	"k8s.io/utils/ptr"

	. "github.com/onsi/gomega"
)

const (
	PKIResourcesDir              = "pki-resources"
	DefaultBackupStoreSecretName = "etcd-backup"
	DefaultEtcdName              = "test"

	// Environment variables read by e2e TestMain entry points.
	EnvKubeconfigPath      = "KUBECONFIG"
	EnvRetainTestArtifacts = "RETAIN_TEST_ARTIFACTS"
	EnvBackupProviders     = "PROVIDERS"
)

// RetainTestArtifactsMode controls whether test artifacts are retained after test execution.
type RetainTestArtifactsMode string

const (
	RetainTestArtifactsAll    RetainTestArtifactsMode = "all"
	RetainTestArtifactsFailed RetainTestArtifactsMode = "failed"
	RetainTestArtifactsNone   RetainTestArtifactsMode = "none"
)

// GetProviderSuffix returns the storage provider suffix for the given storage provider.
func GetProviderSuffix(provider druidv1alpha1.StorageProvider) string {
	if provider == "none" {
		return "none"
	}
	p, err := store.StorageProviderFromInfraProvider(ptr.To(provider))
	if err != nil {
		return ""
	}
	return strings.ToLower(p)
}

// InitializeTestCase sets up the test environment by creating a namespace, generating PKI resources,
// and creating the necessary TLS secrets and backup secret.
func InitializeTestCase(g *WithT, testEnv *testenv.TestEnvironment, logger logr.Logger, testNamespace, etcdName string, provider druidv1alpha1.StorageProvider) {
	CreateNamespace(g, testEnv, logger, testNamespace)
	etcdCertsDir, etcdPeerCertsDir, etcdbrCertsDir := GeneratePKIResources(g, logger, testNamespace, etcdName)
	CreateTLSSecrets(g, testEnv, logger, testNamespace, etcdCertsDir, etcdPeerCertsDir, etcdbrCertsDir)
	CreateBackupSecret(g, testEnv, logger, testNamespace, provider)
}

// CreateNamespace creates a new namespace for testing.
func CreateNamespace(g *WithT, testEnv *testenv.TestEnvironment, logger logr.Logger, testNamespace string) {
	logger.Info("creating test namespace")
	g.Expect(testEnv.CreateTestNamespace(testNamespace)).To(Succeed())
	logger.Info("successfully created test namespace")
}

// GeneratePKIResources generates PKI resources for etcd and returns the directories containing the generated certificates.
func GeneratePKIResources(g *WithT, logger logr.Logger, testNamespace, etcdName string) (string, string, string) {
	logger.Info("generating PKI resources")
	certDir := fmt.Sprintf("%s/%s", PKIResourcesDir, testNamespace)
	etcdCertsDir := fmt.Sprintf("%s/etcd", certDir)
	g.Expect(os.MkdirAll(etcdCertsDir, 0755)).To(Succeed()) // #nosec: G301
	g.Expect(testutils.GeneratePKIResourcesToDirectory(logger, etcdCertsDir, etcdName, testNamespace)).To(Succeed())
	etcdPeerCertsDir := fmt.Sprintf("%s/etcd-peer", certDir)
	g.Expect(os.MkdirAll(etcdPeerCertsDir, 0755)).To(Succeed()) // #nosec: G301
	g.Expect(testutils.GeneratePKIResourcesToDirectory(logger, etcdPeerCertsDir, etcdName, testNamespace)).To(Succeed())
	etcdbrCertsDir := fmt.Sprintf("%s/etcd-backup-restore", certDir)
	g.Expect(os.MkdirAll(etcdbrCertsDir, 0755)).To(Succeed()) // #nosec: G301
	g.Expect(testutils.GeneratePKIResourcesToDirectory(logger, etcdbrCertsDir, etcdName, testNamespace)).To(Succeed())
	logger.Info("successfully generated PKI resources")
	return etcdCertsDir, etcdPeerCertsDir, etcdbrCertsDir
}

// CreateTLSSecrets creates the necessary TLS secrets in the specified namespace using the provided certificate directories.
func CreateTLSSecrets(g *WithT, testEnv *testenv.TestEnvironment, logger logr.Logger, testNamespace, etcdCertsDir, etcdPeerCertsDir, etcdbrCertsDir string) {
	logger.Info("creating TLS secrets")
	g.Expect(CreateCASecret(testEnv.Context(), testEnv.Client(), testutils.ClientTLSCASecretName, testNamespace, etcdCertsDir)).To(Succeed())
	g.Expect(CreateServerTLSSecret(testEnv.Context(), testEnv.Client(), testutils.ClientTLSServerCertSecretName, testNamespace, etcdCertsDir)).To(Succeed())
	g.Expect(CreateClientTLSSecret(testEnv.Context(), testEnv.Client(), testutils.ClientTLSClientCertSecretName, testNamespace, etcdCertsDir)).To(Succeed())
	g.Expect(CreateCASecret(testEnv.Context(), testEnv.Client(), testutils.PeerTLSCASecretName, testNamespace, etcdPeerCertsDir)).To(Succeed())
	g.Expect(CreateServerTLSSecret(testEnv.Context(), testEnv.Client(), testutils.PeerTLSServerCertSecretName, testNamespace, etcdPeerCertsDir)).To(Succeed())
	g.Expect(CreateCASecret(testEnv.Context(), testEnv.Client(), testutils.BackupRestoreTLSCASecretName, testNamespace, etcdbrCertsDir)).To(Succeed())
	g.Expect(CreateServerTLSSecret(testEnv.Context(), testEnv.Client(), testutils.BackupRestoreTLSServerCertSecretName, testNamespace, etcdbrCertsDir)).To(Succeed())
	g.Expect(CreateClientTLSSecret(testEnv.Context(), testEnv.Client(), testutils.BackupRestoreTLSClientCertSecretName, testNamespace, etcdbrCertsDir)).To(Succeed())
	logger.Info("successfully created TLS secrets")
}

// CreateBackupSecret creates a backup secret in the specified namespace.
func CreateBackupSecret(g *WithT, testEnv *testenv.TestEnvironment, logger logr.Logger, namespace string, provider druidv1alpha1.StorageProvider) {
	logger.Info("creating backup secret")
	g.Expect(testutils.CreateBackupSecret(testEnv.Context(), testEnv.Client(), DefaultBackupStoreSecretName, namespace, provider)).To(Succeed())
	logger.Info("successfully created backup secret")
}

// CleanupTestArtifacts deletes the test namespace based on the retention mode and test result.
func CleanupTestArtifacts(retainTestArtifacts RetainTestArtifactsMode, testSucceeded bool, testEnv *testenv.TestEnvironment, logger logr.Logger, g *WithT, ns string) {
	switch retainTestArtifacts {
	case RetainTestArtifactsAll:
		logger.Info("retaining test artifacts as per configuration")
		return
	case RetainTestArtifactsFailed:
		if !testSucceeded {
			logger.Info("retaining test artifacts for failed test as per configuration")
			return
		}
	}
	logger.Info(fmt.Sprintf("deleting namespace %s", ns))
	g.Expect(testEnv.DeleteTestNamespace(ns)).To(Succeed())
	logger.Info("successfully deleted namespace")
}
