// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"fmt"
	"testing"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	e2eutils "github.com/gardener/etcd-druid/test/e2e/utils"
	testutils "github.com/gardener/etcd-druid/test/utils"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/testr"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/utils/ptr"

	. "github.com/onsi/gomega"
)

// TestOnDeleteRollout runs the OnDelete-controlled rollout end-to-end against a real Kind cluster.
func TestOnDeleteRollout(t *testing.T) {
	skipIfOnDeleteGateDisabled(t, testEnv)
	t.Parallel()
	log := testr.NewWithOptions(t, testr.Options{LogTimestamp: true})

	testCases := []struct {
		name string
		fn   func(t *testing.T, tcName string, log logr.Logger)
	}{
		{"3-replica-zero-downtime", testOnDelete3ReplicaZeroDowntime},
		{"scale-up-with-template-change", testOnDeleteScaleUpPlusTemplateChange},
		{"resume-after-operator-restart", testOnDeleteResumesAfterOperatorRestart},
	}

	for _, tc := range testCases {
		tcName := fmt.Sprintf("ondelete-%s", tc.name)
		t.Run(tcName, func(t *testing.T) {
			t.Parallel()
			tc.fn(t, tcName, log)
		})
	}
}

func TestOnDeleteGateOffUsesRollingUpdate(t *testing.T) {
	skipIfOnDeleteGateEnabled(t, testEnv)
	t.Parallel()
	log := testr.NewWithOptions(t, testr.Options{LogTimestamp: true})
	testOnDeleteGateOffUsesRollingUpdate(t, "ondelete-gate-off-uses-rolling-update", log)
}

// testOnDelete3ReplicaZeroDowntime tests that the 3 replica cluster update rolls without any downtime.
func testOnDelete3ReplicaZeroDowntime(t *testing.T, tcName string, log logr.Logger) {
	g := NewWithT(t)
	var testSucceeded bool

	testNamespace := testutils.GenerateTestNamespaceNameWithTestCaseName(t, testNamespacePrefix, tcName, 4)
	logger := log.WithName(tcName).WithValues("etcdName", defaultEtcdName, "namespace", testNamespace)
	defer func() {
		e2eutils.CleanupTestArtifacts(retainTestArtifacts, testSucceeded, testEnv, logger, g, testNamespace)
	}()
	e2eutils.InitializeTestCase(g, testEnv, logger, testNamespace, defaultEtcdName, druidv1alpha1.StorageProvider("none"))

	etcd := testutils.EtcdBuilderWithoutDefaults(defaultEtcdName, testNamespace).
		WithReplicas(3).
		WithEtcdClientPort(ptr.To[int32](2379)).
		WithClientTLS().
		WithPeerTLS().
		WithGRPCGatewayEnabled().
		WithBackupRestoreTLS().
		Build()

	logger.Info("creating Etcd")
	testEnv.CreateAndCheckEtcd(g, etcd, timeoutEtcdCreation)
	logger.Info("successfully created Etcd resource")

	// Confirm that the STS came up with the expected `OnDelete` strategy
	waitForStsUpdateStrategy(g, testEnv, logger, etcd, appsv1.OnDeleteStatefulSetStrategyType, timeoutOnDeleteRolloutStart)

	logger.Info("starting zero-downtime validator job")
	testEnv.DeployZeroDowntimeValidatorJob(g, testNamespace, druidv1alpha1.GetClientServiceName(etcd.ObjectMeta), *etcd.Spec.Etcd.ClientPort, etcd.Spec.Etcd.ClientUrlTLS, timeoutDeployJob)

	fetched, err := testEnv.GetEtcd(etcd.Name, etcd.Namespace)
	g.Expect(err).ToNot(HaveOccurred())
	logger.Info("updating Etcd spec")
	fetched.Spec.Etcd.Metrics = ptr.To(druidv1alpha1.Extensive)
	testEnv.UpdateAndCheckEtcd(g, fetched, timeoutEtcdUpdation)

	waitForOnDeleteRolloutComplete(g, testEnv, logger, fetched, timeoutOnDeleteRolloutComplete)

	logger.Info("checking for downtime")
	testEnv.CheckForDowntime(g, testNamespace, false)
	logger.Info("no downtime observed")

	// Verify post-rollout status conditions.
	assertRolloutConditions(g, testEnv, logger, fetched, timeoutEtcdUpdation)
	testSucceeded = true
}

// testOnDeleteScaleUpPlusTemplateChange creates a 1-replica Etcd, then scales it out to 3 with a template change in the same update.
func testOnDeleteScaleUpPlusTemplateChange(t *testing.T, tcName string, log logr.Logger) {
	g := NewWithT(t)
	var testSucceeded bool

	testNamespace := testutils.GenerateTestNamespaceNameWithTestCaseName(t, testNamespacePrefix, tcName, 4)
	logger := log.WithName(tcName).WithValues("etcdName", defaultEtcdName, "namespace", testNamespace)
	defer func() {
		e2eutils.CleanupTestArtifacts(retainTestArtifacts, testSucceeded, testEnv, logger, g, testNamespace)
	}()
	e2eutils.InitializeTestCase(g, testEnv, logger, testNamespace, defaultEtcdName, druidv1alpha1.StorageProvider("none"))

	etcd := testutils.EtcdBuilderWithoutDefaults(defaultEtcdName, testNamespace).
		WithReplicas(1).
		WithEtcdClientPort(ptr.To[int32](2379)).
		WithClientTLS().
		WithPeerTLS().
		WithGRPCGatewayEnabled().
		WithBackupRestoreTLS().
		Build()

	logger.Info("creating Etcd (1 replica)")
	testEnv.CreateAndCheckEtcd(g, etcd, timeoutEtcdCreation)
	waitForStsUpdateStrategy(g, testEnv, logger, etcd, appsv1.OnDeleteStatefulSetStrategyType, timeoutOnDeleteRolloutStart)

	fetched, err := testEnv.GetEtcd(etcd.Name, etcd.Namespace)
	g.Expect(err).ToNot(HaveOccurred())
	fetched.Spec.Replicas = 3
	fetched.Spec.Etcd.Metrics = ptr.To(druidv1alpha1.Extensive)
	logger.Info("scaling to 3 replicas with a template change in the same update")
	testEnv.UpdateAndCheckEtcd(g, fetched, timeoutEtcdUpdation)

	waitForOnDeleteRolloutComplete(g, testEnv, logger, fetched, timeoutOnDeleteRolloutComplete)
	assertRolloutConditions(g, testEnv, logger, fetched, timeoutEtcdUpdation)
	testSucceeded = true
}

// testOnDeleteResumesAfterOperatorRestart triggers a rollout, restarts the etcd-druid operator once the rollout has started,
// and asserts that the rollout still happens.
func testOnDeleteResumesAfterOperatorRestart(t *testing.T, tcName string, log logr.Logger) {
	g := NewWithT(t)
	var testSucceeded bool

	testNamespace := testutils.GenerateTestNamespaceNameWithTestCaseName(t, testNamespacePrefix, tcName, 4)
	logger := log.WithName(tcName).WithValues("etcdName", defaultEtcdName, "namespace", testNamespace)
	defer func() {
		e2eutils.CleanupTestArtifacts(retainTestArtifacts, testSucceeded, testEnv, logger, g, testNamespace)
	}()
	e2eutils.InitializeTestCase(g, testEnv, logger, testNamespace, defaultEtcdName, druidv1alpha1.StorageProvider("none"))

	etcd := testutils.EtcdBuilderWithoutDefaults(defaultEtcdName, testNamespace).
		WithReplicas(3).
		WithEtcdClientPort(ptr.To[int32](2379)).
		WithClientTLS().
		WithPeerTLS().
		WithGRPCGatewayEnabled().
		WithBackupRestoreTLS().
		Build()

	logger.Info("creating Etcd")
	testEnv.CreateAndCheckEtcd(g, etcd, timeoutEtcdCreation)
	waitForStsUpdateStrategy(g, testEnv, logger, etcd, appsv1.OnDeleteStatefulSetStrategyType, timeoutOnDeleteRolloutStart)

	fetched, err := testEnv.GetEtcd(etcd.Name, etcd.Namespace)
	g.Expect(err).ToNot(HaveOccurred())
	fetched.Spec.Etcd.Metrics = ptr.To(druidv1alpha1.Extensive)
	logger.Info("updating Etcd spec")
	fetched.SetAnnotations(map[string]string{druidv1alpha1.DruidOperationAnnotation: druidv1alpha1.DruidOperationReconcile})
	g.Expect(testEnv.Client().Update(testEnv.Context(), fetched)).To(Succeed())

	// wait until the rollout has begun by checking that at least one pod is deleted
	// before restarting the operator, so the test can verify resuming of an in-progress rollout which is a better test.
	waitForRolloutInProgress(g, testEnv, logger, fetched, timeoutOnDeleteRolloutStart)
	logger.Info("restarting the etcd-druid operator mid-rollout")
	restartDruidOperator(g, testEnv, logger, timeoutDruidDeploymentRestart)

	waitForOnDeleteRolloutComplete(g, testEnv, logger, fetched, timeoutOnDeleteRolloutComplete)
	testEnv.CheckEtcdReady(g, etcd, timeoutEtcdUpdation)
	assertRolloutConditions(g, testEnv, logger, fetched, timeoutEtcdUpdation)
	testSucceeded = true
}

// testOnDeleteGateOffUsesRollingUpdate verifies that when the QuorumAwareUpdates
// feature gate is disabled, the Etcd reconciler keeps the StatefulSet
// on the RollingUpdate strategy and an ordinary update completes normally.
// Skipped when the gate is enabled since the gate-on tests already cover that path.
func testOnDeleteGateOffUsesRollingUpdate(t *testing.T, tcName string, log logr.Logger) {
	skipIfOnDeleteGateEnabled(t, testEnv)

	g := NewWithT(t)
	var testSucceeded bool

	testNamespace := testutils.GenerateTestNamespaceNameWithTestCaseName(t, testNamespacePrefix, tcName, 4)
	logger := log.WithName(tcName).WithValues("etcdName", defaultEtcdName, "namespace", testNamespace)
	defer func() {
		e2eutils.CleanupTestArtifacts(retainTestArtifacts, testSucceeded, testEnv, logger, g, testNamespace)
	}()
	e2eutils.InitializeTestCase(g, testEnv, logger, testNamespace, defaultEtcdName, druidv1alpha1.StorageProvider("none"))

	etcd := testutils.EtcdBuilderWithoutDefaults(defaultEtcdName, testNamespace).
		WithReplicas(3).
		WithEtcdClientPort(ptr.To[int32](2379)).
		WithClientTLS().
		WithPeerTLS().
		WithGRPCGatewayEnabled().
		WithBackupRestoreTLS().
		Build()

	logger.Info("creating Etcd (gate disabled: expect RollingUpdate strategy)")
	testEnv.CreateAndCheckEtcd(g, etcd, timeoutEtcdCreation)

	// With gate off, the sts should be using RollingUpdate
	waitForStsUpdateStrategy(g, testEnv, logger, etcd, appsv1.RollingUpdateStatefulSetStrategyType, timeoutOnDeleteRolloutStart)
	logger.Info("confirmed: StatefulSet uses RollingUpdate when gate is disabled")

	// Trigger an update and verify it completes normally under RollingUpdate.
	fetched, err := testEnv.GetEtcd(etcd.Name, etcd.Namespace)
	g.Expect(err).ToNot(HaveOccurred())
	fetched.Spec.Etcd.Metrics = ptr.To(druidv1alpha1.Extensive)
	testEnv.UpdateAndCheckEtcd(g, fetched, timeoutEtcdUpdation)
	logger.Info("update completed under RollingUpdate strategy")

	testSucceeded = true
}
