// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
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
