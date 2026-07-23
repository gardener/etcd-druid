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
		t.Run("independent-different-namespace-"+e2eutils.GetProviderSuffix(provider), testAdmitIndependentEtcds(provider, false))
		t.Run("independent-same-namespace-"+e2eutils.GetProviderSuffix(provider), testAdmitIndependentEtcds(provider, true))
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
		testEnv.DeployEtcdLoaderJob(g, testNamespace, druidv1alpha1.GetClientServiceName(etcd.ObjectMeta), *etcd.Spec.Etcd.ClientPort, etcd.Spec.Etcd.ClientUrlTLS, 2000, 256*1024, timeoutLargeDeployJob)

		firstTask := newOnDemandSnapshotTask(e2eutils.DefaultEtcdName+"-first-task", testNamespace, e2eutils.DefaultEtcdName, druidv1alpha1.OnDemandSnapshotTypeFull)

		logger.Info("creating first EtcdOpsTask")
		testEnv.CreateEtcdOpsTask(g, firstTask)

		fetchedFirstTask, err := testEnv.GetEtcdOpsTask(firstTask.Name, firstTask.Namespace)
		g.Expect(err).NotTo(HaveOccurred())
		if fetchedFirstTask.Status.State == nil || *fetchedFirstTask.Status.State != druidv1alpha1.TaskStateInProgress {
			logger.Info("waiting for first task to reach InProgress before creating the duplicate")
			testEnv.CheckEtcdOpsTaskState(g, firstTask, druidv1alpha1.TaskStateInProgress, timeoutEtcdOpsTaskCompletion)
		}

		secondTask := newOnDemandSnapshotTask(e2eutils.DefaultEtcdName+"-second-task", testNamespace, e2eutils.DefaultEtcdName, druidv1alpha1.OnDemandSnapshotTypeFull)

		logger.Info("creating second EtcdOpsTask for the same Etcd (should be rejected)")
		testEnv.CreateEtcdOpsTask(g, secondTask)

		logger.Info("waiting for second task to be rejected")
		testEnv.CheckEtcdOpsTaskState(g, secondTask, druidv1alpha1.TaskStateRejected, timeoutEtcdOpsTaskCompletion)
		logger.Info("second task correctly rejected")

		logger.Info("waiting for second task to be deleted after TTL")
		testEnv.CheckEtcdOpsTaskDeleted(g, secondTask, defaultTaskDeletionTimeout)
		logger.Info("second task deleted after TTL")

		logger.Info("waiting for first task to be deleted after TTL")
		testEnv.CheckEtcdOpsTaskDeleted(g, firstTask, defaultTaskDeletionTimeout)
		logger.Info("first task deleted after TTL")

		testSucceeded = true
	}
}

func testAdmitIndependentEtcds(provider druidv1alpha1.StorageProvider, sameNamespace bool) func(*testing.T) {
	return func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		log := testr.NewWithOptions(t, testr.Options{LogTimestamp: true})
		var testSucceeded bool

		layout := "different-namespace"
		if sameNamespace {
			layout = "same-namespace"
		}
		tcName := fmt.Sprintf("admit-independent-%s-%s", layout, e2eutils.GetProviderSuffix(provider))

		etcdNameA := "etcd-a"
		etcdNameB := "etcd-b"

		var nsA, nsB string
		if sameNamespace {
			nsA = testutils.GenerateTestNamespaceNameWithTestCaseName(t, testNamespacePrefix, tcName, 4)
			nsB = nsA
		} else {
			nsA = testutils.GenerateTestNamespaceNameWithTestCaseName(t, testNamespacePrefix, tcName+"-a", 4)
			nsB = testutils.GenerateTestNamespaceNameWithTestCaseName(t, testNamespacePrefix, tcName+"-b", 4)
		}
		logger := log.WithName(tcName)
		defer func() {
			e2eutils.CleanupTestArtifacts(retainTestArtifacts, testSucceeded, testEnv, logger, g, nsA)
			if !sameNamespace {
				e2eutils.CleanupTestArtifacts(retainTestArtifacts, testSucceeded, testEnv, logger, g, nsB)
			}
		}()

		if sameNamespace {
			e2eutils.CreateNamespace(g, testEnv, logger, nsA)
			e2eutils.CreateBackupSecret(g, testEnv, logger, nsA, provider)
		} else {
			e2eutils.InitializeTestCase(g, testEnv, logger, nsA, etcdNameA, provider)
			e2eutils.InitializeTestCase(g, testEnv, logger, nsB, etcdNameB, provider)
		}

		logger.Info("creating first Etcd resource", "namespace", nsA, "etcdName", etcdNameA)
		etcdA := buildIndependentEtcd(etcdNameA, nsA, provider, sameNamespace)
		testEnv.CreateAndCheckEtcd(g, etcdA, timeoutEtcdCreation)

		logger.Info("creating second Etcd resource", "namespace", nsB, "etcdName", etcdNameB)
		etcdB := buildIndependentEtcd(etcdNameB, nsB, provider, sameNamespace)
		testEnv.CreateAndCheckEtcd(g, etcdB, timeoutEtcdCreation)

		taskA := newOnDemandSnapshotTask(etcdNameA+"-task", nsA, etcdNameA, druidv1alpha1.OnDemandSnapshotTypeFull)
		taskB := newOnDemandSnapshotTask(etcdNameB+"-task", nsB, etcdNameB, druidv1alpha1.OnDemandSnapshotTypeFull)

		logger.Info("creating both EtcdOpsTask resources to be admitted and executed concurrently")
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

func buildIndependentEtcd(name, namespace string, provider druidv1alpha1.StorageProvider, sameNamespace bool) *druidv1alpha1.Etcd {
	builder := testutils.EtcdBuilderWithoutDefaults(name, namespace).
		WithReplicas(1).
		WithEtcdClientPort(ptr.To[int32](2379)).
		WithDefaultBackup().
		WithStorageProvider(provider, fmt.Sprintf("%s/%s", namespace, name))
	if !sameNamespace {
		builder = builder.WithClientTLS().WithPeerTLS().WithBackupRestoreTLS()
	}
	return builder.Build()
}
