// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/test/e2e/testenv"
	druide2etestutils "github.com/gardener/etcd-druid/test/e2e/utils"
	testutils "github.com/gardener/etcd-druid/test/utils"

	"github.com/go-logr/logr/testr"
	"k8s.io/utils/ptr"

	. "github.com/onsi/gomega"
)

const (
	// environment variables
	envKubeconfigPath      = "KUBECONFIG"
	envRetainTestArtifacts = "RETAIN_TEST_ARTIFACTS"
	envBackupProviders     = "PROVIDERS"

	// test parameters
	timeoutTest                = 1 * time.Hour
	timeoutEtcdCreation        = 5 * time.Minute
	timeoutEtcdDeletion        = 2 * time.Minute
	timeoutEtcdHibernation     = 2 * time.Minute
	timeoutEtcdUnhibernation   = 5 * time.Minute
	timeoutEtcdUpdation        = 10 * time.Minute
	timeoutEtcdDisruptionStart = 30 * time.Second
	timeoutEtcdRecovery        = 5 * time.Minute
	timeoutDeployJob           = 2 * time.Minute

	testNamespacePrefix = "etcd-e2e"
	defaultEtcdName     = "test"
)

var (
	testEnv             *testenv.TestEnvironment
	retainTestArtifacts = false
	providers           = []druidv1alpha1.StorageProvider{"none"}
)

// TestMain sets up the test environment.
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

	// default to {"none"} if no providers are specified
	backupProviders, err := druide2etestutils.GetEnvOrError(envBackupProviders)
	if err == nil {
		providers = druide2etestutils.ParseBackupProviders(backupProviders)
	}

	exitCode := m.Run()

	testEnv.Close()
	os.Exit(exitCode)
}

// TestBasic tests creation, hibernation, unhibernation and deletion of the Etcd cluster.
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

	for _, provider := range providers {
		for _, tc := range testCases {
			tcName := fmt.Sprintf("basic-%s-%s", tc.name, provider)
			t.Run(tcName, func(t *testing.T) {
				t.Parallel()

				testNamespace := testutils.GenerateTestNamespaceNameWithTestCaseName(t, testNamespacePrefix, tcName, 4)
				logger := log.WithName(tcName).WithValues("etcdName", defaultEtcdName, "namespace", testNamespace)
				defer cleanupTestArtifacts(!retainTestArtifacts, testEnv, logger, g, testNamespace)

				initializeTestCase(g, testEnv, logger, testNamespace, defaultEtcdName)

				logger.Info("running tests")
				etcdBuilder := testutils.EtcdBuilderWithoutDefaults(defaultEtcdName, testNamespace).
					WithReplicas(tc.replicas).
					WithDefaultBackup().
					WithStorageProvider(provider, fmt.Sprintf("%s/%s", testNamespace, defaultEtcdName))

				if tc.tlsEnabled {
					etcdBuilder = etcdBuilder.WithClientTLS().
						WithPeerTLS().
						WithBackupRestoreTLS()
				}
				etcd := etcdBuilder.Build()

				logger.Info("creating Etcd")
				testEnv.CreateAndCheckEtcd(g, etcd, timeoutEtcdCreation)
				logger.Info("successfully created Etcd")

				if tc.replicas != 0 {
					logger.Info("hibernating Etcd")
					replicas := etcd.Spec.Replicas
					testEnv.HibernateAndCheckEtcd(g, etcd, timeoutEtcdHibernation)
					logger.Info("successfully hibernated Etcd")

					logger.Info("unhibernating Etcd")
					etcd, err := testEnv.GetEtcd(etcd.Name, etcd.Namespace)
					g.Expect(err).ToNot(HaveOccurred())
					testEnv.UnhibernateAndCheckEtcd(g, etcd, replicas, timeoutEtcdUnhibernation)
					logger.Info("successfully unhibernated Etcd")
				}

				logger.Info("deleting Etcd")
				etcd, err := testEnv.GetEtcd(etcd.Name, etcd.Namespace)
				g.Expect(err).ToNot(HaveOccurred())
				testEnv.DeleteAndCheckEtcd(g, logger, etcd, timeoutEtcdDeletion)
				logger.Info("successfully deleted Etcd")

				logger.Info("finished running tests")
			})
		}
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
		{
			name:                "with-label-change",
			labelsAfterScaleOut: map[string]string{"foo": "bar"},
		},
		{
			name:                        "enable-ptls-label-change",
			peerTLSEnabledAfterScaleOut: true,
			labelsAfterScaleOut:         map[string]string{"foo": "bar"},
		},
		// TODO: enable this once disabling peer TLS is supported
		//{
		//	name:                         "disable-peer-tls",
		//	peerTLSEnabledBeforeScaleOut: true,
		//},
	}

	g := NewWithT(t)

	for _, provider := range providers {
		for _, tc := range testCases {
			tcName := fmt.Sprintf("scaleout-%s-%s", tc.name, provider)
			t.Run(tcName, func(t *testing.T) {
				t.Parallel()

				testNamespace := testutils.GenerateTestNamespaceNameWithTestCaseName(t, testNamespacePrefix, tcName, 4)
				logger := log.WithName(tcName).WithValues("etcdName", defaultEtcdName, "namespace", testNamespace)
				defer cleanupTestArtifacts(!retainTestArtifacts, testEnv, logger, g, testNamespace)

				initializeTestCase(g, testEnv, logger, testNamespace, defaultEtcdName)

				logger.Info("running tests")
				etcdBuilder := testutils.EtcdBuilderWithoutDefaults(defaultEtcdName, testNamespace).
					WithReplicas(1).
					WithClientTLS().
					WithDefaultBackup().
					WithBackupRestoreTLS().
					WithStorageProvider(provider, fmt.Sprintf("%s/%s", testNamespace, defaultEtcdName))

				if tc.peerTLSEnabledBeforeScaleOut {
					etcdBuilder = etcdBuilder.WithPeerTLS()
				}
				if tc.labelsBeforeScaleOut != nil {
					etcdBuilder = etcdBuilder.WithSpecLabels(tc.labelsBeforeScaleOut)
				}
				etcd := etcdBuilder.Build()

				logger.Info("creating Etcd")
				testEnv.CreateAndCheckEtcd(g, etcd, timeoutEtcdCreation)
				logger.Info("successfully created Etcd")

				logger.Info("scaling out Etcd to 3 replicas")
				etcd.Spec.Replicas = 3
				updateEtcdTLSAndLabels(etcd, false, tc.peerTLSEnabledAfterScaleOut, false, tc.labelsAfterScaleOut)
				testEnv.UpdateAndCheckEtcd(g, etcd, timeoutEtcdUpdation)
				logger.Info("successfully scaled out Etcd to 3 replicas")

				logger.Info("finished running tests")
			})
		}
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
		additionalLabelsAfterUpdate         map[string]string
	}{
		{
			name:                      "enable-peer",
			peerTLSEnabledAfterUpdate: true,
		},
		{
			name:                        "label-change",
			additionalLabelsAfterUpdate: map[string]string{"foo": "bar"},
		},
		{
			name:                               "enable-c-p-br-tls",
			clientTLSEnabledAfterUpdate:        true,
			peerTLSEnabledAfterUpdate:          true,
			backupRestoreTLSEnabledAfterUpdate: true,
		},
		{
			name:                      "enable-ptls-label-change",
			peerTLSEnabledAfterUpdate: true,
			additionalLabelsAfterUpdate: map[string]string{
				"foo": "bar",
			},
		},
		{
			name:                               "enable-c-p-br-label-change",
			clientTLSEnabledAfterUpdate:        true,
			peerTLSEnabledAfterUpdate:          true,
			backupRestoreTLSEnabledAfterUpdate: true,
			additionalLabelsAfterUpdate: map[string]string{
				"foo": "bar",
			},
		},
		{
			name:                                "disable-c-br-tls",
			clientTLSEnabledBeforeUpdate:        true,
			backupRestoreTLSEnabledBeforeUpdate: true,
		},
		// TODO: enable these once disabling peer TLS is supported
		//{
		//	name:                       "disable-p-tls",
		//	peerTLSEnabledBeforeUpdate: true,
		//},
		//{
		//	name:                                "disable-c-p-br-tls",
		//	clientTLSEnabledBeforeUpdate:        true,
		//	peerTLSEnabledBeforeUpdate:          true,
		//	backupRestoreTLSEnabledBeforeUpdate: true,
		//},
		//{
		//	name:                       "disable-ptls-label-change",
		//	peerTLSEnabledBeforeUpdate: true,
		//	additionalLabelsAfterUpdate: map[string]string{
		//		"foo": "bar",
		//	},
		//},
	}

	g := NewWithT(t)

	for _, provider := range providers {
		for _, tc := range testCases {
			tcName := fmt.Sprintf("update1-%s-%s", tc.name, provider)
			t.Run(tcName, func(t *testing.T) {
				t.Parallel()

				testNamespace := testutils.GenerateTestNamespaceNameWithTestCaseName(t, testNamespacePrefix, tcName, 4)
				logger := log.WithName(tcName).WithValues("etcdName", defaultEtcdName, "namespace", testNamespace)
				defer cleanupTestArtifacts(!retainTestArtifacts, testEnv, logger, g, testNamespace)

				initializeTestCase(g, testEnv, logger, testNamespace, defaultEtcdName)

				logger.Info("running tests")
				etcdBuilder := testutils.EtcdBuilderWithoutDefaults(defaultEtcdName, testNamespace).
					WithReplicas(3).
					WithDefaultBackup().
					WithStorageProvider(provider, fmt.Sprintf("%s/%s", testNamespace, defaultEtcdName))

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
				testEnv.CreateAndCheckEtcd(g, etcd, timeoutEtcdCreation)
				logger.Info("successfully created Etcd")

				logger.Info("updating Etcd")
				updateEtcdTLSAndLabels(etcd, tc.clientTLSEnabledAfterUpdate, tc.peerTLSEnabledAfterUpdate, tc.backupRestoreTLSEnabledAfterUpdate, tc.additionalLabelsAfterUpdate)
				testEnv.UpdateAndCheckEtcd(g, etcd, timeoutEtcdUpdation)
				logger.Info("successfully updated Etcd")

				logger.Info("verifying labels on Etcd pods")
				testEnv.VerifyEtcdPodLabels(g, etcd, etcd.Spec.Labels)
				logger.Info("successfully verified labels on Etcd pods")

				if tc.peerTLSEnabledAfterUpdate {
					logger.Info("verifying peer TLS enablement for Etcd members")
					testEnv.VerifyEtcdMemberPeerTLSEnabled(g, etcd)
					logger.Info("successfully verified peer TLS enablement for Etcd members")
				}

				logger.Info("finished running tests")
			})
		}
	}
}

// TestRecovery tests for recovery of an Etcd cluster upon transient failures.
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
			name:               "1-del-1-pod",
			replicas:           1,
			numPodsToBeDeleted: 1,
			expectDowntime:     true,
		},
		{
			name:                    "1-corrupt-data",
			replicas:                1,
			numMembersToBeCorrupted: 1,
			expectDowntime:          true,
		},
		{
			name:               "3-del-1-pod",
			replicas:           3,
			numPodsToBeDeleted: 1,
			expectDowntime:     false,
		},
		{
			name:               "3-del-2-pods",
			replicas:           3,
			numPodsToBeDeleted: 2,
			expectDowntime:     true,
		},
		{
			name:               "3-del-3-pods",
			replicas:           3,
			numPodsToBeDeleted: 3,
			expectDowntime:     true,
		},
		{
			name:                    "3-corrupt-1-mem",
			replicas:                3,
			numMembersToBeCorrupted: 1,
			expectDowntime:          false,
		},
		{
			name:                    "3-del-2-pods-corrupt-1-mem",
			replicas:                3,
			numPodsToBeDeleted:      2,
			numMembersToBeCorrupted: 1,
			expectDowntime:          true,
		},
	}

	g := NewWithT(t)

	for _, provider := range providers {
		for _, tc := range testCases {
			tcName := fmt.Sprintf("recovery-%s-%s", tc.name, provider)
			t.Run(tcName, func(t *testing.T) {
				t.Parallel()

				testNamespace := testutils.GenerateTestNamespaceNameWithTestCaseName(t, testNamespacePrefix, tcName, 4)
				logger := log.WithName(tcName).WithValues("etcdName", defaultEtcdName, "namespace", testNamespace)
				defer cleanupTestArtifacts(!retainTestArtifacts, testEnv, logger, g, testNamespace)

				initializeTestCase(g, testEnv, logger, testNamespace, defaultEtcdName)

				logger.Info("running tests")
				etcdBuilder := testutils.EtcdBuilderWithoutDefaults(defaultEtcdName, testNamespace).
					WithReplicas(tc.replicas).
					WithEtcdClientPort(ptr.To[int32](2379)).
					WithClientTLS().
					WithPeerTLS().
					WithGRPCGatewayEnabled().
					WithDefaultBackup().
					WithBackupRestoreTLS().
					WithStorageProvider(provider, fmt.Sprintf("%s/%s", testNamespace, defaultEtcdName)).
					WithComponentProtectionDisabled()
				etcd := etcdBuilder.Build()

				logger.Info("creating Etcd")
				testEnv.CreateAndCheckEtcd(g, etcd, timeoutEtcdCreation)
				logger.Info("successfully created Etcd")

				logger.Info("starting zero-downtime validator job")
				testEnv.DeployZeroDowntimeValidatorJob(g, testNamespace, druidv1alpha1.GetClientServiceName(etcd.ObjectMeta), *etcd.Spec.Etcd.ClientPort, etcd.Spec.Etcd.ClientUrlTLS, timeoutDeployJob)
				logger.Info("started running zero-downtime validator job")

				logger.Info("disrupting Etcd")
				numPodsToBeDeleted := max(tc.numPodsToBeDeleted, tc.numMembersToBeCorrupted)
				testEnv.DisruptEtcd(g, etcd, numPodsToBeDeleted, tc.numMembersToBeCorrupted, timeoutEtcdDisruptionStart)
				logger.Info("successfully disrupted Etcd")

				logger.Info("waiting for Etcd to be ready again")
				testEnv.CheckEtcdReady(g, etcd, timeoutEtcdRecovery)
				logger.Info("Etcd is ready again")

				logger.Info("checking if downtime occurred")
				testEnv.CheckForDowntime(g, testNamespace, tc.expectDowntime)
				logger.Info("successfully checked if downtime occurred")

				logger.Info("finished running tests")
			})
		}
	}
}

// TestClusterUpdate tests for zero downtime during cluster updates.
func TestClusterUpdate(t *testing.T) {
	t.Parallel()
	log := testr.NewWithOptions(t, testr.Options{LogTimestamp: true})

	testCases := []struct {
		name           string
		replicas       int32
		updateSpec     bool
		expectDowntime bool
	}{
		{
			name:     "1-no-update",
			replicas: 1,
		},
		{
			name:           "1-update",
			replicas:       1,
			updateSpec:     true,
			expectDowntime: true,
		},
		{
			name:     "3-no-update",
			replicas: 3,
		},
		{
			name:       "3-update",
			replicas:   3,
			updateSpec: true,
		},
	}

	g := NewWithT(t)

	for _, provider := range providers {
		for _, tc := range testCases {
			tcName := fmt.Sprintf("update2-%s-%s", tc.name, provider)
			t.Run(tcName, func(t *testing.T) {
				t.Parallel()

				testNamespace := testutils.GenerateTestNamespaceNameWithTestCaseName(t, testNamespacePrefix, tcName, 4)
				logger := log.WithName(tcName).WithValues("etcdName", defaultEtcdName, "namespace", testNamespace)
				defer cleanupTestArtifacts(!retainTestArtifacts, testEnv, logger, g, testNamespace)

				initializeTestCase(g, testEnv, logger, testNamespace, defaultEtcdName)

				logger.Info("running tests")
				etcdBuilder := testutils.EtcdBuilderWithoutDefaults(defaultEtcdName, testNamespace).
					WithReplicas(tc.replicas).
					WithEtcdClientPort(ptr.To[int32](2379)).
					WithClientTLS().
					WithPeerTLS().
					WithGRPCGatewayEnabled().
					WithDefaultBackup().
					WithBackupRestoreTLS().
					WithStorageProvider(provider, fmt.Sprintf("%s/%s", testNamespace, defaultEtcdName))
				etcd := etcdBuilder.Build()

				logger.Info("creating Etcd")
				testEnv.CreateAndCheckEtcd(g, etcd, timeoutEtcdCreation)
				logger.Info("successfully created Etcd")

				logger.Info("starting zero-downtime validator job")
				testEnv.DeployZeroDowntimeValidatorJob(g, testNamespace, druidv1alpha1.GetClientServiceName(etcd.ObjectMeta), *etcd.Spec.Etcd.ClientPort, etcd.Spec.Etcd.ClientUrlTLS, timeoutDeployJob)
				logger.Info("started running zero-downtime validator job")

				etcd, err := testEnv.GetEtcd(etcd.Name, etcd.Namespace)
				g.Expect(err).ToNot(HaveOccurred())
				if tc.updateSpec {
					logger.Info("updating Etcd spec")
					etcd.Spec.Etcd.Metrics = ptr.To(druidv1alpha1.Extensive)
					testEnv.UpdateAndCheckEtcd(g, etcd, timeoutEtcdUpdation)
					logger.Info("successfully updated and reconciled Etcd spec")
				}

				logger.Info("checking if downtime occurred")
				testEnv.CheckForDowntime(g, testNamespace, tc.expectDowntime)
				logger.Info("successfully checked if downtime occurred")

				logger.Info("finished running tests")
			})
		}
	}
}

// TestClusterMaintenance tests for zero downtime during cluster maintenance.
// TODO: complete me once EtcdOpsTask for on-demand defragmentation is implemented.
// This test is currently not implemented since etcd-druid does not control defragmentation
// operations on the etcd members, and that is managed by the backup sidecars themselves.
//func TestClusterMaintenance(t *testing.T) {
//	t.Parallel()
//	log := testr.NewWithOptions(t, testr.Options{LogTimestamp: true})
//
//	testCases := []struct {
//		name           string
//		replicas       int32
//		expectDowntime bool
//	}{
//		{
//			name:           "1-defrag",
//			replicas:       1,
//			expectDowntime: true,
//		},
//		{
//			name:     "3-defrag",
//			replicas: 3,
//		},
//	}
//
//	g := NewWithT(t)
//
//	for _, provider := range providers {
//		for _, tc := range testCases {
//			tcName := fmt.Sprintf("maintenance-%s-%s", tc.name, provider)
//			t.Run(tcName, func(t *testing.T) {
//				t.Parallel()
//
//				testNamespace := testutils.GenerateTestNamespaceNameWithTestCaseName(t, testNamespacePrefix, tcName, 4)
//				logger := log.WithName(tcName).WithValues("etcdName", defaultEtcdName, "namespace", testNamespace)
//              defer cleanupTestArtifacts(!retainTestArtifacts, testEnv, logger, g, testNamespace)
//
//              initializeTestCase(g, testEnv, logger, testNamespace, defaultEtcdName)
//
//				logger.Info("running tests")
//				etcdBuilder := testutils.EtcdBuilderWithoutDefaults(defaultEtcdName, testNamespace).
//					WithReplicas(tc.replicas).
//					WithGRPCGatewayEnabled()
//				etcd := etcdBuilder.Build()
//
//				logger.Info("creating Etcd")
//				testEnv.CreateAndCheckEtcd(g, etcd, timeoutEtcdCreation)
//				logger.Info("successfully created Etcd")
//
//				logger.Info("starting zero-downtime validator job")
//				testEnv.DeployZeroDowntimeValidatorJob(g, testNamespace, druidv1alpha1.GetClientServiceName(etcd.ObjectMeta), etcd.Spec.Etcd.ClientUrlTLS, timeoutDeployJob)
//				logger.Info("started running zero-downtime validator job")
//
//				// deploy EtcdOpsTask for on-demand defragmentation.
//				// check that eventually its status should say Succeeded.
//
//				logger.Info("checking if downtime occurred")
//				testEnv.CheckForDowntime(g, testNamespace, tc.expectDowntime)
//				logger.Info("successfully checked if downtime occurred")
//
//				logger.Info("finished running tests")
//			})
//		}
//	}
//}

// TODO: complete me when we support 3-node KIND clusters for e2e testing
// TestScaleOutWithExternalTSCInjection tests scale out of an Etcd cluster from 1 -> 3 replicas with
// external injection of Topology Spread Constraints into the pods.
//func TestScaleOutWithExternalTSCInjection(t *testing.T) {
//}
