// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package etcdopstask

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

// TestAdmit validates the task-agnostic admit conditions for an EtcdOpsTask.
func TestAdmit(t *testing.T) {
	t.Parallel()

	for _, provider := range providers {
		if provider == "none" {
			continue
		}
		t.Run("duplicate-rejected-same-etcd-"+e2eutils.GetProviderSuffix(provider), testAdmitDuplicateRejected(provider))
		t.Run("independent-different-etcd-"+e2eutils.GetProviderSuffix(provider), testAdmitIndependentEtcds(provider))
	}
}

func testAdmitDuplicateRejected(provider druidv1alpha1.StorageProvider) func(*testing.T) {
	return func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		log := testr.NewWithOptions(t, testr.Options{LogTimestamp: true})
		var testSucceeded bool

		tcName := fmt.Sprintf("admit-duplicate-rejected-same-etcd-%s", e2eutils.GetProviderSuffix(provider))
		testNamespace := testutils.GenerateTestNamespaceNameWithTestCaseName(t, testNamespacePrefix, tcName, 4)
		logger := log.WithName(tcName).WithValues("etcdName", e2eutils.DefaultEtcdName, "namespace", testNamespace)
		defer func() {
			e2eutils.CleanupTestArtifacts(retainTestArtifacts, testSucceeded, testEnv, logger, g, testNamespace)
		}()
		e2eutils.InitializeTestCase(g, testEnv, logger, testNamespace, e2eutils.DefaultEtcdName, provider)

		logger.Info("creating Etcd resource")
		etcd := testutils.EtcdBuilderWithoutDefaults(e2eutils.DefaultEtcdName, testNamespace).
			WithReplicas(1).
			WithEtcdClientPort(ptr.To[int32](2379)).
			WithClientTLS().
			WithPeerTLS().
			WithGRPCGatewayEnabled().
			WithDefaultBackup().
			WithStorageProvider(provider, fmt.Sprintf("%s/%s", testNamespace, e2eutils.DefaultEtcdName)).
			WithBackupRestoreTLS().
			Build()
		testEnv.CreateAndCheckEtcd(g, etcd, timeoutEtcdCreation)
		logger.Info("successfully created Etcd resource")

		logger.Info("loading data into etcd to ensure snapshot takes measurable time")
		testEnv.DeployEtcdLoaderJob(g, testNamespace, druidv1alpha1.GetClientServiceName(etcd.ObjectMeta), *etcd.Spec.Etcd.ClientPort, etcd.Spec.Etcd.ClientUrlTLS, 1000, timeoutDeployJob)

		firstTask := newOnDemandSnapshotTask(e2eutils.DefaultEtcdName+"-first-task", testNamespace, e2eutils.DefaultEtcdName, druidv1alpha1.OnDemandSnapshotTypeFull)

		logger.Info("creating first EtcdOpsTask")
		testEnv.CreateEtcdOpsTask(g, firstTask)

		secondTask := newOnDemandSnapshotTask(e2eutils.DefaultEtcdName+"-second-task", testNamespace, e2eutils.DefaultEtcdName, druidv1alpha1.OnDemandSnapshotTypeFull)

		logger.Info("creating second EtcdOpsTask for the same Etcd (should be rejected)")
		testEnv.CreateEtcdOpsTask(g, secondTask)

		logger.Info("waiting for second task to be rejected")
		testEnv.CheckEtcdOpsTaskState(g, secondTask, druidv1alpha1.TaskStateRejected, timeoutEtcdOpsTaskCompletion)
		logger.Info("second task correctly rejected")

		logger.Info("waiting for second task to be deleted after TTL")
		testEnv.CheckEtcdOpsTaskDeleted(g, secondTask, defaultTaskDeletionTimeout)
		logger.Info("second task deleted after TTL")

		testSucceeded = true
	}
}

func testAdmitIndependentEtcds(provider druidv1alpha1.StorageProvider) func(*testing.T) {
	return func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		log := testr.NewWithOptions(t, testr.Options{LogTimestamp: true})
		var testSucceeded bool

		tcName := fmt.Sprintf("admit-independent-different-etcd-%s", e2eutils.GetProviderSuffix(provider))
		ns1 := testutils.GenerateTestNamespaceNameWithTestCaseName(t, testNamespacePrefix, tcName+"-a", 4)
		ns2 := testutils.GenerateTestNamespaceNameWithTestCaseName(t, testNamespacePrefix, tcName+"-b", 4)
		logger := log.WithName(tcName)
		defer func() {
			e2eutils.CleanupTestArtifacts(retainTestArtifacts, testSucceeded, testEnv, logger, g, ns1)
			e2eutils.CleanupTestArtifacts(retainTestArtifacts, testSucceeded, testEnv, logger, g, ns2)
		}()

		etcdNameA := "etcd-a"
		etcdNameB := "etcd-b"

		e2eutils.InitializeTestCase(g, testEnv, logger, ns1, etcdNameA, provider)
		e2eutils.InitializeTestCase(g, testEnv, logger, ns2, etcdNameB, provider)

		logger.Info("creating first Etcd resource", "namespace", ns1, "etcdName", etcdNameA)
		etcdA := testutils.EtcdBuilderWithoutDefaults(etcdNameA, ns1).
			WithReplicas(1).
			WithEtcdClientPort(ptr.To[int32](2379)).
			WithClientTLS().
			WithPeerTLS().
			WithDefaultBackup().
			WithStorageProvider(provider, fmt.Sprintf("%s/%s", ns1, etcdNameA)).
			WithBackupRestoreTLS().
			Build()
		testEnv.CreateAndCheckEtcd(g, etcdA, timeoutEtcdCreation)

		logger.Info("creating second Etcd resource", "namespace", ns2, "etcdName", etcdNameB)
		etcdB := testutils.EtcdBuilderWithoutDefaults(etcdNameB, ns2).
			WithReplicas(1).
			WithEtcdClientPort(ptr.To[int32](2379)).
			WithClientTLS().
			WithPeerTLS().
			WithDefaultBackup().
			WithStorageProvider(provider, fmt.Sprintf("%s/%s", ns2, etcdNameB)).
			WithBackupRestoreTLS().
			Build()
		testEnv.CreateAndCheckEtcd(g, etcdB, timeoutEtcdCreation)

		taskA := newOnDemandSnapshotTask(etcdNameA+"-task", ns1, etcdNameA, druidv1alpha1.OnDemandSnapshotTypeFull)

		taskB := newOnDemandSnapshotTask(etcdNameB+"-task", ns2, etcdNameB, druidv1alpha1.OnDemandSnapshotTypeFull)

		logger.Info("creating both EtcdOpsTask resources concurrently")
		testEnv.CreateEtcdOpsTask(g, taskA)
		testEnv.CreateEtcdOpsTask(g, taskB)

		logger.Info("waiting for both tasks to succeed independently")
		testEnv.CheckEtcdOpsTaskState(g, taskA, druidv1alpha1.TaskStateSucceeded, timeoutEtcdOpsTaskCompletion)
		testEnv.CheckEtcdOpsTaskState(g, taskB, druidv1alpha1.TaskStateSucceeded, timeoutEtcdOpsTaskCompletion)
		logger.Info("both tasks succeeded independently")

		logger.Info("waiting for both tasks to be deleted after TTL")
		testEnv.CheckEtcdOpsTaskDeleted(g, taskA, defaultTaskDeletionTimeout)
		testEnv.CheckEtcdOpsTaskDeleted(g, taskB, defaultTaskDeletionTimeout)
		logger.Info("both tasks deleted after TTL")

		testSucceeded = true
	}
}
