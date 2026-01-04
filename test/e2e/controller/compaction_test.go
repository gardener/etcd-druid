// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"fmt"
	"testing"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	testutils "github.com/gardener/etcd-druid/test/utils"

	"github.com/go-logr/logr/testr"
	"k8s.io/utils/ptr"

	. "github.com/onsi/gomega"
)

const (
	configuredEventsThreshold = 15 // as configured in the compaction controller for e2e tests (via ../../../skaffold.yaml)

	timeoutCompaction = 1 * time.Minute
	timeoutSnapshot   = 30 * time.Second
)

// TestSnapshotCompaction tests snapshot compaction.
func TestSnapshotCompaction(t *testing.T) {
	t.Parallel()
	log := testr.NewWithOptions(t, testr.Options{LogTimestamp: true})

	defaultRevisionsForFullSnapshot := int64(10)

	testCases := []struct {
		name                          string
		revisionsForFullSnapshot      int64
		revisionsForDeltaSnapshot     int64
		expectCompaction              bool
		expectedFullSnapshotRevision  int64
		expectedDeltaSnapshotRevision int64
	}{
		{
			name:                          "no-snaps",
			expectCompaction:              false,
			expectedFullSnapshotRevision:  1, // default revision in etcd cluster
			expectedDeltaSnapshotRevision: 0,
		},
		{
			name:                          "full-no-delta",
			revisionsForFullSnapshot:      defaultRevisionsForFullSnapshot,
			expectCompaction:              false,
			expectedFullSnapshotRevision:  1 + defaultRevisionsForFullSnapshot,
			expectedDeltaSnapshotRevision: 0,
		},
		{
			name:                          "full-delta-no-comp",
			revisionsForFullSnapshot:      defaultRevisionsForFullSnapshot,
			revisionsForDeltaSnapshot:     configuredEventsThreshold - 1,
			expectCompaction:              false,
			expectedFullSnapshotRevision:  1 + defaultRevisionsForFullSnapshot,
			expectedDeltaSnapshotRevision: defaultRevisionsForFullSnapshot + configuredEventsThreshold, // 1 + full + (threshold - 1)
		},
		{
			name:                          "full-delta-comp",
			revisionsForFullSnapshot:      defaultRevisionsForFullSnapshot,
			revisionsForDeltaSnapshot:     configuredEventsThreshold,
			expectCompaction:              true,
			expectedFullSnapshotRevision:  1 + defaultRevisionsForFullSnapshot + configuredEventsThreshold, // 1 + full + threshold
			expectedDeltaSnapshotRevision: 1 + defaultRevisionsForFullSnapshot + configuredEventsThreshold, // same as full snapshot revision
		},
	}

	g := NewWithT(t)

	provider := druidv1alpha1.StorageProvider("local")
	for _, tc := range testCases {
		tcName := fmt.Sprintf("compaction-%s", tc.name)
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			testNamespace := testutils.GenerateTestNamespaceNameWithTestCaseName(t, testNamespacePrefix, tcName, 4)
			logger := log.WithName(tcName).WithValues("etcdName", defaultEtcdName, "namespace", testNamespace)
			defer cleanupTestArtifacts(!retainTestArtifacts, testEnv, logger, g, testNamespace)

			initializeTestCase(g, testEnv, logger, testNamespace, defaultEtcdName)

			logger.Info("running tests")
			etcd := testutils.EtcdBuilderWithoutDefaults(defaultEtcdName, testNamespace).
				WithReplicas(1).
				WithEtcdClientPort(ptr.To[int32](2379)).
				WithClientTLS().
				WithPeerTLS().
				WithGRPCGatewayEnabled().
				WithDefaultBackup().
				WithStorageProvider(provider, fmt.Sprintf("%s/%s", testNamespace, defaultEtcdName)).
				WithDeltaSnapshotPeriod(300*time.Hour).                                                                   // TODO: set to 0 (disable scheduled delta snapshots) after https://github.com/gardener/etcd-backup-restore/issues/965 is resolved
				WithGarbageCollection(600*time.Hour, druidv1alpha1.GarbageCollectionPolicyLimitBased, ptr.To[int32](10)). // TODO: remove this line once we set delta snapshot period to 0 after https://github.com/gardener/etcd-backup-restore/issues/965 is resolved
				WithBackupRestoreTLS().
				Build()

			logger.Info("creating Etcd resource")
			testEnv.CreateAndCheckEtcd(g, etcd, timeoutEtcdCreation)
			logger.Info("successfully created Etcd resource")

			if tc.revisionsForFullSnapshot > 0 {
				logger.Info("creating EtcdLoader job to put data for full snapshot")
				testEnv.DeployEtcdLoaderJob(g, testNamespace, druidv1alpha1.GetClientServiceName(etcd.ObjectMeta), *etcd.Spec.Etcd.ClientPort, etcd.Spec.Etcd.ClientUrlTLS, tc.revisionsForFullSnapshot, timeoutCompaction)
				logger.Info("successfully put data for full snapshot")

				logger.Info("triggering full snapshot")
				testEnv.TakeFullSnapshot(g, etcd, timeoutSnapshot)
				logger.Info("successfully triggered full snapshot")
			}

			if tc.revisionsForDeltaSnapshot > 0 {
				g.Expect(tc.revisionsForDeltaSnapshot).To(BeNumerically(">", tc.revisionsForFullSnapshot))

				logger.Info("creating EtcdLoader job to put data for delta snapshot")
				testEnv.DeployEtcdLoaderJob(g, testNamespace, druidv1alpha1.GetClientServiceName(etcd.ObjectMeta), *etcd.Spec.Etcd.ClientPort, etcd.Spec.Etcd.ClientUrlTLS, tc.revisionsForDeltaSnapshot, timeoutCompaction)
				logger.Info("successfully put data for delta snapshot")

				logger.Info("triggering delta snapshot")
				testEnv.TakeDeltaSnapshot(g, etcd, timeoutSnapshot)
				logger.Info("successfully triggered delta snapshot")
			}

			if tc.expectCompaction {
				g.Expect(tc.expectedFullSnapshotRevision).ToNot(BeZero())
				g.Expect(tc.expectedDeltaSnapshotRevision).ToNot(BeZero())
				logger.Info("waiting for compaction to be triggered")
				testEnv.EnsureCompaction(g, etcd.ObjectMeta, tc.expectedFullSnapshotRevision, tc.expectedDeltaSnapshotRevision, timeoutCompaction)
				logger.Info("compaction successfully triggered")
			} else {
				logger.Info(fmt.Sprintf("waiting for duration %v to ensure that compaction is not triggered", timeoutCompaction.String()))
				testEnv.EnsureNoCompaction(g, etcd.ObjectMeta, tc.expectedFullSnapshotRevision, tc.expectedDeltaSnapshotRevision, timeoutCompaction)
				logger.Info("ensured that compaction was not triggered")
			}
		})
	}
}
