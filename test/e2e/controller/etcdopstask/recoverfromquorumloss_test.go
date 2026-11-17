// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package etcdopstask

import (
	"fmt"
	"testing"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	healthcondition "github.com/gardener/etcd-druid/internal/health/condition"
	e2eutils "github.com/gardener/etcd-druid/test/e2e/utils"
	testutils "github.com/gardener/etcd-druid/test/utils"

	"github.com/go-logr/logr/testr"
	"k8s.io/utils/ptr"

	. "github.com/onsi/gomega"
)

// confirmRecoveryAnnotation is the key that must be set to "true" on an EtcdOpsTask before
// the RecoverFromQuorumLoss handler will proceed past WaitConfirmation.
const confirmRecoveryAnnotation = "druid.gardener.cloud/confirm-recovery"

// TestRecoverFromQuorumLossLifecycle validates the lifecycle of RecoverFromQuorumLoss tasks.
func TestRecoverFromQuorumLossLifecycle(t *testing.T) {
	t.Parallel()
	log := testr.NewWithOptions(t, testr.Options{LogTimestamp: true})

	testCases := []struct {
		name               string
		purpose            string
		replicas           int32
		backupEnabled      bool
		simulateQuorumLoss bool
		allowDataLoss      *bool
		expectedState      druidv1alpha1.TaskState
		// requiresProvider skips this test case when running against the "none" provider.
		requiresProvider bool
	}{
		{
			name:             "rejected-single-member",
			purpose:          "test Rejected state when etcd has only one replica",
			replicas:         1,
			backupEnabled:    true,
			requiresProvider: true,
			expectedState:    druidv1alpha1.TaskStateRejected,
		},
		{
			name:          "rejected-no-backup-no-allow-data-loss",
			purpose:       "test Rejected state when no backup is configured and AllowDataLoss is not set",
			replicas:      3,
			backupEnabled: false,
			// simulateQuorumLoss is false — we cannot reach QuorumLost without backup anyway because
			// the handler rejects before checking the Ready condition, but we still need a 3-member
			// etcd so the single-member guard does not fire first. No real quorum loss is needed here
			// since the backup-missing check fires before the quorum-check.
			expectedState: druidv1alpha1.TaskStateRejected,
		},
		{
			name:             "rejected-etcd-healthy",
			purpose:          "test Rejected state when the etcd cluster is healthy (Ready=True)",
			replicas:         3,
			backupEnabled:    true,
			requiresProvider: true,
			// no simulateQuorumLoss — etcd is running normally, task should be rejected
			expectedState: druidv1alpha1.TaskStateRejected,
		},
		{
			name:               "succeeds-after-quorum-loss",
			purpose:            "test Succeeded state after quorum loss is simulated and the task is confirmed",
			replicas:           3,
			backupEnabled:      true,
			simulateQuorumLoss: true,
			requiresProvider:   true,
			// AllowDataLoss is set because after 2 of 3 pods are killed the surviving pod's
			// backup sidecar may lose BackupReady=True (it cannot write to the store with a
			// degraded etcd cluster), which would cause Admit to reject the task. The full
			// snapshot was already taken before disruption, so data loss risk is low.
			allowDataLoss: ptr.To(true),
			expectedState: druidv1alpha1.TaskStateSucceeded,
		},
		{
			name:               "succeeds-allow-data-loss-no-backup",
			purpose:            "test Succeeded state after quorum loss when AllowDataLoss=true and no backup is configured",
			replicas:           3,
			backupEnabled:      false,
			simulateQuorumLoss: true,
			allowDataLoss:      ptr.To(true),
			expectedState:      druidv1alpha1.TaskStateSucceeded,
		},
	}

	for _, provider := range providers {
		for _, tc := range testCases {
			if tc.requiresProvider && provider == "none" {
				continue
			}
			if !tc.backupEnabled && provider != "none" {
				// No-backup test cases only make sense for the "none" provider to avoid
				// redundant runs across backup providers.
				continue
			}
			tcName := fmt.Sprintf("qrloss-%s-%s", tc.name, e2eutils.GetProviderSuffix(provider))
			t.Run(tcName, func(t *testing.T) {
				t.Parallel()
				g := NewWithT(t)
				var testSucceeded bool

				testNamespace := testutils.GenerateTestNamespaceNameWithTestCaseName(t, testNamespacePrefix, tcName, 4)
				logger := log.WithName(tcName).WithValues("etcdName", e2eutils.DefaultEtcdName, "namespace", testNamespace)
				defer func() {
					e2eutils.CleanupTestArtifacts(retainTestArtifacts, testSucceeded, testEnv, logger, g, testNamespace)
				}()

				logger.Info("running test", "purpose", tc.purpose)

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
				if tc.simulateQuorumLoss {
					// Component protection must be off so the test can delete pods/PVCs directly.
					etcdBuilder = etcdBuilder.WithComponentProtectionDisabled()
				}
				etcd := etcdBuilder.Build()

				logger.Info("creating Etcd resource")
				if tc.replicas > 0 && (tc.backupEnabled || tc.simulateQuorumLoss) {
					testEnv.CreateAndCheckEtcd(g, etcd, timeoutEtcdCreation3Member)
				} else {
					g.Expect(testEnv.Client().Create(testEnv.Context(), etcd)).To(Succeed())
				}
				logger.Info("successfully created Etcd resource")

				if tc.simulateQuorumLoss {
					if tc.backupEnabled {
						logger.Info("taking full snapshot to ensure BackupReady=True before disruption")
						testEnv.TakeFullSnapshot(g, etcd, timeoutFullSnapshot)
					}

					logger.Info("simulating quorum loss by deleting 2 of 3 pods and their PVCs")
					testEnv.DisruptEtcd(g, etcd, 2, 2, timeoutEtcdCreation)

					logger.Info("waiting for Ready=False/QuorumLost condition to appear on Etcd")
					testEnv.CheckEtcdCondition(
						g, etcd,
						druidv1alpha1.ConditionTypeReady,
						druidv1alpha1.ConditionFalse,
						healthcondition.ReadyConditionReasonQuorumLost,
						timeoutQuorumLostCondition,
					)
					logger.Info("QuorumLost condition confirmed; re-checking stability before creating task")
					// Re-check QuorumLost stability: after disrupting pods, the surviving pod can
					// crash-loop briefly, clearing Status.Members and causing the health reconciler to
					// emit Ready=Unknown/NoMembersInStatus instead of Ready=False/QuorumLost.
					// The Admit handler only accepts QuorumLost, so we wait a second time to
					// confirm the condition is still stable before creating the task.
					testEnv.CheckEtcdCondition(
						g, etcd,
						druidv1alpha1.ConditionTypeReady,
						druidv1alpha1.ConditionFalse,
						healthcondition.ReadyConditionReasonQuorumLost,
						timeoutQuorumLostCondition,
					)
					logger.Info("QuorumLost condition stable")
				}

				taskName := fmt.Sprintf("%s-%s-task", e2eutils.DefaultEtcdName, tc.name)
				cfg := &druidv1alpha1.RecoverFromQuorumLossConfig{}
				if tc.allowDataLoss != nil {
					cfg.AllowDataLoss = tc.allowDataLoss
				}
				// Use a long TTL for success cases: the 13-step recovery flow with requeueing
				// takes minutes, and the short default TTL can delete the task before the
				// controller finishes processing it under cluster load.
				ttl := defaultOpsTaskTTLSeconds
				if tc.expectedState == druidv1alpha1.TaskStateSucceeded {
					ttl = int32(3600)
				}
				task := testutils.EtcdOpsTaskBuilderWithoutDefaults(taskName, testNamespace).
					WithEtcdName(e2eutils.DefaultEtcdName).
					WithTTLSecondsAfterFinished(ttl).
					WithRecoverFromQuorumLossConfig(cfg).
					Build()

				logger.Info("creating EtcdOpsTask", "taskName", taskName)
				testEnv.CreateEtcdOpsTask(g, task)

				if tc.expectedState == druidv1alpha1.TaskStateSucceeded {
					// Wait for the task to be admitted and reach InProgress before confirming,
					// so we can verify the WaitConfirmation gate is actually enforced.
					logger.Info("waiting for task to reach InProgress before adding confirmation annotation")
					testEnv.CheckEtcdOpsTaskState(g, task, druidv1alpha1.TaskStateInProgress, timeoutEtcdOpsTaskCompletion)
					logger.Info("task is InProgress; adding confirmation annotation")
					testEnv.AddAnnotationToEtcdOpsTask(g, task, confirmRecoveryAnnotation, "true")
					logger.Info("confirmation annotation added")
				}

				logger.Info("waiting for EtcdOpsTask to reach expected state", "expectedState", tc.expectedState)
				testEnv.CheckEtcdOpsTaskState(g, task, tc.expectedState, timeoutRecoveryTaskCompletion)
				logger.Info("EtcdOpsTask reached expected state")

				if tc.expectedState == druidv1alpha1.TaskStateSucceeded {
					logger.Info("verifying etcd cluster is fully ready after recovery")
					testEnv.CheckEtcdReady(g, etcd, timeoutEtcdRecoveryReady)
					logger.Info("etcd cluster is ready")
				}

				// Only wait for TTL-driven deletion on rejected cases (short TTL=30s).
				// Success cases use TTL=3600s to prevent the task from being garbage-collected
				// during the long recovery flow; waiting 3600+30s here would make the test
				// impractically long, so we skip the deletion check for those.
				if tc.expectedState != druidv1alpha1.TaskStateSucceeded {
					logger.Info("waiting for task to be deleted after TTL")
					testEnv.CheckEtcdOpsTaskDeleted(g, task, defaultTaskDeletionTimeout)
					logger.Info("task deleted after TTL expiry")
				}

				testSucceeded = true
			})
		}
	}
}
