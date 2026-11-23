// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package etcdopstask

import (
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/test/e2e/testenv"
	e2eutils "github.com/gardener/etcd-druid/test/e2e/utils"
	testutils "github.com/gardener/etcd-druid/test/utils"
)

const (
	testNamespacePrefix = "eot-e2e"

	timeoutTest                  = 1 * time.Hour
	timeoutEtcdCreation          = 5 * time.Minute
	timeoutEtcdOpsTaskCompletion = 3 * time.Minute
	timeoutEtcdOpsTaskTTLExpiry  = 30 * time.Second
	timeoutDeployJob             = 2 * time.Minute
	timeoutLargeDeployJob        = 15 * time.Minute
	timeoutFullSnapshot          = 30 * time.Second

	// timeoutEtcdCreation3Member is the upper bound for CreateAndCheckEtcd on a 3-replica
	// cluster in the etcdopstask tests. 3-replica clusters start more slowly under KIND load
	// than the single-replica clusters, so a longer timeout is needed.
	timeoutEtcdCreation3Member = 10 * time.Minute
	// to transition to False/QuorumLost after member pods have been deleted.
	timeoutQuorumLostCondition = 3 * time.Minute
	// timeoutRecoveryTaskCompletion is the upper bound for the RecoverFromQuorumLoss task to
	// reach a terminal state (Succeeded or Failed). Recovery involves scale-down, PVC deletion,
	// scale-up, and multi-member readiness, so it takes longer than a snapshot task.
	timeoutRecoveryTaskCompletion = 10 * time.Minute
	// timeoutEtcdRecoveryReady is the upper bound for the etcd cluster to become fully ready
	// (all members) after the recovery task has completed.
	timeoutEtcdRecoveryReady = 5 * time.Minute

	defaultOpsTaskTTLSeconds = int32(30)
)

var (
	testEnv             *testenv.TestEnvironment
	retainTestArtifacts e2eutils.RetainTestArtifactsMode
	providers           []druidv1alpha1.StorageProvider

	// defaultTaskDeletionTimeout is the upper bound for waiting on TTL-driven deletion of the EtcdOpsTask.
	defaultTaskDeletionTimeout = time.Duration(defaultOpsTaskTTLSeconds)*time.Second + timeoutEtcdOpsTaskTTLExpiry
)

// newOnDemandSnapshotTask returns an EtcdOpsTask for an on-demand snapshot of the given type, with the package's default TTL and timeout values.
func newOnDemandSnapshotTask(name, namespace, etcdName string, snapshotType druidv1alpha1.OnDemandSnapshotType) *druidv1alpha1.EtcdOpsTask {
	return testutils.EtcdOpsTaskBuilderWithoutDefaults(name, namespace).
		WithEtcdName(etcdName).
		WithTTLSecondsAfterFinished(defaultOpsTaskTTLSeconds).
		WithOnDemandSnapshotConfig(&druidv1alpha1.OnDemandSnapshotConfig{
			Type: snapshotType,
		}).
		Build()
}

// newRecoverFromQuorumLossTask returns an EtcdOpsTask for quorum-loss recovery with the
// package's default TTL and the given config.
func newRecoverFromQuorumLossTask(name, namespace, etcdName string, cfg *druidv1alpha1.RecoverFromQuorumLossConfig) *druidv1alpha1.EtcdOpsTask {
	return testutils.EtcdOpsTaskBuilderWithoutDefaults(name, namespace).
		WithEtcdName(etcdName).
		WithTTLSecondsAfterFinished(defaultOpsTaskTTLSeconds).
		WithRecoverFromQuorumLossConfig(cfg).
		Build()
}
