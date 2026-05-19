// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"context"
	"fmt"
	"os"
	"testing"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/test/e2e/testenv"
	e2eutils "github.com/gardener/etcd-druid/test/e2e/utils"
	testutils "github.com/gardener/etcd-druid/test/utils"

	"github.com/go-logr/logr/testr"
	"k8s.io/utils/ptr"

	. "github.com/onsi/gomega"
)

// TestMain sets up the test environment.
func TestMain(m *testing.M) {
	kubeconfigPath, err := testutils.GetEnvOrError(e2eutils.EnvKubeconfigPath)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "KUBECONFIG not provided: %v\n", err)
		os.Exit(1)
	}
	cl, err := e2eutils.GetKubernetesClient(kubeconfigPath)
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

	val, _ := testutils.GetEnvOrError(e2eutils.EnvRetainTestArtifacts)
	switch e2eutils.RetainTestArtifactsMode(val) {
	case e2eutils.RetainTestArtifactsAll:
		retainTestArtifacts = e2eutils.RetainTestArtifactsAll
	case e2eutils.RetainTestArtifactsFailed:
		retainTestArtifacts = e2eutils.RetainTestArtifactsFailed
	default:
		retainTestArtifacts = e2eutils.RetainTestArtifactsNone
	}

	backupProviders := testutils.GetEnvOrDefault(e2eutils.EnvBackupProviders, "none,local")
	providers, err = e2eutils.ParseBackupProviders(backupProviders)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Failed to parse backup providers: %v\n", err)
		os.Exit(1)
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
		purpose    string
		replicas   int32
		tlsEnabled bool
	}{
		{
			name:       "no-tls-0",
			purpose:    "test Etcd with 0 replicas and client, peer, backup-restore TLS disabled",
			tlsEnabled: false,
			replicas:   0,
		},
		{
			name:       "no-tls-1",
			purpose:    "test Etcd with 1 replica and client, peer, backup-restore TLS disabled",
			tlsEnabled: false,
			replicas:   1,
		},
		{
			name:       "no-tls-3",
			purpose:    "test Etcd with 3 replicas and client, peer, backup-restore TLS disabled",
			tlsEnabled: false,
			replicas:   3,
		},
		{
			name:       "tls-0",
			purpose:    "test Etcd with 0 replicas and client, peer, backup-restore TLS enabled",
			tlsEnabled: true,
			replicas:   0,
		},
		{
			name:       "tls-1",
			purpose:    "test Etcd with 1 replica and client, peer, backup-restore TLS enabled",
			tlsEnabled: true,
			replicas:   1,
		},
		{
			name:       "tls-3",
			purpose:    "test Etcd with 3 replicas and client, peer, backup-restore TLS enabled",
			tlsEnabled: true,
			replicas:   3,
		},
	}

	for _, provider := range providers {
		for _, tc := range testCases {
			tcName := fmt.Sprintf("basic-%s-%s", tc.name, e2eutils.GetProviderSuffix(provider))
			t.Run(tcName, func(t *testing.T) {
				t.Parallel()
				g := NewWithT(t)
				var testSucceeded bool // cannot use t.Failed() in deferred functions since it is evaluated at the end of the test

				testNamespace := testutils.GenerateTestNamespaceNameWithTestCaseName(t, testNamespacePrefix, tcName, 4)
				logger := log.WithName(tcName).WithValues("etcdName", e2eutils.DefaultEtcdName, "namespace", testNamespace)
				defer func() {
					e2eutils.CleanupTestArtifacts(retainTestArtifacts, testSucceeded, testEnv, logger, g, testNamespace)
				}()
				e2eutils.InitializeTestCase(g, testEnv, logger, testNamespace, e2eutils.DefaultEtcdName, provider)

				logger.Info("running tests", "purpose", tc.purpose)
				etcdBuilder := testutils.EtcdBuilderWithoutDefaults(e2eutils.DefaultEtcdName, testNamespace).
					WithReplicas(tc.replicas).
					WithDefaultBackup().
					WithStorageProvider(provider, fmt.Sprintf("%s/%s", testNamespace, e2eutils.DefaultEtcdName))

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

					logger.Info("unhibernating Etcd") // currently, unhibernation assumes the Etcd members' PVCs are intact
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
				testSucceeded = true
			})
		}
	}
}

// TestBootstrapWithExistingCluster verifies that a target Etcd configured with
// spec.etcd.bootstrapWithExistingCluster joins an already-running source Etcd
// and that the controller records the source-member inventory in status once
// the join succeeds. It also asserts that the BootstrappedWithExistingCluster
// condition is sticky once True and that JoinedAt is shared across entries and
// preserved on later reconciles.
func TestBootstrapWithExistingCluster(t *testing.T) {
	t.Parallel()
	log := testr.NewWithOptions(t, testr.Options{LogTimestamp: true})

	const (
		sourceEtcdName = "source"
		targetEtcdName = "target"
		clusterSize    = 3
	)

	for _, provider := range providers {
		tcName := fmt.Sprintf("bootstrap-existing-%s", getProviderSuffix(provider))
		t.Run(tcName, func(t *testing.T) {
			t.Parallel()
			g := NewWithT(t)
			var testSucceeded bool // cannot use t.Failed() in deferred functions since it is evaluated at the end of the test

			testNamespace := testutils.GenerateTestNamespaceNameWithTestCaseName(t, testNamespacePrefix, tcName, 4)
			logger := log.WithName(tcName).WithValues("namespace", testNamespace)
			defer func() {
				cleanupTestArtifacts(retainTestArtifacts, testSucceeded, testEnv, logger, g, testNamespace)
			}()
			initializeTestCase(g, testEnv, logger, testNamespace, defaultEtcdName, provider)

			logger.Info("creating 3-member source Etcd")
			sourceEtcd := testutils.EtcdBuilderWithoutDefaults(sourceEtcdName, testNamespace).
				WithReplicas(clusterSize).
				WithDefaultBackup().
				WithStorageProvider(provider, fmt.Sprintf("%s/%s", testNamespace, sourceEtcdName)).
				Build()
			testEnv.CreateAndCheckEtcd(g, sourceEtcd, timeoutEtcdCreation)
			logger.Info("successfully created source Etcd")

			// Build the list of source members the target must join. With a 3-member source the
			// initial-cluster view passed to the target contains every source pod's peer URL.
			sourceMembers := make([]druidv1alpha1.BootstrapExistingMember, 0, clusterSize)
			sourceMemberNames := make([]string, 0, clusterSize)
			sourcePeerURLs := make([]string, 0, clusterSize)
			for i := range clusterSize {
				name := fmt.Sprintf("%s-%d", sourceEtcdName, i)
				peerURL := fmt.Sprintf("http://%s-%d.%s-peer.%s.svc:2380", sourceEtcdName, i, sourceEtcdName, testNamespace)
				sourceMembers = append(sourceMembers, druidv1alpha1.BootstrapExistingMember{
					Name:     name,
					PeerURLs: []string{peerURL},
				})
				sourceMemberNames = append(sourceMemberNames, name)
				sourcePeerURLs = append(sourcePeerURLs, peerURL)
			}
			sourceClientEndpoint := fmt.Sprintf("http://%s-client.%s.svc:2379", sourceEtcdName, testNamespace)

			logger.Info("creating 3-member target Etcd configured with bootstrapWithExistingCluster")
			targetEtcd := testutils.EtcdBuilderWithoutDefaults(targetEtcdName, testNamespace).
				WithReplicas(clusterSize).
				WithDefaultBackup().
				WithStorageProvider(provider, fmt.Sprintf("%s/%s", testNamespace, targetEtcdName)).
				Build()
			targetEtcd.Spec.Etcd.BootstrapWithExistingCluster = &druidv1alpha1.BootstrapWithExistingCluster{
				Members:         sourceMembers,
				ClientEndpoints: []string{sourceClientEndpoint},
			}
			testEnv.CreateAndCheckEtcd(g, targetEtcd, timeoutEtcdCreation)
			logger.Info("successfully created target Etcd")

			logger.Info("waiting for BootstrappedWithExistingCluster=True and status snapshot to be recorded")
			g.Eventually(func(g Gomega) {
				etcd, err := testEnv.GetEtcd(targetEtcdName, testNamespace)
				g.Expect(err).NotTo(HaveOccurred())

				// The dedicated condition must reach True.
				g.Expect(etcd.Status.Conditions).To(ContainElement(Satisfy(func(condition druidv1alpha1.Condition) bool {
					return condition.Type == druidv1alpha1.ConditionTypeBootstrappedWithExistingCluster && condition.Status == druidv1alpha1.ConditionTrue
				})), "BootstrappedWithExistingCluster condition must reach True after the target joins")

				// Status snapshot must record all source members with their peer URLs
				// and a single non-zero JoinedAt.
				snap := etcd.Status.BootstrapWithExistingCluster
				g.Expect(snap).NotTo(BeNil())
				g.Expect(snap.JoinedAt.IsZero()).To(BeFalse(), "JoinedAt must be set when the snapshot is recorded")
				g.Expect(snap.Members).To(HaveLen(clusterSize))

				recordedNames := make([]string, 0, len(snap.Members))
				recordedPeerURLs := make([]string, 0, len(snap.Members))
				for _, m := range snap.Members {
					recordedNames = append(recordedNames, m.Name)
					recordedPeerURLs = append(recordedPeerURLs, m.PeerURLs...)
				}
				g.Expect(recordedNames).To(ConsistOf(sourceMemberNames))
				g.Expect(recordedPeerURLs).To(ConsistOf(sourcePeerURLs))
			}, timeoutEtcdCreation, timeoutEtcdDisruptionStart).Should(Succeed())
			logger.Info("BootstrappedWithExistingCluster=True and status snapshot recorded")

			// Capture the recorded JoinedAt to verify stickiness — later reconciles must
			// not overwrite the original timestamp.
			targetAfterJoin, err := testEnv.GetEtcd(targetEtcdName, testNamespace)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(targetAfterJoin.Status.BootstrapWithExistingCluster).NotTo(BeNil())
			originalJoinedAt := targetAfterJoin.Status.BootstrapWithExistingCluster.JoinedAt

			logger.Info("verifying that the condition stays True and JoinedAt is preserved across reconciles")
			g.Consistently(func(g Gomega) {
				etcd, err := testEnv.GetEtcd(targetEtcdName, testNamespace)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(etcd.Status.Conditions).To(ContainElement(Satisfy(func(condition druidv1alpha1.Condition) bool {
					return condition.Type == druidv1alpha1.ConditionTypeBootstrappedWithExistingCluster && condition.Status == druidv1alpha1.ConditionTrue
				})), "BootstrappedWithExistingCluster must remain True after the join completes (sticky success)")
				snap := etcd.Status.BootstrapWithExistingCluster
				g.Expect(snap).NotTo(BeNil())
				g.Expect(snap.Members).To(HaveLen(clusterSize))
				g.Expect(snap.JoinedAt).To(Equal(originalJoinedAt),
					"JoinedAt must not be rewritten on later reconciles")
			}, timeoutEtcdDisruptionStart, pollingInterval).Should(Succeed())
			logger.Info("condition and JoinedAt are stable")

			logger.Info("finished running bootstrapWithExistingCluster test")
			testSucceeded = true
		})
	}
}

// TestScaleOut tests scale out of an Etcd cluster from 1 -> 3 replicas along with label changes.
func TestScaleOut(t *testing.T) {
	t.Parallel()
	log := testr.NewWithOptions(t, testr.Options{LogTimestamp: true})

	testCases := []struct {
		name                          string
		purpose                       string
		peerTLSEnabledBeforeScaleOut  bool
		peerTLSEnabledAfterScaleOut   bool
		additionalLabelsAfterScaleOut map[string]string
	}{
		{
			name:    "basic",
			purpose: "test Etcd with TLS disabled before and after scale out, with no label changes",
		},
		{
			name:                        "enable-peer-tls",
			purpose:                     "test Etcd with peer TLS disabled before scale out and enabled after scale out, with no label changes",
			peerTLSEnabledAfterScaleOut: true,
		},
		{
			name:                         "with-peer-tls",
			purpose:                      "test Etcd with peer TLS enabled before and after scale out, with no label changes",
			peerTLSEnabledBeforeScaleOut: true,
			peerTLSEnabledAfterScaleOut:  true,
		},
		{
			name:                          "with-label-change",
			purpose:                       "test Etcd with TLS disabled before and after scale out, with label changes",
			additionalLabelsAfterScaleOut: map[string]string{"foo": "bar"},
		},
		{
			name:                          "enable-ptls-label-change",
			purpose:                       "test Etcd with peer TLS disabled before scale out and enabled after scale out, with label changes",
			peerTLSEnabledAfterScaleOut:   true,
			additionalLabelsAfterScaleOut: map[string]string{"foo": "bar"},
		},
	}

	for _, provider := range providers {
		for _, tc := range testCases {
			tcName := fmt.Sprintf("scaleout-%s-%s", tc.name, e2eutils.GetProviderSuffix(provider))
			t.Run(tcName, func(t *testing.T) {
				t.Parallel()
				g := NewWithT(t)
				var testSucceeded bool

				testNamespace := testutils.GenerateTestNamespaceNameWithTestCaseName(t, testNamespacePrefix, tcName, 4)
				logger := log.WithName(tcName).WithValues("etcdName", e2eutils.DefaultEtcdName, "namespace", testNamespace)
				defer func() {
					e2eutils.CleanupTestArtifacts(retainTestArtifacts, testSucceeded, testEnv, logger, g, testNamespace)
				}()
				e2eutils.InitializeTestCase(g, testEnv, logger, testNamespace, e2eutils.DefaultEtcdName, provider)

				logger.Info("running tests", "purpose", tc.purpose)
				etcdBuilder := testutils.EtcdBuilderWithoutDefaults(e2eutils.DefaultEtcdName, testNamespace).
					WithReplicas(1).
					WithClientTLS().
					WithDefaultBackup().
					WithBackupRestoreTLS().
					WithStorageProvider(provider, fmt.Sprintf("%s/%s", testNamespace, e2eutils.DefaultEtcdName))

				if tc.peerTLSEnabledBeforeScaleOut {
					etcdBuilder = etcdBuilder.WithPeerTLS()
				}
				etcd := etcdBuilder.Build()

				logger.Info("creating Etcd")
				testEnv.CreateAndCheckEtcd(g, etcd, timeoutEtcdCreation)
				logger.Info("successfully created Etcd")

				logger.Info("scaling out Etcd to 3 replicas")
				etcd.Spec.Replicas = 3
				updateEtcdTLSAndLabels(etcd, true, tc.peerTLSEnabledAfterScaleOut, true, tc.additionalLabelsAfterScaleOut)
				testEnv.UpdateAndCheckEtcd(g, etcd, timeoutEtcdUpdation)
				logger.Info("successfully scaled out Etcd to 3 replicas")

				logger.Info("finished running tests")
				testSucceeded = true
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
		purpose                             string
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
			purpose:                   "test Etcd with peer TLS disabled before update and enabled after update, with no other TLS changes and no label changes",
			peerTLSEnabledAfterUpdate: true,
		},
		{
			name:                        "label-change",
			purpose:                     "test Etcd with TLS disabled before and after update, with label changes",
			additionalLabelsAfterUpdate: map[string]string{"foo": "bar"},
		},
		{
			name:                               "enable-c-p-br-tls",
			purpose:                            "test Etcd with all TLS disabled before update and enabled after update, with no label changes",
			clientTLSEnabledAfterUpdate:        true,
			peerTLSEnabledAfterUpdate:          true,
			backupRestoreTLSEnabledAfterUpdate: true,
		},
		{
			name:                      "enable-ptls-label-change",
			purpose:                   "test Etcd with peer TLS disabled before update and enabled after update, with label changes",
			peerTLSEnabledAfterUpdate: true,
			additionalLabelsAfterUpdate: map[string]string{
				"foo": "bar",
			},
		},
		{
			name:                               "enable-c-p-br-label-change",
			purpose:                            "test Etcd with all TLS disabled before update and enabled after update, with label changes",
			clientTLSEnabledAfterUpdate:        true,
			peerTLSEnabledAfterUpdate:          true,
			backupRestoreTLSEnabledAfterUpdate: true,
			additionalLabelsAfterUpdate: map[string]string{
				"foo": "bar",
			},
		},
		{
			name:                                "disable-c-br-tls",
			purpose:                             "test Etcd with client and backup-restore TLS enabled before update and disabled after update, with no peer TLS changes and no label changes",
			clientTLSEnabledBeforeUpdate:        true,
			backupRestoreTLSEnabledBeforeUpdate: true,
		},
	}

	for _, provider := range providers {
		for _, tc := range testCases {
			tcName := fmt.Sprintf("update1-%s-%s", tc.name, e2eutils.GetProviderSuffix(provider))
			t.Run(tcName, func(t *testing.T) {
				t.Parallel()
				g := NewWithT(t)
				var testSucceeded bool

				testNamespace := testutils.GenerateTestNamespaceNameWithTestCaseName(t, testNamespacePrefix, tcName, 4)
				logger := log.WithName(tcName).WithValues("etcdName", e2eutils.DefaultEtcdName, "namespace", testNamespace)
				defer func() {
					e2eutils.CleanupTestArtifacts(retainTestArtifacts, testSucceeded, testEnv, logger, g, testNamespace)
				}()
				e2eutils.InitializeTestCase(g, testEnv, logger, testNamespace, e2eutils.DefaultEtcdName, provider)

				logger.Info("running tests", "purpose", tc.purpose)
				etcdBuilder := testutils.EtcdBuilderWithoutDefaults(e2eutils.DefaultEtcdName, testNamespace).
					WithReplicas(3).
					WithDefaultBackup().
					WithStorageProvider(provider, fmt.Sprintf("%s/%s", testNamespace, e2eutils.DefaultEtcdName))

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
				testSucceeded = true
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
		purpose                 string
		replicas                int32
		numMembersToBeCorrupted int
		numPodsToBeDeleted      int
		expectDowntime          bool
	}{
		{
			name:               "1-del-1-pod",
			purpose:            "test Etcd with 1 replica by deleting 1 pod",
			replicas:           1,
			numPodsToBeDeleted: 1,
			expectDowntime:     true,
		},
		{
			name:                    "1-corrupt-data",
			purpose:                 "test Etcd with 1 replica by corrupting data of 1 member",
			replicas:                1,
			numMembersToBeCorrupted: 1,
			expectDowntime:          true,
		},
		{
			name:               "3-del-1-pod",
			purpose:            "test Etcd with 3 replicas by deleting 1 pod",
			replicas:           3,
			numPodsToBeDeleted: 1,
			expectDowntime:     false,
		},
		{
			name:               "3-del-2-pods",
			purpose:            "test Etcd with 3 replicas by deleting 2 pods",
			replicas:           3,
			numPodsToBeDeleted: 2,
			expectDowntime:     true,
		},
		{
			name:               "3-del-3-pods",
			purpose:            "test Etcd with 3 replicas by deleting all 3 pods",
			replicas:           3,
			numPodsToBeDeleted: 3,
			expectDowntime:     true,
		},
		{
			name:                    "3-corrupt-1-mem",
			purpose:                 "test Etcd with 3 replicas by corrupting data of 1 member",
			replicas:                3,
			numMembersToBeCorrupted: 1,
			expectDowntime:          false,
		},
		{
			name:                    "3-del-2-pods-corrupt-1-mem",
			purpose:                 "test Etcd with 3 replicas by deleting 2 pods and corrupting data of 1 member",
			replicas:                3,
			numPodsToBeDeleted:      2,
			numMembersToBeCorrupted: 1,
			expectDowntime:          true,
		},
	}

	for _, provider := range providers {
		for _, tc := range testCases {
			tcName := fmt.Sprintf("recovery-%s-%s", tc.name, e2eutils.GetProviderSuffix(provider))
			t.Run(tcName, func(t *testing.T) {
				t.Parallel()
				g := NewWithT(t)
				var testSucceeded bool

				testNamespace := testutils.GenerateTestNamespaceNameWithTestCaseName(t, testNamespacePrefix, tcName, 4)
				logger := log.WithName(tcName).WithValues("etcdName", e2eutils.DefaultEtcdName, "namespace", testNamespace)
				defer func() {
					e2eutils.CleanupTestArtifacts(retainTestArtifacts, testSucceeded, testEnv, logger, g, testNamespace)
				}()
				e2eutils.InitializeTestCase(g, testEnv, logger, testNamespace, e2eutils.DefaultEtcdName, provider)

				logger.Info("running tests", "purpose", tc.purpose)
				etcdBuilder := testutils.EtcdBuilderWithoutDefaults(e2eutils.DefaultEtcdName, testNamespace).
					WithReplicas(tc.replicas).
					WithEtcdClientPort(ptr.To[int32](2379)).
					WithClientTLS().
					WithPeerTLS().
					WithGRPCGatewayEnabled().
					WithDefaultBackup().
					WithBackupRestoreTLS().
					WithStorageProvider(provider, fmt.Sprintf("%s/%s", testNamespace, e2eutils.DefaultEtcdName)).
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
				testSucceeded = true
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
		purpose        string
		replicas       int32
		updateSpec     bool
		expectDowntime bool
	}{
		{
			name:     "1-no-update",
			purpose:  "test Etcd with 1 replica without any spec updates",
			replicas: 1,
		},
		{
			name:           "1-update",
			purpose:        "test Etcd with 1 replica with spec updates",
			replicas:       1,
			updateSpec:     true,
			expectDowntime: true,
		},
		{
			name:     "3-no-update",
			purpose:  "test Etcd with 3 replicas without any spec updates",
			replicas: 3,
		},
		{
			name:       "3-update",
			purpose:    "test Etcd with 3 replicas with spec updates",
			replicas:   3,
			updateSpec: true,
		},
	}

	for _, provider := range providers {
		for _, tc := range testCases {
			tcName := fmt.Sprintf("update2-%s-%s", tc.name, e2eutils.GetProviderSuffix(provider))
			t.Run(tcName, func(t *testing.T) {
				t.Parallel()
				g := NewWithT(t)
				var testSucceeded bool

				testNamespace := testutils.GenerateTestNamespaceNameWithTestCaseName(t, testNamespacePrefix, tcName, 4)
				logger := log.WithName(tcName).WithValues("etcdName", e2eutils.DefaultEtcdName, "namespace", testNamespace)
				defer func() {
					e2eutils.CleanupTestArtifacts(retainTestArtifacts, testSucceeded, testEnv, logger, g, testNamespace)
				}()
				e2eutils.InitializeTestCase(g, testEnv, logger, testNamespace, e2eutils.DefaultEtcdName, provider)

				logger.Info("running tests", "purpose", tc.purpose)
				etcdBuilder := testutils.EtcdBuilderWithoutDefaults(e2eutils.DefaultEtcdName, testNamespace).
					WithReplicas(tc.replicas).
					WithEtcdClientPort(ptr.To[int32](2379)).
					WithClientTLS().
					WithPeerTLS().
					WithGRPCGatewayEnabled().
					WithDefaultBackup().
					WithBackupRestoreTLS().
					WithStorageProvider(provider, fmt.Sprintf("%s/%s", testNamespace, e2eutils.DefaultEtcdName))
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
				testSucceeded = true
			})
		}
	}
}
