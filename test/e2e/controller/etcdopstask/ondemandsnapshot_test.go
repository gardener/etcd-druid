// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package etcdopstask

import (
	"fmt"
	"testing"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	e2eutils "github.com/gardener/etcd-druid/test/e2e/utils"
	testutils "github.com/gardener/etcd-druid/test/utils"

	"github.com/go-logr/logr/testr"
	"k8s.io/utils/ptr"

	. "github.com/onsi/gomega"
)

// TestOnDemandSnapshotLifecycle validates the complete lifecycle of OnDemandSnapshot tasks.
func TestOnDemandSnapshotLifecycle(t *testing.T) {
	t.Parallel()
	log := testr.NewWithOptions(t, testr.Options{LogTimestamp: true})

	testCases := []struct {
		name            string
		purpose         string
		snapshotType    druidv1alpha1.OnDemandSnapshotType
		backupEnabled   bool
		replicas        int32
		loadKeys        int64
		needsFullFirst  bool
		disruptEtcd     bool
		deleteMidFlight bool
		expectedState   druidv1alpha1.TaskState
		checkLease      bool
	}{
		{
			name:          "rejected-backup-not-enabled",
			purpose:       "test Rejected state of OnDemandSnapshot when backup is not enabled",
			snapshotType:  druidv1alpha1.OnDemandSnapshotTypeFull,
			backupEnabled: false,
			replicas:      1,
			expectedState: druidv1alpha1.TaskStateRejected,
		},
		{
			name:          "rejected-etcd-not-ready",
			purpose:       "test Rejected state of OnDemandSnapshot when Etcd is not in Ready state",
			snapshotType:  druidv1alpha1.OnDemandSnapshotTypeFull,
			backupEnabled: true,
			replicas:      0,
			expectedState: druidv1alpha1.TaskStateRejected,
		},
		{
			name:          "full-snapshot-fails-etcd-disrupted",
			purpose:       "test Failed state of full OnDemandSnapshot when etcd is disrupted by scaling down StatefulSet to 0 mid-execution",
			snapshotType:  druidv1alpha1.OnDemandSnapshotTypeFull,
			backupEnabled: true,
			replicas:      1,
			loadKeys:      5000,
			disruptEtcd:   true,
			expectedState: druidv1alpha1.TaskStateFailed,
		},
		{
			name:          "full-snapshot-succeeds",
			purpose:       "test Succeeded state of full OnDemandSnapshot on completion of full snapshot",
			snapshotType:  druidv1alpha1.OnDemandSnapshotTypeFull,
			backupEnabled: true,
			replicas:      1,
			loadKeys:      1000,
			expectedState: druidv1alpha1.TaskStateSucceeded,
			checkLease:    true,
		},
		{
			name:           "delta-snapshot-succeeds",
			purpose:        "test Succeeded state of delta OnDemandSnapshot on completion of delta snapshot",
			snapshotType:   druidv1alpha1.OnDemandSnapshotTypeDelta,
			backupEnabled:  true,
			replicas:       1,
			loadKeys:       1000,
			needsFullFirst: true,
			expectedState:  druidv1alpha1.TaskStateSucceeded,
			checkLease:     true,
		},
		{
			name:           "delta-snapshot-skipped-no-events",
			purpose:        "test Succeeded state of delta OnDemandSnapshot when the delta snapshot is skipped by backup-restore server",
			snapshotType:   druidv1alpha1.OnDemandSnapshotTypeDelta,
			backupEnabled:  true,
			replicas:       1,
			needsFullFirst: true,
			expectedState:  druidv1alpha1.TaskStateSucceeded,
		},
		{
			name:            "mid-flight-deletion-cleans-up",
			purpose:         "test cleanup when deletion is triggered mid-execution",
			snapshotType:    druidv1alpha1.OnDemandSnapshotTypeFull,
			backupEnabled:   true,
			replicas:        1,
			loadKeys:        5000,
			deleteMidFlight: true,
		},
	}

	for _, provider := range providers {
		if provider == "none" {
			continue
		}

		for _, tc := range testCases {
			tcName := fmt.Sprintf("ondemsnap-%s-%s", tc.name, e2eutils.GetProviderSuffix(provider))
			t.Run(tcName, func(t *testing.T) {
				t.Parallel()
				g := NewWithT(t)
				var testSucceeded bool

				testNamespace := testutils.GenerateTestNamespaceNameWithTestCaseName(t, testNamespacePrefix, tcName, 4)
				logger := log.WithName(tcName).WithValues("etcdName", e2eutils.DefaultEtcdName, "namespace", testNamespace)
				defer func() {
					e2eutils.CleanupTestArtifacts(retainTestArtifacts, testSucceeded, testEnv, logger, g, testNamespace)
				}()

				logger.Info("running tests", "purpose", tc.purpose)

				if tc.backupEnabled {
					e2eutils.InitializeTestCase(g, testEnv, logger, testNamespace, e2eutils.DefaultEtcdName, provider)
				} else {
					e2eutils.CreateNamespace(g, testEnv, logger, testNamespace)
					etcdCertsDir, etcdPeerCertsDir, etcdbrCertsDir := e2eutils.GeneratePKIResources(g, logger, testNamespace, e2eutils.DefaultEtcdName)
					e2eutils.CreateTLSSecrets(g, testEnv, logger, testNamespace, etcdCertsDir, etcdPeerCertsDir, etcdbrCertsDir)
				}

				etcdBuilder := testutils.EtcdBuilderWithoutDefaults(e2eutils.DefaultEtcdName, testNamespace).
					WithReplicas(tc.replicas).
					WithEtcdClientPort(ptr.To[int32](2379)).
					WithClientTLS().
					WithPeerTLS().
					WithGRPCGatewayEnabled()

				if tc.backupEnabled {
					etcdBuilder = etcdBuilder.
						WithDefaultBackup().
						WithStorageProvider(provider, fmt.Sprintf("%s/%s", testNamespace, e2eutils.DefaultEtcdName)).
						WithBackupRestoreTLS()
				}
				if tc.disruptEtcd {
					etcdBuilder = etcdBuilder.WithComponentProtectionDisabled()
				}
				etcd := etcdBuilder.Build()

				logger.Info("creating Etcd resource")
				if tc.backupEnabled && tc.replicas > 0 {
					testEnv.CreateAndCheckEtcd(g, etcd, timeoutEtcdCreation)
				} else {
					g.Expect(testEnv.Client().Create(testEnv.Context(), etcd)).To(Succeed())
				}
				logger.Info("successfully created Etcd resource")

				if tc.needsFullFirst {
					logger.Info("taking initial full snapshot")
					testEnv.TakeFullSnapshot(g, etcd, timeoutFullSnapshot)
					logger.Info("full snapshot taken successfully")
				}

				if tc.loadKeys > 0 {
					valueSizeBytes := int64(0)
					loadTimeout := timeoutDeployJob
					if tc.disruptEtcd || tc.deleteMidFlight {
						valueSizeBytes = 256 * 1024
						loadTimeout = timeoutLargeDeployJob
					}
					logger.Info("loading data into etcd", "keys", tc.loadKeys, "valueSizeBytes", valueSizeBytes)
					testEnv.DeployEtcdLoaderJob(g, testNamespace, druidv1alpha1.GetClientServiceName(etcd.ObjectMeta), *etcd.Spec.Etcd.ClientPort, etcd.Spec.Etcd.ClientUrlTLS, tc.loadKeys, valueSizeBytes, loadTimeout)
				}

				var fullRevBefore, deltaRevBefore int64
				if tc.checkLease {
					var err error
					fullRevBefore, deltaRevBefore, err = testEnv.GetSnapshotRevisions(etcd.ObjectMeta)
					g.Expect(err).NotTo(HaveOccurred())
					logger.Info("snapshot revisions before task", "fullRev", fullRevBefore, "deltaRev", deltaRevBefore)
				}

				taskName := fmt.Sprintf("%s-%s-task", e2eutils.DefaultEtcdName, tc.name)
				var task *druidv1alpha1.EtcdOpsTask
				if tc.deleteMidFlight {
					// Long TTL ensures any deletion observed within the assertion window is driven by the manual-deletion flow, not by TTL-based reconciliation.
					task = testutils.EtcdOpsTaskBuilderWithoutDefaults(taskName, testNamespace).
						WithEtcdName(e2eutils.DefaultEtcdName).
						WithTTLSecondsAfterFinished(int32(3600)).
						WithOnDemandSnapshotConfig(&druidv1alpha1.OnDemandSnapshotConfig{Type: tc.snapshotType}).
						Build()
				} else {
					task = newOnDemandSnapshotTask(taskName, testNamespace, e2eutils.DefaultEtcdName, tc.snapshotType)
				}

				logger.Info("creating EtcdOpsTask", "taskName", taskName, "snapshotType", tc.snapshotType)
				testEnv.CreateEtcdOpsTask(g, task)

				if tc.disruptEtcd {
					logger.Info("waiting for task to reach InProgress before disrupting etcd")
					testEnv.CheckEtcdOpsTaskState(g, task, druidv1alpha1.TaskStateInProgress, timeoutEtcdOpsTaskCompletion)
					logger.Info("scaling StatefulSet to 0 to disrupt etcd")
					e2eutils.ScaleStatefulSetToZero(g, testEnv, etcd)
					logger.Info("StatefulSet scaled to 0")
				}

				if tc.deleteMidFlight {
					logger.Info("manually deleting EtcdOpsTask mid-execution")
					testEnv.DeleteEtcdOpsTask(g, task)
					logger.Info("verifying task is removed regardless of in-flight state")
					testEnv.CheckEtcdOpsTaskDeleted(g, task, timeoutEtcdOpsTaskCompletion)
					logger.Info("task removed via manual deletion")
					testSucceeded = true
					return
				}

				logger.Info("waiting for EtcdOpsTask to reach expected state", "expectedState", tc.expectedState)
				testEnv.CheckEtcdOpsTaskState(g, task, tc.expectedState, timeoutEtcdOpsTaskCompletion)
				logger.Info("EtcdOpsTask reached expected state")

				if tc.checkLease {
					g.Eventually(func() bool {
						fullRevAfter, deltaRevAfter, err := testEnv.GetSnapshotRevisions(etcd.ObjectMeta)
						if err != nil {
							return false
						}
						if tc.snapshotType == druidv1alpha1.OnDemandSnapshotTypeFull {
							return fullRevAfter > fullRevBefore
						}
						return deltaRevAfter > deltaRevBefore
					}, 30*time.Second, 2*time.Second).Should(BeTrue(), "snapshot lease revision did not advance after task completion")
					logger.Info("snapshot lease revision advanced as expected")
				}

				logger.Info("waiting for task to be deleted after TTL")
				testEnv.CheckEtcdOpsTaskDeleted(g, task, defaultTaskDeletionTimeout)
				logger.Info("task deleted after TTL expiry")

				testSucceeded = true
			})
		}
	}
}
