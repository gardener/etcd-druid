// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
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
		{"scale-up-with-template-change", testOnDeleteScaleUpPlusTemplateChange},
		{"single-replica-rollout", testOnDeleteSingleReplicaRollout},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.fn(t, tc.name, log)
		})
	}
}

// TestOnDeleteGateOffUsesRollingUpdate verifies that the StatefulSet keeps the RollingUpdate strategy when the gate is off.
func TestOnDeleteGateOffUsesRollingUpdate(t *testing.T) {
	skipIfOnDeleteGateEnabled(t, testEnv)
	t.Parallel()
	log := testr.NewWithOptions(t, testr.Options{LogTimestamp: true})
	testOnDeleteGateOffUsesRollingUpdate(t, "gate-off-uses-rolling-update", log)
}

// TestOnDeleteResumesAfterOperatorRestart holds a rollout with a Pending replacement, stops etcd-druid, and verifies
// that no further pod is updated until etcd-druid is back, after which the rollout completes.
// It cordons all nodes and stops etcd-druid, so it must not run in parallel with other tests.
func TestOnDeleteResumesAfterOperatorRestart(t *testing.T) {
	skipIfOnDeleteGateDisabled(t, testEnv)
	log := testr.NewWithOptions(t, testr.Options{LogTimestamp: true})
	tcName := "resume-after-restart"
	g := NewWithT(t)
	var testSucceeded bool

	testNamespace := testutils.GenerateTestNamespaceNameWithTestCaseName(t, onDeleteTestNamespacePrefix, tcName, 4)
	logger := log.WithName(tcName).WithValues("etcdName", defaultEtcdName, "namespace", testNamespace)
	defer func() {
		e2eutils.CleanupTestArtifacts(retainTestArtifacts, testSucceeded, testEnv, logger, g, testNamespace)
	}()
	// Deferred after the cleanup so that it runs first: deleting druid-managed resources needs etcd-druid's webhook.
	changes := newClusterChanges(testEnv, logger)
	defer changes.undo(g)
	e2eutils.InitializeTestCase(g, testEnv, logger, testNamespace, defaultEtcdName, druidv1alpha1.StorageProvider("none"))

	etcd := newOnDeleteTestEtcd(testNamespace, 3)
	logger.Info("creating Etcd")
	testEnv.CreateAndCheckEtcd(g, etcd, timeoutEtcdCreation)
	waitForStsUpdateStrategy(g, testEnv, logger, etcd, appsv1.OnDeleteStatefulSetStrategyType, timeoutOnDeleteRolloutStart)
	snapshot := snapshotStsPods(g, testEnv, logger, etcd)

	changes.cordonNodes(g)
	triggerPodTemplateUpdate(g, testEnv, logger, etcd)
	replacement := waitForPendingReplacement(g, testEnv, logger, etcd, snapshot, timeoutReplacementPending)

	changes.scaleDruidToZero(g)
	waitForDruidPodsGone(g, testEnv, logger, timeoutDruidDeploymentRestart)
	changes.uncordonNodes(g)

	waitForEtcdContainerReady(g, testEnv, logger, replacement, timeoutReplacementReady)
	remaining := snapshot.podNamesExcept(replacement.Name)
	expectPodsUntouched(g, testEnv, logger, etcd, snapshot, remaining, podsUntouchedWindow)
	expectDruidStopped(g, testEnv)

	changes.restoreDruid(g)
	waitForAnyPodReplaced(g, testEnv, logger, etcd, snapshot, remaining, timeoutOnDeleteRolloutComplete)
	waitForOnDeleteRolloutComplete(g, testEnv, logger, etcd, timeoutOnDeleteRolloutComplete)
	testEnv.CheckEtcdReady(g, etcd, timeoutEtcdUpdation)
	assertRolloutConditions(g, testEnv, logger, etcd, timeoutEtcdUpdation)
	expectAllPodsReplaced(g, testEnv, etcd, snapshot)
	testSucceeded = true
}

// TestOnDeleteUpdatesOnePodAtATime holds the first replacement Pending and verifies that no other pod is touched until
// it is Ready, and that the rollout causes no downtime.
// It cordons all nodes, so it must not run in parallel with other tests.
func TestOnDeleteUpdatesOnePodAtATime(t *testing.T) {
	skipIfOnDeleteGateDisabled(t, testEnv)
	log := testr.NewWithOptions(t, testr.Options{LogTimestamp: true})
	tcName := "one-pod-at-a-time"
	g := NewWithT(t)
	var testSucceeded bool

	testNamespace := testutils.GenerateTestNamespaceNameWithTestCaseName(t, onDeleteTestNamespacePrefix, tcName, 4)
	logger := log.WithName(tcName).WithValues("etcdName", defaultEtcdName, "namespace", testNamespace)
	defer func() {
		e2eutils.CleanupTestArtifacts(retainTestArtifacts, testSucceeded, testEnv, logger, g, testNamespace)
	}()
	// Uncordons even if the test fails, so that later tests can schedule pods.
	changes := newClusterChanges(testEnv, logger)
	defer changes.undo(g)
	e2eutils.InitializeTestCase(g, testEnv, logger, testNamespace, defaultEtcdName, druidv1alpha1.StorageProvider("none"))

	etcd := newOnDeleteTestEtcd(testNamespace, 3)
	logger.Info("creating Etcd")
	testEnv.CreateAndCheckEtcd(g, etcd, timeoutEtcdCreation)
	waitForStsUpdateStrategy(g, testEnv, logger, etcd, appsv1.OnDeleteStatefulSetStrategyType, timeoutOnDeleteRolloutStart)
	waitForEtcdCondition(g, testEnv, logger, etcd, druidv1alpha1.ConditionTypeAllMembersUpdated, druidv1alpha1.ConditionTrue, timeoutEtcdConditionChange)
	waitForEtcdCondition(g, testEnv, logger, etcd, druidv1alpha1.ConditionTypeAllMembersReady, druidv1alpha1.ConditionTrue, timeoutEtcdConditionChange)

	// The validator pod has to be scheduled before the nodes are cordoned.
	logger.Info("starting zero-downtime validator job")
	testEnv.DeployZeroDowntimeValidatorJob(g, testNamespace, druidv1alpha1.GetClientServiceName(etcd.ObjectMeta), *etcd.Spec.Etcd.ClientPort, etcd.Spec.Etcd.ClientUrlTLS, timeoutDeployJob)

	snapshot := snapshotStsPods(g, testEnv, logger, etcd)
	recorder := startPodEventRecorder(g, testEnv, etcd)
	defer recorder.stop()

	changes.cordonNodes(g)
	triggerPodTemplateUpdate(g, testEnv, logger, etcd)
	replacement := waitForPendingReplacement(g, testEnv, logger, etcd, snapshot, timeoutReplacementPending)

	expectPodsUntouched(g, testEnv, logger, etcd, snapshot, snapshot.podNamesExcept(replacement.Name), podsUntouchedWindow)
	waitForEtcdCondition(g, testEnv, logger, etcd, druidv1alpha1.ConditionTypeAllMembersUpdated, druidv1alpha1.ConditionFalse, timeoutEtcdConditionChange)
	waitForEtcdCondition(g, testEnv, logger, etcd, druidv1alpha1.ConditionTypeAllMembersReady, druidv1alpha1.ConditionFalse, timeoutAllMembersNotReady)

	changes.uncordonNodes(g)
	waitForOnDeleteRolloutComplete(g, testEnv, logger, etcd, timeoutOnDeleteRolloutComplete)
	testEnv.CheckEtcdReady(g, etcd, timeoutEtcdUpdation)
	assertRolloutConditions(g, testEnv, logger, etcd, timeoutEtcdUpdation)
	expectAllPodsReplaced(g, testEnv, etcd, snapshot)

	logger.Info("checking for downtime")
	testEnv.CheckForDowntime(g, testNamespace, false)
	expectReplacementReadyBeforeNextDeletion(g, recorder.recordedPods(g), snapshot, replacement)
	testSucceeded = true
}

// testOnDeleteScaleUpPlusTemplateChange creates a 1-replica Etcd, then scales it out to 3 with a template change in the same update.
func testOnDeleteScaleUpPlusTemplateChange(t *testing.T, tcName string, log logr.Logger) {
	g := NewWithT(t)
	var testSucceeded bool

	testNamespace := testutils.GenerateTestNamespaceNameWithTestCaseName(t, onDeleteTestNamespacePrefix, tcName, 4)
	logger := log.WithName(tcName).WithValues("etcdName", defaultEtcdName, "namespace", testNamespace)
	defer func() {
		e2eutils.CleanupTestArtifacts(retainTestArtifacts, testSucceeded, testEnv, logger, g, testNamespace)
	}()
	e2eutils.InitializeTestCase(g, testEnv, logger, testNamespace, defaultEtcdName, druidv1alpha1.StorageProvider("none"))

	etcd := newOnDeleteTestEtcd(testNamespace, 1)
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

// testOnDeleteSingleReplicaRollout verifies that a single-replica (replicas=1) OnDelete
// cluster rolls out a spec change to completion and returns to a ready state.
func testOnDeleteSingleReplicaRollout(t *testing.T, tcName string, log logr.Logger) {
	g := NewWithT(t)
	var testSucceeded bool

	testNamespace := testutils.GenerateTestNamespaceNameWithTestCaseName(t, onDeleteTestNamespacePrefix, tcName, 4)
	logger := log.WithName(tcName).WithValues("etcdName", defaultEtcdName, "namespace", testNamespace)
	defer func() {
		e2eutils.CleanupTestArtifacts(retainTestArtifacts, testSucceeded, testEnv, logger, g, testNamespace)
	}()
	e2eutils.InitializeTestCase(g, testEnv, logger, testNamespace, defaultEtcdName, druidv1alpha1.StorageProvider("none"))

	etcd := newOnDeleteTestEtcd(testNamespace, 1)
	logger.Info("creating Etcd (1 replica)")
	testEnv.CreateAndCheckEtcd(g, etcd, timeoutEtcdCreation)
	waitForStsUpdateStrategy(g, testEnv, logger, etcd, appsv1.OnDeleteStatefulSetStrategyType, timeoutOnDeleteRolloutStart)
	snapshot := snapshotStsPods(g, testEnv, logger, etcd)

	fetched, err := testEnv.GetEtcd(etcd.Name, etcd.Namespace)
	g.Expect(err).ToNot(HaveOccurred())
	logger.Info("updating Etcd spec")
	fetched.Spec.Etcd.Metrics = ptr.To(druidv1alpha1.Extensive)
	testEnv.UpdateAndCheckEtcd(g, fetched, timeoutEtcdUpdation)

	waitForOnDeleteRolloutComplete(g, testEnv, logger, fetched, timeoutOnDeleteRolloutComplete)
	testEnv.CheckEtcdReady(g, etcd, timeoutEtcdUpdation)
	assertRolloutConditions(g, testEnv, logger, fetched, timeoutEtcdUpdation)
	expectAllPodsReplaced(g, testEnv, etcd, snapshot)
	testSucceeded = true
}

// testOnDeleteGateOffUsesRollingUpdate asserts RollingUpdate by waiting for the StatefulSet's spec.updateStrategy.type
// to be RollingUpdate and for a spec update to complete through UpdateAndCheckEtcd.
func testOnDeleteGateOffUsesRollingUpdate(t *testing.T, tcName string, log logr.Logger) {
	g := NewWithT(t)
	var testSucceeded bool

	testNamespace := testutils.GenerateTestNamespaceNameWithTestCaseName(t, onDeleteTestNamespacePrefix, tcName, 4)
	logger := log.WithName(tcName).WithValues("etcdName", defaultEtcdName, "namespace", testNamespace)
	defer func() {
		e2eutils.CleanupTestArtifacts(retainTestArtifacts, testSucceeded, testEnv, logger, g, testNamespace)
	}()
	e2eutils.InitializeTestCase(g, testEnv, logger, testNamespace, defaultEtcdName, druidv1alpha1.StorageProvider("none"))

	etcd := newOnDeleteTestEtcd(testNamespace, 3)
	logger.Info("creating Etcd (gate disabled: expect RollingUpdate strategy)")
	testEnv.CreateAndCheckEtcd(g, etcd, timeoutEtcdCreation)

	waitForStsUpdateStrategy(g, testEnv, logger, etcd, appsv1.RollingUpdateStatefulSetStrategyType, timeoutOnDeleteRolloutStart)
	logger.Info("confirmed: StatefulSet uses RollingUpdate when gate is disabled")

	fetched, err := testEnv.GetEtcd(etcd.Name, etcd.Namespace)
	g.Expect(err).ToNot(HaveOccurred())
	fetched.Spec.Etcd.Metrics = ptr.To(druidv1alpha1.Extensive)
	testEnv.UpdateAndCheckEtcd(g, fetched, timeoutEtcdUpdation)
	logger.Info("update completed under RollingUpdate strategy")

	testSucceeded = true
}
