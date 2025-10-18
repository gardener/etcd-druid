// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package etcdopstask

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

	druidapiconstants "github.com/gardener/etcd-druid/api/common"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/api/core/v1alpha1/crds"
	"github.com/gardener/etcd-druid/internal/controller/etcdopstask"
	"github.com/gardener/etcd-druid/internal/controller/etcdopstask/handler/ondemandsnapshot"
	"github.com/gardener/etcd-druid/test/it/assets"
	"github.com/gardener/etcd-druid/test/it/setup"
	testutils "github.com/gardener/etcd-druid/test/utils"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/gomega"
)

const testNamespacePrefix = "etcdopstask-test"

var (
	sharedITTestEnv         setup.DruidTestEnvironment
	sharedReconcilerTestEnv ReconcilerTestEnv
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

	sharedITTestEnv, itTestEnvCloser, err = setup.NewDruidTestEnvironment(testNamespacePrefix, []string{
		assets.GetEtcdOpsTaskCrdPath(),
		assets.GetEtcdCrdPath(k8sVersionAbove129),
	})
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to create integration test environment: %v\n", err)
		os.Exit(1)
	}

	// Create shared reconciler test environment
	sharedReconcilerTestEnv = initializeEtcdOpsTaskReconcilerTestEnv(nil, sharedITTestEnv, testutils.NewTestClientBuilder())

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
		fn   func(t *testing.T, namespace string, reconcilerTestEnv ReconcilerTestEnv)
	}{
		{"should fail to create task with empty config", testEtcdOpsTaskCreationFailEmptyConfig},
		{"should successfully create task with onDemandSnapshot config", testEtcdOpsTaskCreationSuccessValidConfig},
	}

	g := NewWithT(t)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			testNs := testutils.GenerateTestNamespaceName(t, testNamespacePrefix)
			t.Logf("successfully created namespace: %s to run test => '%s'", testNs, t.Name())
			g.Expect(sharedReconcilerTestEnv.itTestEnv.CreateTestNamespace(testNs)).To(Succeed())
			tc.fn(t, testNs, sharedReconcilerTestEnv)
		})
	}
}

// testEtcdOpsTaskCreationFailEmptyConfig tests that creating an EtcdOpsTask with empty config fails.
func testEtcdOpsTaskCreationFailEmptyConfig(t *testing.T, namespace string, reconcilerTestEnv ReconcilerTestEnv) {
	g := NewWithT(t)
	cl := reconcilerTestEnv.itTestEnv.GetClient()
	ctx := context.Background()

	emptyConfigTask := &druidv1alpha1.EtcdOpsTask{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-task-empty-config",
			Namespace: namespace,
		},
		Spec: druidv1alpha1.EtcdOpsTaskSpec{
			Config:                  druidv1alpha1.EtcdOpsTaskConfig{},
			TTLSecondsAfterFinished: ptr.To(int32(5)),
			EtcdName:                ptr.To("test-etcd"),
		},
	}

	err := cl.Create(ctx, emptyConfigTask)
	g.Expect(err).To(HaveOccurred())
	t.Logf("empty config test: task creation failed as expected with error: %v", err)
}

// testEtcdOpsTaskCreationSuccessValidConfig tests that creating an EtcdOpsTask with valid config succeeds.
func testEtcdOpsTaskCreationSuccessValidConfig(t *testing.T, namespace string, reconcilerTestEnv ReconcilerTestEnv) {
	g := NewWithT(t)
	cl := reconcilerTestEnv.itTestEnv.GetClient()
	ctx := context.Background()

	validConfigTask := &druidv1alpha1.EtcdOpsTask{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-task-valid-config",
			Namespace: namespace,
		},
		Spec: druidv1alpha1.EtcdOpsTaskSpec{
			Config: druidv1alpha1.EtcdOpsTaskConfig{
				OnDemandSnapshot: &druidv1alpha1.OnDemandSnapshotConfig{
					Type:               druidv1alpha1.OnDemandSnapshotTypeFull,
					TimeoutSecondsFull: ptr.To(int32(30)),
					IsFinal:            ptr.To(false),
				},
			},
			TTLSecondsAfterFinished: ptr.To(int32(60)),
			EtcdName:                ptr.To("test-etcd"),
		},
	}

	err := cl.Create(ctx, validConfigTask)
	g.Expect(err).NotTo(HaveOccurred())
	t.Logf("valid config test: created etcdopstask %s/%s successfully", validConfigTask.Namespace, validConfigTask.Name)
}

// TestEtcdOpsTaskAdmitConditions tests the admit conditions of EtcdOpsTask reconciler.
func TestEtcdOpsTaskAdmitConditions(t *testing.T) {
	tests := []struct {
		name string
		fn   func(t *testing.T, namespace string, reconcilerTestEnv ReconcilerTestEnv)
	}{
		{"should add finalizer to etcdops task", testEtcdOpsTaskAddFinalizer},
		{"Task should be rejected if the etcd's backup is disabled", testEtcdOpsTaskRejectedIfEtcdBackupDisabled},
		{"Task should be rejected if the etcd is not in ready state", testEtcdOpsTaskRejectedIfEtcdNotReady},
		{"Task should be rejected if there is a duplicate task for the same etcd", testEtcdOpsTaskRejectedDuplicateTask},
	}

	g := NewWithT(t)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			testNs := testutils.GenerateTestNamespaceName(t, testNamespacePrefix)
			t.Logf("successfully created namespace: %s to run test => '%s'", testNs, t.Name())
			g.Expect(sharedReconcilerTestEnv.itTestEnv.CreateTestNamespace(testNs)).To(Succeed())
			tc.fn(t, testNs, sharedReconcilerTestEnv)
		})
	}
}

// testEtcdOpsTaskAddFinalizer tests that the finalizer is added to the EtcdOpsTask upon creation.
func testEtcdOpsTaskAddFinalizer(t *testing.T, namespace string, reconcilerTestEnv ReconcilerTestEnv) {
	g := NewWithT(t)
	etcdopstaskInstance := newTestTask(namespace, nil)
	cl := reconcilerTestEnv.itTestEnv.GetClient()
	ctx := context.Background()
	g.Expect(cl.Create(ctx, etcdopstaskInstance)).To(Succeed())

	g.Eventually(func() bool {
		if err := cl.Get(ctx, client.ObjectKeyFromObject(etcdopstaskInstance), etcdopstaskInstance); err != nil {
			return false
		}
		return slices.Contains(etcdopstaskInstance.Finalizers, druidapiconstants.EtcdOpsTaskFinalizerName)
	}, 5*time.Second, 100*time.Millisecond).Should(BeTrue(), "finalizer should be added to etcdopstask")

	t.Logf("test completed: finalizer %s was successfully added to etcdopstask %s/%s", druidapiconstants.EtcdOpsTaskFinalizerName, etcdopstaskInstance.Namespace, etcdopstaskInstance.Name)
}

// testEtcdOpsTaskRejctedIfEtcdBackupDisabled tests that an EtcdOpsTask is rejected if the referenced etcd has backup disabled.
func testEtcdOpsTaskRejectedIfEtcdBackupDisabled(t *testing.T, namespace string, reconcilerTestEnv ReconcilerTestEnv) {
	g := NewWithT(t)

	etcdInstance := testutils.EtcdBuilderWithDefaults("test-etcd", namespace).Build()
	etcdInstance.Spec.Backup = druidv1alpha1.BackupSpec{}

	cl := reconcilerTestEnv.itTestEnv.GetClient()
	ctx := context.Background()

	g.Expect(cl.Create(ctx, etcdInstance)).To(Succeed())
	t.Logf("created etcd instance %s/%s with backup disabled", etcdInstance.Namespace, etcdInstance.Name)

	g.Expect(etcdInstance.IsBackupStoreEnabled()).To(BeFalse(), "backup should be disabled for this test")

	etcdOpsTaskInstance := newTestTask(namespace, nil)
	g.Expect(cl.Create(ctx, etcdOpsTaskInstance)).To(Succeed())
	t.Logf("created etcdopstask %s/%s", etcdOpsTaskInstance.Namespace, etcdOpsTaskInstance.Name)

	expectedErrorCode := ondemandsnapshot.ErrBackupNotEnabled
	assertEtcdOpsTaskStateAndErrorCode(ctx, g, t, cl, etcdOpsTaskInstance, druidv1alpha1.TaskStateRejected, druidv1alpha1.LastOperationStateError, druidv1alpha1.OperationPhaseAdmit, &expectedErrorCode)

	t.Logf("test completed: etcdopstask was correctly rejected due to backup being disabled")
}

// testEtcdOpsTaskRejectedIfEtcdNotReady tests that an EtcdOpsTask is rejected if the referenced etcd is not in ready state.
func testEtcdOpsTaskRejectedIfEtcdNotReady(t *testing.T, namespace string, reconcilerTestEnv ReconcilerTestEnv) {
	g := NewWithT(t)

	etcdInstance := testutils.EtcdBuilderWithDefaults("test-etcd", namespace).Build()
	etcdInstance.Status.Conditions = []druidv1alpha1.Condition{
		{
			Type:    druidv1alpha1.ConditionTypeReady,
			Status:  druidv1alpha1.ConditionFalse,
			Message: "Etcd is not ready for testing",
		},
	}

	ctx := context.Background()
	cl := reconcilerTestEnv.itTestEnv.GetClient()

	g.Expect(cl.Create(ctx, etcdInstance)).To(Succeed())
	t.Logf("created etcd instance %s/%s with not ready condition", etcdInstance.Namespace, etcdInstance.Name)

	etcdOpsTaskInstance := newTestTask(namespace, nil)
	g.Expect(cl.Create(ctx, etcdOpsTaskInstance)).To(Succeed())
	t.Logf("created etcdopstask %s/%s", etcdOpsTaskInstance.Namespace, etcdOpsTaskInstance.Name)

	expectedErrorCode := ondemandsnapshot.ErrEtcdNotReady
	assertEtcdOpsTaskStateAndErrorCode(ctx, g, t, cl, etcdOpsTaskInstance, druidv1alpha1.TaskStateRejected, druidv1alpha1.LastOperationStateError, druidv1alpha1.OperationPhaseAdmit, &expectedErrorCode)
	assertEtcdOpsTaskDeletedAfterTTL(ctx, g, t, cl, etcdOpsTaskInstance, druidv1alpha1.OperationPhaseAdmit)

	t.Logf("test completed: etcdopstask was correctly rejected due to etcd not being ready")
}

// testEtcdOpsTaskRejectedDuplicateTask tests that a duplicate EtcdOpsTask for the same etcd is rejected.
func testEtcdOpsTaskRejectedDuplicateTask(t *testing.T, namespace string, reconcilerTestEnv ReconcilerTestEnv) {
	g := NewWithT(t)
	cl := reconcilerTestEnv.itTestEnv.GetClient()
	ctx := context.Background()

	etcdInstance := testutils.EtcdBuilderWithDefaults("test-etcd", namespace).Build()
	g.Expect(cl.Create(ctx, etcdInstance)).To(Succeed())

	initialEtcdOpsTaskInstance := newTestTask(namespace, nil)
	g.Expect(cl.Create(ctx, initialEtcdOpsTaskInstance)).To(Succeed())

	duplicateEtcdOpsTaskInstance := newTestTask(namespace, nil)
	duplicateEtcdOpsTaskInstance.Name = "duplicate-test-task"
	g.Expect(cl.Create(ctx, duplicateEtcdOpsTaskInstance)).To(Succeed())
	t.Logf("created duplicate etcdopstask %s/%s", duplicateEtcdOpsTaskInstance.Namespace, duplicateEtcdOpsTaskInstance.Name)

	expectedErrorCode := etcdopstask.ErrDuplicateTask
	assertEtcdOpsTaskStateAndErrorCode(ctx, g, t, cl, duplicateEtcdOpsTaskInstance, druidv1alpha1.TaskStateRejected, druidv1alpha1.LastOperationStateError, druidv1alpha1.OperationPhaseAdmit, &expectedErrorCode)
	assertEtcdOpsTaskDeletedAfterTTL(ctx, g, t, cl, duplicateEtcdOpsTaskInstance, druidv1alpha1.OperationPhaseAdmit)

	t.Logf("test completed: duplicate etcdopstask was correctly rejected")
}

// TestEtcdOpsTaskLifecycleWithHTTPMock tests the lifecycle with HTTP client mocking.
func TestEtcdOpsTaskLifecycle(t *testing.T) {
	tests := []struct {
		name       string
		fn         func(t *testing.T, namespace string, reconcilerTestEnv ReconcilerTestEnv)
		httpClient *http.Client
	}{
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
			g.Expect(sharedReconcilerTestEnv.itTestEnv.CreateTestNamespace(testNs)).To(Succeed())

			sharedReconcilerTestEnv.InjectHTTPClient(tc.httpClient)
			tc.fn(t, testNs, sharedReconcilerTestEnv)
		})
	}
}

// testEtcdOpsTaskSuccessfulLifecycle tests the successful lifecycle with actual HTTP client mocking.
func testEtcdOpsTaskSuccessfulLifecycle(t *testing.T, namespace string, reconcilerTestEnv ReconcilerTestEnv) {
	g := NewWithT(t)

	cl := reconcilerTestEnv.itTestEnv.GetClient()
	ctx := context.Background()

	// Setup ready etcd instance
	_ = setupReadyEtcdInstance(ctx, g, t, cl, namespace)

	etcdOpsTaskInstance := newTestTask(namespace, nil)
	g.Expect(cl.Create(ctx, etcdOpsTaskInstance)).To(Succeed())

	assertEtcdOpsTaskStateAndErrorCode(ctx, g, t, cl, etcdOpsTaskInstance, druidv1alpha1.TaskStateSucceeded, druidv1alpha1.LastOperationStateSucceeded, druidv1alpha1.OperationPhaseRunning, nil)
	assertEtcdOpsTaskDeletedAfterTTL(ctx, g, t, cl, etcdOpsTaskInstance, druidv1alpha1.OperationPhaseRunning)
	t.Logf("test completed: etcdopstask successfully completed lifecycle and was deleted after TTL")
}

// testEtcdOpsTaskUnSuccessfulLifecycle tests the unsuccessful lifecycle.
func testEtcdOpsTaskUnsuccessfulLifecycle(t *testing.T, namespace string, reconcilerTestEnv ReconcilerTestEnv) {
	g := NewWithT(t)

	cl := reconcilerTestEnv.itTestEnv.GetClient()
	ctx := context.Background()

	_ = setupReadyEtcdInstance(ctx, g, t, cl, namespace)

	etcdOpsTaskInstance := newTestTask(namespace, nil)
	g.Expect(cl.Create(ctx, etcdOpsTaskInstance)).To(Succeed())

	expectedErrorCode := ondemandsnapshot.ErrCreateSnapshot
	assertEtcdOpsTaskStateAndErrorCode(ctx, g, t, cl, etcdOpsTaskInstance, druidv1alpha1.TaskStateFailed, druidv1alpha1.LastOperationStateError, druidv1alpha1.OperationPhaseRunning, &expectedErrorCode)
	assertEtcdOpsTaskDeletedAfterTTL(ctx, g, t, cl, etcdOpsTaskInstance, druidv1alpha1.OperationPhaseRunning)

	t.Logf("test completed: etcdopstask failed as expected and was deleted after TTL")
}
