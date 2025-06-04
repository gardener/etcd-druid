// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	"github.com/go-logr/logr/testr"
	"os"
	"testing"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/test/e2e/testenv"
	druide2etestutils "github.com/gardener/etcd-druid/test/e2e/utils"
	testutils "github.com/gardener/etcd-druid/test/utils"

	. "github.com/onsi/gomega"
)

const (
	// environment variables
	envKubeconfigPath      = "KUBECONFIG"
	envRetainTestArtifacts = "RETAIN_TEST_ARTIFACTS"

	// test parameters
	// TODO: check and adjust value
	timeoutTest                = 1 * time.Hour
	timeoutEtcdReconciliation  = 5 * time.Minute
	timeoutDeployJob           = 2 * time.Minute
	timeoutEtcdDisruptionStart = 30 * time.Second
	timeoutEtcdRecovery        = 5 * time.Minute

	pkiResourcesDir              = "pki-resources"
	testNamespacePrefix          = "etcd-druid-e2e-"
	defaultEtcdName              = "test"
	defaultBackupStoreSecretName = "etcd-backup"
)

var (
	testEnv             *testenv.TestEnvironment
	retainTestArtifacts = false
)

func TestMain(m *testing.M) {
	kubeconfigPath, err := druide2etestutils.GetEnvOrError(envKubeconfigPath)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "KUBECONFIG not provided: %v\n", err)
		os.Exit(1)
	}
	cl, err := druide2etestutils.GetKubernetesClient(kubeconfigPath)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Failed to create Kubernetes client: %v\n", err)
		os.Exit(1)
	}

	ctx, cancelCtx := context.WithTimeout(context.Background(), timeoutTest)

	testEnv = testenv.NewTestEnvironment(ctx, cancelCtx, cl)
	if err = testEnv.PrepareScheme(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Failed to prepare scheme: %v\n", err)
		os.Exit(1)
	}

	val, err := druide2etestutils.GetEnvOrError(envRetainTestArtifacts)
	if err == nil && val == "true" {
		retainTestArtifacts = true
	}

	exitCode := m.Run()

	testEnv.Close()
	os.Exit(exitCode)
}

// TestBasic tests creation of 1-replica Etcd, hibernation, unhibernation and deletion of the Etcd cluster.
func TestBasic(t *testing.T) {
	t.Parallel()
	log := testr.NewWithOptions(t, testr.Options{LogTimestamp: true})

	testCases := []struct {
		name       string
		replicas   int32
		tlsEnabled bool
	}{
		{
			name:       "no-tls-0",
			tlsEnabled: false,
			replicas:   0,
		},
		{
			name:       "no-tls-1",
			tlsEnabled: false,
			replicas:   1,
		},
		{
			name:       "no-tls-3",
			tlsEnabled: false,
			replicas:   3,
		},
		{
			name:       "tls-0",
			tlsEnabled: true,
			replicas:   0,
		},
		{
			name:       "tls-1",
			tlsEnabled: true,
			replicas:   1,
		},
		{
			name:       "tls-3",
			tlsEnabled: true,
			replicas:   3,
		},
	}

	g := NewWithT(t)

	for _, tc := range testCases {
		tcName := fmt.Sprintf("basic-%s", tc.name)
		t.Run(tcName, func(t *testing.T) {
			t.Parallel()

			testNamespace := testutils.GenerateTestNamespaceName(t, fmt.Sprintf("%s%s", testNamespacePrefix, tcName))
			logger := log.WithName(tcName).WithValues("etcdName", defaultEtcdName, "namespace", testNamespace)

			initializeTestCase(g, logger, testNamespace)

			logger.Info("running tests")
			etcdBuilder := testutils.EtcdBuilderWithoutDefaults(defaultEtcdName, testNamespace).
				WithReplicas(tc.replicas).
				WithDefaultBackup().
				WithProviderLocal().
				WithBackupStorePrefix(fmt.Sprintf("%s/%s", testNamespace, defaultEtcdName))
			// TODO: use local provider based on env var config (two different test runs for no-provider and provider-local)
			if tc.tlsEnabled {
				etcdBuilder = etcdBuilder.WithClientTLS().
					WithPeerTLS().
					WithBackupRestoreTLS()
			}
			etcd := etcdBuilder.Build()

			logger.Info("creating Etcd")
			testEnv.CreateAndCheckEtcd(g, etcd, timeoutEtcdReconciliation)
			logger.Info("successfully created Etcd")

			// TODO: check if referred secrets have finalizer set on them

			logger.Info("hibernating Etcd")
			replicas := etcd.Spec.Replicas
			testEnv.HibernateAndCheckEtcd(g, etcd, timeoutEtcdReconciliation)
			logger.Info("successfully hibernated Etcd")

			logger.Info("unhibernating Etcd")
			etcd, err := testEnv.GetEtcd(etcd.Name, etcd.Namespace)
			g.Expect(err).ToNot(HaveOccurred())
			testEnv.UnhibernateAndCheckEtcd(g, etcd, replicas, timeoutEtcdReconciliation)
			logger.Info("successfully unhibernated Etcd")

			logger.Info("deleting Etcd")
			etcd, err = testEnv.GetEtcd(etcd.Name, etcd.Namespace)
			g.Expect(err).ToNot(HaveOccurred())
			testEnv.DeleteAndCheckEtcd(g, logger, etcd, timeoutEtcdReconciliation)
			logger.Info("successfully deleted Etcd")

			logger.Info("finished running tests")

			cleanupTestArtifactsIfNecessary(logger, g, testNamespace, etcd)
		})
	}
}

// TestScaleOut tests scale out of an Etcd cluster from 1 -> 3 replicas along with label changes.
func TestScaleOut(t *testing.T) {
	t.Parallel()
	log := testr.NewWithOptions(t, testr.Options{LogTimestamp: true})

	testCases := []struct {
		name                         string
		peerTLSEnabledBeforeScaleOut bool
		labelsBeforeScaleOut         map[string]string
		peerTLSEnabledAfterScaleOut  bool
		labelsAfterScaleOut          map[string]string
	}{
		{
			name: "basic",
		},
		{
			name:                        "enable-peer-tls",
			peerTLSEnabledAfterScaleOut: true,
		},
		{
			name:                         "with-peer-tls",
			peerTLSEnabledBeforeScaleOut: true,
			peerTLSEnabledAfterScaleOut:  true,
		},
		// TODO: uncomment after fixing reconciler behavior
		//{
		//	name:                         "disable-peer-tls",
		//	peerTLSEnabledBeforeScaleOut: true,
		//},
		{
			name:                "with-label-change",
			labelsAfterScaleOut: map[string]string{"foo": "bar"},
		},
		{
			name:                        "enable-peer-tls-label-change",
			peerTLSEnabledAfterScaleOut: true,
			labelsAfterScaleOut:         map[string]string{"foo": "bar"},
		},
	}

	g := NewWithT(t)

	for _, tc := range testCases {
		tcName := fmt.Sprintf("scaleout-%s", tc.name)
		t.Run(tcName, func(t *testing.T) {
			t.Parallel()

			testNamespace := testutils.GenerateTestNamespaceName(t, fmt.Sprintf("%s%s", testNamespacePrefix, tcName))
			logger := log.WithName(tcName).WithValues("etcdName", defaultEtcdName, "namespace", testNamespace)

			initializeTestCase(g, logger, testNamespace)

			logger.Info("running tests")
			etcdBuilder := testutils.EtcdBuilderWithoutDefaults(defaultEtcdName, testNamespace).
				WithClientTLS().
				WithBackupRestoreTLS().
				WithReplicas(1)
			// TODO: use local provider
			//WithProviderLocal()
			if tc.peerTLSEnabledBeforeScaleOut {
				etcdBuilder = etcdBuilder.WithPeerTLS()
			}
			if tc.labelsBeforeScaleOut != nil {
				etcdBuilder = etcdBuilder.WithSpecLabels(tc.labelsBeforeScaleOut)
			}
			etcd := etcdBuilder.Build()

			logger.Info("creating Etcd")
			testEnv.CreateAndCheckEtcd(g, etcd, timeoutEtcdReconciliation)
			logger.Info("successfully created Etcd")

			logger.Info("scaling out Etcd to 3 replicas")
			etcd.Spec.Replicas = 3
			updateEtcdTLSAndLabels(etcd, false, tc.peerTLSEnabledAfterScaleOut, false, etcd.Spec.Labels)
			testEnv.UpdateAndCheckEtcd(g, etcd, timeoutEtcdReconciliation*2)
			logger.Info("successfully scaled out Etcd to 3 replicas")

			logger.Info("finished running tests")

			cleanupTestArtifactsIfNecessary(logger, g, testNamespace, etcd)
		})
	}
}

// TestTLSAndLabelUpdates tests TLS changes along with label changes in an Etcd cluster.
func TestTLSAndLabelUpdates(t *testing.T) {
	t.Parallel()
	log := testr.NewWithOptions(t, testr.Options{LogTimestamp: true})

	testCases := []struct {
		name                                string
		clientTLSEnabledBeforeUpdate        bool
		peerTLSEnabledBeforeUpdate          bool
		backupRestoreTLSEnabledBeforeUpdate bool
		labelsBeforeUpdate                  map[string]string
		clientTLSEnabledAfterUpdate         bool
		peerTLSEnabledAfterUpdate           bool
		backupRestoreTLSEnabledAfterUpdate  bool
		labelsAfterUpdate                   map[string]string
	}{
		{
			name:                      "enable-peer",
			peerTLSEnabledAfterUpdate: true,
		},
		{
			name:                               "enable-client-peer-br",
			clientTLSEnabledAfterUpdate:        true,
			peerTLSEnabledAfterUpdate:          true,
			backupRestoreTLSEnabledAfterUpdate: true,
		},
		{
			name:                       "disable-peer",
			peerTLSEnabledBeforeUpdate: true,
		},
		{
			name:                                "disable-client-peer-br",
			clientTLSEnabledBeforeUpdate:        true,
			peerTLSEnabledBeforeUpdate:          true,
			backupRestoreTLSEnabledBeforeUpdate: true,
		},
		{
			name:                      "enable-peer-label-change",
			peerTLSEnabledAfterUpdate: true,
			labelsAfterUpdate: map[string]string{
				"foo": "bar",
			},
		},
		{
			name:                       "disable-peer-with-label-change",
			peerTLSEnabledBeforeUpdate: true,
			labelsAfterUpdate: map[string]string{
				"foo": "bar",
			},
		},
	}

	g := NewWithT(t)

	for _, tc := range testCases {
		tcName := fmt.Sprintf("update-%s", tc.name)
		t.Run(tcName, func(t *testing.T) {
			t.Parallel()

			testNamespace := testutils.GenerateTestNamespaceName(t, fmt.Sprintf("%s%s", testNamespacePrefix, tcName))
			logger := log.WithName(tcName).WithValues("etcdName", defaultEtcdName, "namespace", testNamespace)

			initializeTestCase(g, logger, testNamespace)

			logger.Info("running tests")
			etcdBuilder := testutils.EtcdBuilderWithoutDefaults(defaultEtcdName, testNamespace).
				WithReplicas(1)
			// TODO: use local provider
			//WithProviderLocal()
			if tc.clientTLSEnabledBeforeUpdate {
				etcdBuilder = etcdBuilder.WithClientTLS()
			}
			if tc.peerTLSEnabledBeforeUpdate {
				etcdBuilder = etcdBuilder.WithPeerTLS()
			}
			if tc.backupRestoreTLSEnabledBeforeUpdate {
				etcdBuilder = etcdBuilder.WithBackupRestoreTLS()
			}
			if tc.labelsBeforeUpdate != nil {
				etcdBuilder = etcdBuilder.WithSpecLabels(tc.labelsBeforeUpdate)
			}
			etcd := etcdBuilder.Build()

			logger.Info("creating Etcd")
			testEnv.CreateAndCheckEtcd(g, etcd, timeoutEtcdReconciliation)
			logger.Info("successfully created Etcd")

			logger.Info("updating Etcd")
			updateEtcdTLSAndLabels(etcd, tc.clientTLSEnabledAfterUpdate, tc.peerTLSEnabledAfterUpdate, tc.backupRestoreTLSEnabledAfterUpdate, tc.labelsAfterUpdate)
			testEnv.UpdateAndCheckEtcd(g, etcd, timeoutEtcdReconciliation*2)
			logger.Info("successfully updated Etcd")

			logger.Info("finished running tests")

			cleanupTestArtifactsIfNecessary(logger, g, testNamespace, etcd)
		})
	}
}

// TestRecovery tests for recovery of an Etcd cluster upon simulated disasters.
func TestRecovery(t *testing.T) {
	t.Parallel()
	log := testr.NewWithOptions(t, testr.Options{LogTimestamp: true})

	testCases := []struct {
		name                    string
		replicas                int32
		numMembersToBeCorrupted int
		numPodsToBeDeleted      int
		expectDowntime          bool
	}{
		{
			name:               "single-node-delete-1-pod",
			replicas:           1,
			numPodsToBeDeleted: 1,
			expectDowntime:     true,
		},
		{
			name:                    "single-node-corrupt-data",
			replicas:                1,
			numMembersToBeCorrupted: 1,
			expectDowntime:          true,
		},
		{
			name:               "delete-1-pod",
			replicas:           3,
			numPodsToBeDeleted: 1,
			expectDowntime:     false,
		},
		{
			name:               "delete-2-pods",
			replicas:           3,
			numPodsToBeDeleted: 2,
			expectDowntime:     true,
		},
		{
			name:               "delete-3-pods",
			replicas:           3,
			numPodsToBeDeleted: 3,
			expectDowntime:     true,
		},
		{
			name:                    "corrupt-1-member",
			replicas:                3,
			numMembersToBeCorrupted: 1,
			expectDowntime:          false,
		},
		{
			name:                    "delete-2-pods-corrupt-1-member",
			replicas:                3,
			numPodsToBeDeleted:      2,
			numMembersToBeCorrupted: 1,
			expectDowntime:          true,
		},
	}

	g := NewWithT(t)

	for _, tc := range testCases {
		tcName := fmt.Sprintf("recovery-%s", tc.name)
		t.Run(tcName, func(t *testing.T) {
			t.Parallel()

			testNamespace := testutils.GenerateTestNamespaceName(t, fmt.Sprintf("%s%s", testNamespacePrefix, tcName))
			logger := log.WithName(tcName).WithValues("etcdName", defaultEtcdName, "namespace", testNamespace)

			initializeTestCase(g, logger, testNamespace)

			logger.Info("running tests")
			etcdBuilder := testutils.EtcdBuilderWithoutDefaults(defaultEtcdName, testNamespace).
				WithClientTLS().
				WithPeerTLS().
				WithBackupRestoreTLS().
				WithReplicas(tc.replicas).
				WithAnnotations(map[string]string{
					"druid.gardener.cloud/disable-etcd-component-protection": "",
				})
			// TODO: use local provider
			//WithProviderLocal()
			etcd := etcdBuilder.Build()

			logger.Info("creating Etcd")
			testEnv.CreateAndCheckEtcd(g, etcd, timeoutEtcdReconciliation)
			logger.Info("successfully created Etcd")

			logger.Info("starting zero-downtime validator job")
			testEnv.DeployZeroDowntimeValidatorJob(g, testNamespace, druidv1alpha1.GetClientServiceName(etcd.ObjectMeta), etcd.Spec.Etcd.ClientUrlTLS, timeoutDeployJob)
			logger.Info("started running zero-downtime validator job")

			logger.Info("disrupting Etcd")
			numPodsToBeDeleted := tc.numPodsToBeDeleted
			if tc.numMembersToBeCorrupted > tc.numPodsToBeDeleted {
				numPodsToBeDeleted = tc.numMembersToBeCorrupted
			}
			testEnv.DisruptEtcd(g, etcd, numPodsToBeDeleted, tc.numMembersToBeCorrupted, timeoutEtcdDisruptionStart)
			logger.Info("successfully disrupted Etcd")

			logger.Info("waiting for Etcd to be ready again")
			testEnv.CheckEtcdReady(g, etcd, timeoutEtcdRecovery)
			logger.Info("Etcd is ready again")

			logger.Info("checking if downtime occurred")
			testEnv.CheckForDowntime(g, testNamespace, tc.expectDowntime)
			logger.Info("successfully checked if downtime occurred")

			logger.Info("finished running tests")

			cleanupTestArtifactsIfNecessary(logger, g, testNamespace, etcd)
		})
	}
}

// TODO: complete me
// TestClusterUpdates tests for zero downtime during cluster updates.
//func TestClusterUpdates(t *testing.T) {
//}

// TODO: complete me
// TestClusterMaintenance tests for zero downtime during cluster maintenance.
//func TestClusterMaintenance(t *testing.T) {
//}

// TODO: complete me
// TestCertificateRotation tests for TLS certificate rotations in an Etcd cluster.
//func TestCertificateRotation(t *testing.T) {
//}

// TODO: complete me
// TestScaleOutWithExternalTSCInjection tests scale out of an Etcd cluster from 1 -> 3 replicas with
// external injection of Topology Spread Constraints into the pods.
//func TestScaleOutWithExternalTSCInjection(t *testing.T) {
//}

func initializeTestCase(g *WithT, logger logr.Logger, testNamespace string) {
	createNamespace(g, logger, testNamespace)
	etcdCertsDir, etcdPeerCertsDir, etcdbrCertsDir := generatePKIResources(g, logger, testNamespace)
	createTLSSecrets(g, logger, testNamespace, etcdCertsDir, etcdPeerCertsDir, etcdbrCertsDir)
	createBackupSecret(g, logger, testNamespace)
}

func createNamespace(g *WithT, logger logr.Logger, testNamespace string) {
	logger.Info("creating test namespace")
	g.Expect(testEnv.CreateTestNamespace(testNamespace)).To(Succeed())
	logger.Info("successfully created test namespace")
}

func generatePKIResources(g *WithT, logger logr.Logger, testNamespace string) (string, string, string) {
	logger.Info("generating PKI resources")
	certDir := fmt.Sprintf("%s/%s", pkiResourcesDir, testNamespace)
	// certs for etcd server-client communication
	etcdCertsDir := fmt.Sprintf("%s/etcd", certDir)
	g.Expect(os.MkdirAll(etcdCertsDir, 0755)).To(Succeed())
	g.Expect(druide2etestutils.GeneratePKIResources(logger, etcdCertsDir, defaultEtcdName, testNamespace)).To(Succeed())
	// certs for etcd peer communication
	etcdPeerCertsDir := fmt.Sprintf("%s/etcd-peer", certDir)
	g.Expect(os.MkdirAll(etcdPeerCertsDir, 0755)).To(Succeed())
	g.Expect(druide2etestutils.GeneratePKIResources(logger, etcdPeerCertsDir, defaultEtcdName, testNamespace)).To(Succeed())
	// certs for etcd-backup-restore TLS
	etcdbrCertsDir := fmt.Sprintf("%s/etcd-backup-restore", certDir)
	g.Expect(os.MkdirAll(etcdbrCertsDir, 0755)).To(Succeed())
	g.Expect(druide2etestutils.GeneratePKIResources(logger, etcdbrCertsDir, defaultEtcdName, testNamespace)).To(Succeed())
	logger.Info("successfully generated PKI resources")
	return etcdCertsDir, etcdPeerCertsDir, etcdbrCertsDir
}

func createTLSSecrets(g *WithT, logger logr.Logger, testNamespace string, etcdCertsDir string, etcdPeerCertsDir string, etcdbrCertsDir string) {
	logger.Info("creating TLS secrets")
	// TLS secrets for etcd server-client communication
	g.Expect(druide2etestutils.CreateCASecret(testEnv.GetContext(), testEnv.GetClient(), testutils.ClientTLSCASecretName, testNamespace, etcdCertsDir)).To(Succeed())
	g.Expect(druide2etestutils.CreateServerTLSSecret(testEnv.GetContext(), testEnv.GetClient(), testutils.ClientTLSServerCertSecretName, testNamespace, etcdCertsDir)).To(Succeed())
	g.Expect(druide2etestutils.CreateClientTLSSecret(testEnv.GetContext(), testEnv.GetClient(), testutils.ClientTLSClientCertSecretName, testNamespace, etcdCertsDir)).To(Succeed())
	// TLS secrets for etcd peer communication
	g.Expect(druide2etestutils.CreateCASecret(testEnv.GetContext(), testEnv.GetClient(), testutils.PeerTLSCASecretName, testNamespace, etcdPeerCertsDir)).To(Succeed())
	g.Expect(druide2etestutils.CreateServerTLSSecret(testEnv.GetContext(), testEnv.GetClient(), testutils.PeerTLSServerCertSecretName, testNamespace, etcdPeerCertsDir)).To(Succeed())
	// TLS secrets for etcd-backup-restore TLS
	g.Expect(druide2etestutils.CreateCASecret(testEnv.GetContext(), testEnv.GetClient(), testutils.BackupRestoreTLSCASecretName, testNamespace, etcdbrCertsDir)).To(Succeed())
	g.Expect(druide2etestutils.CreateServerTLSSecret(testEnv.GetContext(), testEnv.GetClient(), testutils.BackupRestoreTLSServerCertSecretName, testNamespace, etcdbrCertsDir)).To(Succeed())
	g.Expect(druide2etestutils.CreateClientTLSSecret(testEnv.GetContext(), testEnv.GetClient(), testutils.BackupRestoreTLSClientCertSecretName, testNamespace, etcdbrCertsDir)).To(Succeed())
	logger.Info("successfully created TLS secrets")
}

func createBackupSecret(g *WithT, logger logr.Logger, namespace string) {
	logger.Info("creating backup secret")
	g.Expect(testutils.CreateBackupProviderLocalSecret(testEnv.GetContext(), testEnv.GetClient(), defaultBackupStoreSecretName, namespace)).To(Succeed())
	logger.Info("successfully created backup secret")
}

func updateEtcdTLSAndLabels(etcd *druidv1alpha1.Etcd, clientTLSEnabled, peerTLSEnabled, backupRestoreTLSEnabled bool, labels map[string]string) {
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

	etcd.Spec.Labels = testutils.MergeMaps(etcd.Spec.Labels, labels)
}

func cleanupTestArtifactsIfNecessary(logger logr.Logger, g *WithT, ns string, etcd *druidv1alpha1.Etcd) {
	if !retainTestArtifacts {
		logger.Info("deleting test artifacts after running test")

		testEnv.DeleteAndCheckEtcd(g, logger, etcd, timeoutEtcdReconciliation)
		logger.Info("successfully deleted Etcd")

		g.Expect(testEnv.DeletePVCs(ns)).To(Succeed())
		logger.Info("successfully deleted PVCs in namespace")

		g.Expect(testEnv.DeleteTestNamespace(ns)).To(Succeed())
		logger.Info("successfully deleted namespace")
	}
}
