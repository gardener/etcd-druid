// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package ondemandsnapshot

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	druidapicommon "github.com/gardener/etcd-druid/api/common"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/api/core/v1alpha1/crds"
	taskhandler "github.com/gardener/etcd-druid/internal/controller/etcdopstask/handler"
	"github.com/gardener/etcd-druid/internal/controller/etcdopstask/handler/ondemandsnapshot"
	"github.com/gardener/etcd-druid/test/it/assets"
	etcdopstasktest "github.com/gardener/etcd-druid/test/it/controller/etcdopstask"
	"github.com/gardener/etcd-druid/test/it/setup"
	testutils "github.com/gardener/etcd-druid/test/utils"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/gomega"
)

const testNamespacePrefix = "etcdopstask-test"

var (
	sharedITTestEnv         setup.DruidTestEnvironment
	sharedReconcilerTestEnv etcdopstasktest.ReconcilerTestEnv
	k8sVersionAbove129      bool
)

func TestMain(m *testing.M) {
	var (
		itTestEnvCloser setup.DruidTestEnvCloser
		err             error
	)

	k8sVersion, err := assets.GetK8sVersionFromEnv()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Failed to get the kubernetes version: %v\n", err)
		os.Exit(1)
	}

	k8sVersionAbove129, err = crds.IsK8sVersionEqualToOrAbove129(k8sVersion)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to compare k8s version: %v\n", err)
		os.Exit(1)
	}

	etcdOpsTaskCrd, err := assets.GetEtcdOpsTaskCrd(k8sVersion)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to get EtcdOpsTask CRD: %v\n", err)
		os.Exit(1)
	}

	etcdCrd, err := assets.GetEtcdCrd(k8sVersion)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to get Etcd CRD: %v\n", err)
		os.Exit(1)
	}

	sharedITTestEnv, itTestEnvCloser, err = setup.NewDruidTestEnvironment(testNamespacePrefix, []*apiextensionsv1.CustomResourceDefinition{
		etcdOpsTaskCrd,
		etcdCrd,
	})
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to create integration test environment: %v\n", err)
		os.Exit(1)
	}

	// Create shared reconciler test environment
	sharedReconcilerTestEnv = etcdopstasktest.InitializeEtcdOpsTaskReconcilerTestEnv(nil, sharedITTestEnv, testutils.NewTestClientBuilder())

	// os.Exit() does not respect defer statements
	exitCode := m.Run()
	itTestEnvCloser()
	os.Exit(exitCode)
}

// -----------------------------------OnDemandSnapshot Task Tests-----------------------------------------------------

// TestETcdOpsTaskConfigValidation tests the validation of EtcdOpsTask spec.config values.
func TestEtcdOpsTaskConfigValidation(t *testing.T) {
	tests := []struct {
		name string
		fn   func(t *testing.T, namespace string, reconcilerTestEnv etcdopstasktest.ReconcilerTestEnv)
	}{
		{"should fail to create task with empty config", testEtcdOpsTaskCreationFailEmptyConfig},
		{"should successfully create task with onDemandSnapshot config", testEtcdOpsTaskCreationSuccessValidConfig},
	}

	g := NewWithT(t)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			testNs := testutils.GenerateTestNamespaceName(t, testNamespacePrefix)
			t.Logf("successfully created namespace: %s to run test => '%s'", testNs, t.Name())
			g.Expect(sharedReconcilerTestEnv.ItTestEnv.CreateTestNamespace(testNs)).To(Succeed())
			tc.fn(t, testNs, sharedReconcilerTestEnv)
		})
	}
}

// testEtcdOpsTaskCreationFailEmptyConfig tests that creating an EtcdOpsTask with empty config fails.
func testEtcdOpsTaskCreationFailEmptyConfig(t *testing.T, namespace string, reconcilerTestEnv etcdopstasktest.ReconcilerTestEnv) {
	g := NewWithT(t)
	cl := reconcilerTestEnv.ItTestEnv.GetClient()
	ctx := context.Background()

	emptyConfigTask := testutils.EtcdOpsTaskBuilderWithoutDefaults("test-task-empty-config", namespace).
		WithEtcdName("test-etcd").
		WithTTLSecondsAfterFinished(5).
		Build()

	err := cl.Create(ctx, emptyConfigTask)
	g.Expect(err).To(HaveOccurred())
	t.Logf("empty config test: task creation failed as expected with error: %v", err)
}

// testEtcdOpsTaskCreationSuccessValidConfig tests that creating an EtcdOpsTask with valid config succeeds.
func testEtcdOpsTaskCreationSuccessValidConfig(t *testing.T, namespace string, reconcilerTestEnv etcdopstasktest.ReconcilerTestEnv) {
	g := NewWithT(t)
	cl := reconcilerTestEnv.ItTestEnv.GetClient()
	ctx := context.Background()

	validConfigTask := testutils.EtcdOpsTaskBuilderWithoutDefaults("test-task-valid-config", namespace).
		WithEtcdName("test-etcd").
		WithTTLSecondsAfterFinished(60).
		WithOnDemandSnapshotConfig(&druidv1alpha1.OnDemandSnapshotConfig{
			Type:               druidv1alpha1.OnDemandSnapshotTypeFull,
			TimeoutSecondsFull: ptr.To(int32(150)),
			IsFinal:            ptr.To(false),
		}).Build()

	err := cl.Create(ctx, validConfigTask)
	g.Expect(err).NotTo(HaveOccurred())
	t.Logf("valid config test: created etcdopstask %s/%s successfully", validConfigTask.Namespace, validConfigTask.Name)
}

// TestEtcdOpsTaskAdmitConditions tests the admit conditions of EtcdOpsTask reconciler.
func TestEtcdOpsTaskAdmitConditions(t *testing.T) {
	tests := []struct {
		name string
		fn   func(t *testing.T, namespace string, reconcilerTestEnv etcdopstasktest.ReconcilerTestEnv)
	}{
		{"Task should be rejected if the etcd's backup is disabled", testEtcdOpsTaskRejectedIfEtcdBackupDisabled},
		{"Task should be rejected if the etcd is not in ready state", testEtcdOpsTaskRejectedIfEtcdNotReady},
		{"Task should be rejected if there is a duplicate task for the same etcd", testEtcdOpsTaskRejectedDuplicateTask},
	}

	g := NewWithT(t)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			testNs := testutils.GenerateTestNamespaceName(t, testNamespacePrefix)
			t.Logf("successfully created namespace: %s to run test => '%s'", testNs, t.Name())
			g.Expect(sharedReconcilerTestEnv.ItTestEnv.CreateTestNamespace(testNs)).To(Succeed())
			tc.fn(t, testNs, sharedReconcilerTestEnv)
		})
	}
}

// testEtcdOpsTaskRejctedIfEtcdBackupDisabled tests that an EtcdOpsTask is rejected if the referenced etcd has backup disabled.
func testEtcdOpsTaskRejectedIfEtcdBackupDisabled(t *testing.T, namespace string, reconcilerTestEnv etcdopstasktest.ReconcilerTestEnv) {
	g := NewWithT(t)

	etcdInstance := testutils.EtcdBuilderWithDefaults("test-etcd", namespace).Build()
	etcdInstance.Spec.Backup = druidv1alpha1.BackupSpec{}

	cl := reconcilerTestEnv.ItTestEnv.GetClient()
	ctx := context.Background()

	g.Expect(cl.Create(ctx, etcdInstance)).To(Succeed())
	t.Logf("created etcd instance %s/%s with backup disabled", etcdInstance.Namespace, etcdInstance.Name)

	etcdOpsTaskInstance := testutils.EtcdOpsTaskBuilderWithoutDefaults("test-task", namespace).
		WithEtcdName("test-etcd").
		WithTTLSecondsAfterFinished(20).
		WithOnDemandSnapshotConfig(&druidv1alpha1.OnDemandSnapshotConfig{
			Type:               druidv1alpha1.OnDemandSnapshotTypeFull,
			TimeoutSecondsFull: ptr.To(int32(150)),
			IsFinal:            ptr.To(false),
		}).Build()

	g.Expect(cl.Create(ctx, etcdOpsTaskInstance)).To(Succeed())
	t.Logf("created etcdopstask %s/%s", etcdOpsTaskInstance.Namespace, etcdOpsTaskInstance.Name)

	expectedErrorCode := ondemandsnapshot.ErrBackupNotEnabled
	etcdopstasktest.AssertEtcdOpsTaskStateAndErrorCode(ctx, g, t, cl, etcdOpsTaskInstance, druidv1alpha1.TaskStateRejected, druidv1alpha1.LastOperationTypeAdmit, druidv1alpha1.LastOperationStateFailed, &expectedErrorCode)

	t.Logf("test completed: etcdopstask was correctly rejected due to backup being disabled")
}

// testEtcdOpsTaskRejectedIfEtcdNotReady tests that an EtcdOpsTask is rejected if the referenced etcd is not in ready state.
func testEtcdOpsTaskRejectedIfEtcdNotReady(t *testing.T, namespace string, reconcilerTestEnv etcdopstasktest.ReconcilerTestEnv) {
	g := NewWithT(t)

	etcdStatusConditions := []druidv1alpha1.Condition{
		{
			Type:    druidv1alpha1.ConditionTypeReady,
			Status:  druidv1alpha1.ConditionFalse,
			Message: "Etcd is not ready for testing",
		},
	}
	etcdInstance := testutils.EtcdBuilderWithDefaults("test-etcd", namespace).WithStatusConditions(etcdStatusConditions).Build()

	ctx := context.Background()
	cl := reconcilerTestEnv.ItTestEnv.GetClient()

	g.Expect(cl.Create(ctx, etcdInstance)).To(Succeed())

	g.Eventually(func() error {
		return cl.Get(ctx, client.ObjectKeyFromObject(etcdInstance), etcdInstance)
	}, 5*time.Second, 100*time.Millisecond).Should(Succeed(), "etcd should be visible after creation")

	t.Logf("created etcd instance %s/%s with not ready condition", etcdInstance.Namespace, etcdInstance.Name)

	etcdOpsTaskInstance := testutils.EtcdOpsTaskBuilderWithoutDefaults("test-task", namespace).
		WithEtcdName("test-etcd").
		WithTTLSecondsAfterFinished(20).
		WithOnDemandSnapshotConfig(&druidv1alpha1.OnDemandSnapshotConfig{
			Type:               druidv1alpha1.OnDemandSnapshotTypeFull,
			TimeoutSecondsFull: ptr.To(int32(150)),
			IsFinal:            ptr.To(false),
		}).Build()

	g.Expect(cl.Create(ctx, etcdOpsTaskInstance)).To(Succeed())
	t.Logf("created etcdopstask %s/%s", etcdOpsTaskInstance.Namespace, etcdOpsTaskInstance.Name)

	expectedErrorCode := ondemandsnapshot.ErrEtcdNotReady
	etcdopstasktest.AssertEtcdOpsTaskStateAndErrorCode(ctx, g, t, cl, etcdOpsTaskInstance, druidv1alpha1.TaskStateRejected, druidv1alpha1.LastOperationTypeAdmit, druidv1alpha1.LastOperationStateFailed, &expectedErrorCode)
	etcdopstasktest.AssertEtcdOpsTaskDeletedAfterTTL(ctx, g, t, cl, etcdOpsTaskInstance, druidv1alpha1.LastOperationTypeAdmit)

	t.Logf("test completed: etcdopstask was correctly rejected due to etcd not being ready")
}

// testEtcdOpsTaskRejectedDuplicateTask tests that a duplicate EtcdOpsTask for the same etcd is rejected.
func testEtcdOpsTaskRejectedDuplicateTask(t *testing.T, namespace string, reconcilerTestEnv etcdopstasktest.ReconcilerTestEnv) {
	g := NewWithT(t)
	cl := reconcilerTestEnv.ItTestEnv.GetClient()
	ctx := context.Background()

	_ = etcdopstasktest.DeployReadyEtcd(ctx, g, t, cl, namespace)

	ondemandSnapshotTask := testutils.EtcdOpsTaskBuilderWithoutDefaults("test-task", namespace).
		WithEtcdName("test-etcd").
		WithTTLSecondsAfterFinished(20).
		WithOnDemandSnapshotConfig(&druidv1alpha1.OnDemandSnapshotConfig{
			Type:               druidv1alpha1.OnDemandSnapshotTypeFull,
			TimeoutSecondsFull: ptr.To(int32(150)),
			IsFinal:            ptr.To(false),
		}).Build()

	g.Expect(cl.Create(ctx, ondemandSnapshotTask)).To(Succeed())

	g.Eventually(func() bool {
		if err := cl.Get(ctx, client.ObjectKeyFromObject(ondemandSnapshotTask), ondemandSnapshotTask); err != nil {
			return false
		}
		if ondemandSnapshotTask.Status.State != nil {
			t.Logf("current task state: %s", *ondemandSnapshotTask.Status.State)
		}
		return ondemandSnapshotTask.Status.State != nil && *ondemandSnapshotTask.Status.State == druidv1alpha1.TaskStateInProgress
	}, 5*time.Second, 100*time.Millisecond).Should(BeTrue(), "first task should reach InProgress state before creating duplicate")

	duplicateOndemandSnapshotTask := testutils.EtcdOpsTaskBuilderWithoutDefaults("duplicate-ondemand-snapshot", namespace).
		WithEtcdName("test-etcd").
		WithTTLSecondsAfterFinished(20).
		WithOnDemandSnapshotConfig(&druidv1alpha1.OnDemandSnapshotConfig{
			Type:               druidv1alpha1.OnDemandSnapshotTypeFull,
			TimeoutSecondsFull: ptr.To(int32(150)),
			IsFinal:            ptr.To(false),
		}).Build()

	g.Expect(cl.Create(ctx, duplicateOndemandSnapshotTask)).To(Succeed())
	t.Logf("created duplicate etcdopstask %s/%s", duplicateOndemandSnapshotTask.Namespace, duplicateOndemandSnapshotTask.Name)

	expectedErrorCode := taskhandler.ErrDuplicateTask
	etcdopstasktest.AssertEtcdOpsTaskStateAndErrorCode(ctx, g, t, cl, duplicateOndemandSnapshotTask, druidv1alpha1.TaskStateRejected, druidv1alpha1.LastOperationTypeAdmit, druidv1alpha1.LastOperationStateFailed, &expectedErrorCode)
	etcdopstasktest.AssertEtcdOpsTaskDeletedAfterTTL(ctx, g, t, cl, duplicateOndemandSnapshotTask, druidv1alpha1.LastOperationTypeAdmit)

	t.Logf("test completed: duplicate etcdopstask was correctly rejected")
}

// TestEtcdOpsTaskLifecycleWithHTTPMock tests the lifecycle with HTTP client mocking.
func TestEtcdOpsTaskLifecycle(t *testing.T) {
	tests := []struct {
		name       string
		fn         func(t *testing.T, namespace string, reconcilerTestEnv etcdopstasktest.ReconcilerTestEnv)
		httpClient *http.Client
	}{
		{
			"Finalizer is added on creation",
			testEtcdOpsTaskAddFinalizer,
			nil,
		},
		{
			"Successful Lifecycle",
			testEtcdOpsTaskSuccessfulLifecycle,
			&http.Client{
				Transport: &testutils.MockRoundTripper{
					Response: &http.Response{
						StatusCode: http.StatusOK,
						Status:     "200 OK",
						Body:       io.NopCloser(strings.NewReader("OK")),
					},
					Err: nil,
				},
			},
		},
		{
			"UnSuccessful Lifecycle: Run fails",
			testEtcdOpsTaskUnsuccessfulLifecycle,
			&http.Client{
				Transport: &testutils.MockRoundTripper{
					Response: &http.Response{
						StatusCode: http.StatusInternalServerError,
						Status:     "500 Internal Server Error",
						Body:       io.NopCloser(strings.NewReader("Internal Server Error")),
					},
					Err: nil,
				},
			},
		},
	}

	g := NewWithT(t)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			testNs := testutils.GenerateTestNamespaceName(t, testNamespacePrefix)
			t.Logf("successfully created namespace: %s to run test => '%s'", testNs, t.Name())
			g.Expect(sharedReconcilerTestEnv.ItTestEnv.CreateTestNamespace(testNs)).To(Succeed())

			if tc.httpClient != nil {
				sharedReconcilerTestEnv.InjectHTTPClient(tc.httpClient)
			}
			tc.fn(t, testNs, sharedReconcilerTestEnv)
		})
	}
}

// testEtcdOpsTaskAddFinalizer tests that the finalizer is added to the EtcdOpsTask upon creation.
func testEtcdOpsTaskAddFinalizer(t *testing.T, namespace string, reconcilerTestEnv etcdopstasktest.ReconcilerTestEnv) {
	g := NewWithT(t)

	etcdopstaskInstance := testutils.EtcdOpsTaskBuilderWithoutDefaults("test-task", namespace).
		WithEtcdName("test-etcd").
		WithTTLSecondsAfterFinished(20).
		WithOnDemandSnapshotConfig(&druidv1alpha1.OnDemandSnapshotConfig{
			Type:               druidv1alpha1.OnDemandSnapshotTypeFull,
			TimeoutSecondsFull: ptr.To(int32(150)),
			IsFinal:            ptr.To(false),
		}).Build()

	cl := reconcilerTestEnv.ItTestEnv.GetClient()
	ctx := context.Background()
	g.Expect(cl.Create(ctx, etcdopstaskInstance)).To(Succeed())

	g.Eventually(func() bool {
		if err := cl.Get(ctx, client.ObjectKeyFromObject(etcdopstaskInstance), etcdopstaskInstance); err != nil {
			return false
		}
		return slices.Contains(etcdopstaskInstance.Finalizers, druidapicommon.EtcdOpsTaskFinalizerName)
	}, 5*time.Second, 100*time.Millisecond).Should(BeTrue(), "finalizer should be added to etcdopstask")

	t.Logf("test completed: finalizer %s was successfully added to etcdopstask %s/%s", druidapicommon.EtcdOpsTaskFinalizerName, etcdopstaskInstance.Namespace, etcdopstaskInstance.Name)
}

// testEtcdOpsTaskSuccessfulLifecycle tests the successful lifecycle with actual HTTP client mocking.
func testEtcdOpsTaskSuccessfulLifecycle(t *testing.T, namespace string, reconcilerTestEnv etcdopstasktest.ReconcilerTestEnv) {
	g := NewWithT(t)

	cl := reconcilerTestEnv.ItTestEnv.GetClient()
	ctx := context.Background()

	// Setup ready etcd instance
	_ = etcdopstasktest.DeployReadyEtcd(ctx, g, t, cl, namespace)

	etcdOpsTaskInstance := testutils.EtcdOpsTaskBuilderWithoutDefaults("test-task", namespace).
		WithEtcdName("test-etcd").
		WithTTLSecondsAfterFinished(20).
		WithOnDemandSnapshotConfig(&druidv1alpha1.OnDemandSnapshotConfig{
			Type:               druidv1alpha1.OnDemandSnapshotTypeFull,
			TimeoutSecondsFull: ptr.To(int32(150)),
			IsFinal:            ptr.To(false),
		}).Build()

	g.Expect(cl.Create(ctx, etcdOpsTaskInstance)).To(Succeed())

	etcdopstasktest.AssertEtcdOpsTaskStateAndErrorCode(ctx, g, t, cl, etcdOpsTaskInstance, druidv1alpha1.TaskStateSucceeded, druidv1alpha1.LastOperationTypeExecution, druidv1alpha1.LastOperationStateCompleted, nil)
	etcdopstasktest.AssertEtcdOpsTaskDeletedAfterTTL(ctx, g, t, cl, etcdOpsTaskInstance, druidv1alpha1.LastOperationTypeExecution)
	t.Logf("test completed: etcdopstask successfully completed lifecycle and was deleted after TTL")
}

// testEtcdOpsTaskUnSuccessfulLifecycle tests the unsuccessful lifecycle.
func testEtcdOpsTaskUnsuccessfulLifecycle(t *testing.T, namespace string, reconcilerTestEnv etcdopstasktest.ReconcilerTestEnv) {
	g := NewWithT(t)

	cl := reconcilerTestEnv.ItTestEnv.GetClient()
	ctx := context.Background()

	_ = etcdopstasktest.DeployReadyEtcd(ctx, g, t, cl, namespace)

	etcdOpsTaskInstance := testutils.EtcdOpsTaskBuilderWithoutDefaults("test-task", namespace).
		WithEtcdName("test-etcd").
		WithTTLSecondsAfterFinished(20).
		WithOnDemandSnapshotConfig(&druidv1alpha1.OnDemandSnapshotConfig{
			Type:               druidv1alpha1.OnDemandSnapshotTypeFull,
			TimeoutSecondsFull: ptr.To(int32(150)),
			IsFinal:            ptr.To(false),
		}).Build()

	g.Expect(cl.Create(ctx, etcdOpsTaskInstance)).To(Succeed())

	expectedErrorCode := ondemandsnapshot.ErrCreateSnapshot
	etcdopstasktest.AssertEtcdOpsTaskStateAndErrorCode(ctx, g, t, cl, etcdOpsTaskInstance, druidv1alpha1.TaskStateFailed, druidv1alpha1.LastOperationTypeExecution, druidv1alpha1.LastOperationStateFailed, &expectedErrorCode)
	etcdopstasktest.AssertEtcdOpsTaskDeletedAfterTTL(ctx, g, t, cl, etcdOpsTaskInstance, druidv1alpha1.LastOperationTypeExecution)

	t.Logf("test completed: etcdopstask failed as expected and was deleted after TTL")
}
