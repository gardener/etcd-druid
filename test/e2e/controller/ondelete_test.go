// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"fmt"
	"testing"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
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

// testOnDelete3ReplicaZeroDowntime tests that the 3 replica cluster update rolls without any downtime.
func testOnDelete3ReplicaZeroDowntime(t *testing.T, tcName string, log logr.Logger) {
	g := NewWithT(t)
	var testSucceeded bool

	testNamespace := testutils.GenerateTestNamespaceNameWithTestCaseName(t, testNamespacePrefix, tcName, 4)
	logger := log.WithName(tcName).WithValues("etcdName", defaultEtcdName, "namespace", testNamespace)
	defer func() {
		cleanupTestArtifacts(retainTestArtifacts, testSucceeded, testEnv, logger, g, testNamespace)
	}()
	initializeTestCase(g, testEnv, logger, testNamespace, defaultEtcdName, druidv1alpha1.StorageProvider("none"))

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
	testSucceeded = true
}

// testOnDeleteScaleUpPlusTemplateChange creates a 1-replica Etcd, then scales it out to 3 with a template change in the same update.
func testOnDeleteScaleUpPlusTemplateChange(t *testing.T, tcName string, log logr.Logger) {
	g := NewWithT(t)
	var testSucceeded bool

	testNamespace := testutils.GenerateTestNamespaceNameWithTestCaseName(t, testNamespacePrefix, tcName, 4)
	logger := log.WithName(tcName).WithValues("etcdName", defaultEtcdName, "namespace", testNamespace)
	defer func() {
		cleanupTestArtifacts(retainTestArtifacts, testSucceeded, testEnv, logger, g, testNamespace)
	}()
	initializeTestCase(g, testEnv, logger, testNamespace, defaultEtcdName, druidv1alpha1.StorageProvider("none"))

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
		cleanupTestArtifacts(retainTestArtifacts, testSucceeded, testEnv, logger, g, testNamespace)
	}()
	initializeTestCase(g, testEnv, logger, testNamespace, defaultEtcdName, druidv1alpha1.StorageProvider("none"))

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

	// restart the operator while the rollout is (probably) still in flight.
	logger.Info("restarting the etcd-druid operator")
	restartDruidOperator(g, testEnv, logger, timeoutDruidDeploymentRestart)

	waitForOnDeleteRolloutComplete(g, testEnv, logger, fetched, timeoutOnDeleteRolloutComplete)
	testEnv.CheckEtcdReady(g, etcd, timeoutEtcdUpdation)
	testSucceeded = true
}
