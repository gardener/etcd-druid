package etcdopstask

import (
	"context"
	"slices"

	"testing"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/controller/etcdopstask"
	testutils "github.com/gardener/etcd-druid/test/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/gomega"
)

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
			EtcdRef: &druidv1alpha1.EtcdReference{
				Name:      "test-etcd",
				Namespace: namespace,
			},
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
					Type:           druidv1alpha1.OnDemandSnapshotTypeFull,
					TimeoutSeconds: ptr.To(int32(30)),
					IsFinal:        ptr.To(false),
				},
			},
			TTLSecondsAfterFinished: ptr.To(int32(60)),
			EtcdRef: &druidv1alpha1.EtcdReference{
				Name:      "test-etcd",
				Namespace: namespace,
			},
		},
	}

	err := cl.Create(ctx, validConfigTask)
	g.Expect(err).NotTo(HaveOccurred())
	t.Logf("valid config test: created etcdopstask %s/%s successfully", validConfigTask.Namespace, validConfigTask.Name)
}

// testEtcdOpsTaskAddFinalizer tests that the finalizer is added to the EtcdOpsTask upon creation.
func testEtcdOpsTaskAddFinalizer(t *testing.T, namespace string, reconcilerTestEnv ReconcilerTestEnv) {
	g := NewWithT(t)
	etcdopstaskInstance := newTestTask(namespace, nil)
	cl := reconcilerTestEnv.itTestEnv.GetClient()
	ctx := context.Background()
	g.Expect(cl.Create(ctx, etcdopstaskInstance)).To(Succeed())

	// Check if finalizer is added
	g.Eventually(func() bool {
		if err := cl.Get(ctx, client.ObjectKeyFromObject(etcdopstaskInstance), etcdopstaskInstance); err != nil {
			return false
		}
		return slices.Contains(etcdopstaskInstance.Finalizers, etcdopstask.FinalizerName)
	}, 5*time.Second, 100*time.Millisecond).Should(BeTrue(), "finalizer should be added to etcdopstask")

	t.Logf("test completed: finalizer %s was successfully added to etcdopstask %s/%s", etcdopstask.FinalizerName, etcdopstaskInstance.Namespace, etcdopstaskInstance.Name)
}

// testEtcdOpsTaskRejcEtcdBackupDisabled tests that an EtcdOpsTask is rejected if the referenced etcd has backup disabled.
func testEtcdOpsTaskRejecEtcdBackupDisabled(t *testing.T, namespace string, reconcilerTestEnv ReconcilerTestEnv) {
	g := NewWithT(t)

	// Create etcd instance with backup disabled (empty backup spec means no store configured)
	etcdInstance := testutils.EtcdBuilderWithDefaults("test-etcd", namespace).Build()
	etcdInstance.Spec.Backup = druidv1alpha1.BackupSpec{}

	cl := reconcilerTestEnv.itTestEnv.GetClient()
	ctx := context.Background()

	g.Expect(cl.Create(ctx, etcdInstance)).To(Succeed())
	t.Logf("created etcd instance %s/%s with backup disabled", etcdInstance.Namespace, etcdInstance.Name)

	g.Expect(etcdInstance.IsBackupStoreEnabled()).To(BeFalse(), "backup should be disabled for this test")

	etcdOpsTask := newTestTask(namespace, nil)
	g.Expect(cl.Create(ctx, etcdOpsTask)).To(Succeed())
	t.Logf("created etcdopstask %s/%s", etcdOpsTask.Namespace, etcdOpsTask.Name)

	assertEtcdOpsTaskRejectionAndErrorCode(ctx, g, t, cl, etcdOpsTask, druidv1alpha1.OperationPhaseAdmit, "ERR_BACKUP_NOT_ENABLED")

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

	etcdOpsTask := newTestTask(namespace, nil)
	g.Expect(cl.Create(ctx, etcdOpsTask)).To(Succeed())
	t.Logf("created etcdopstask %s/%s", etcdOpsTask.Namespace, etcdOpsTask.Name)

	assertEtcdOpsTaskRejectionAndErrorCode(ctx, g, t, cl, etcdOpsTask, druidv1alpha1.OperationPhaseAdmit, "ERR_ETCD_NOT_READY")
	assertEtcdOpsTaskDeletedAfterTTL(ctx, g, t, cl, etcdOpsTask, druidv1alpha1.OperationPhaseAdmit)

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

	assertEtcdOpsTaskRejectionAndErrorCode(ctx, g, t, cl, duplicateEtcdOpsTaskInstance, druidv1alpha1.OperationPhaseAdmit, "ERR_DUPLICATE_TASK")
	assertEtcdOpsTaskDeletedAfterTTL(ctx, g, t, cl, duplicateEtcdOpsTaskInstance, druidv1alpha1.OperationPhaseAdmit)

	t.Logf("test completed: duplicate etcdopstask was correctly rejected")
}

// testEtcdOpsTaskSuccessfulLifecycle tests the successful lifecycle of an EtcdOpsTask.
func testEtcdOpsTaskSuccessfulLifecycle(t *testing.T, namespace string, reconcilerTestEnv ReconcilerTestEnv) {
	g := NewWithT(t)
	cl := reconcilerTestEnv.itTestEnv.GetClient()
	ctx := context.Background()

	// Setup ready etcd instance
	_ = setupReadyEtcdInstance(ctx, g, t, cl, namespace)

	// Create EtcdOpsTask and wait for it to be in progress
	etcdOpsTaskInstance := createEtcdOpsTaskAndWaitForInProgress(ctx, g, t, cl, namespace)

	// Wait for task to reach Running phase
	waitForTaskToReachRunningPhase(ctx, g, cl, etcdOpsTaskInstance)

	t.Logf("manually marking task as succeeded to simulate successful snapshot creation")

	// Manually set task as succeeded to simulate successful HTTP call.
	etcdOpsTaskInstance.Status.State = &[]druidv1alpha1.TaskState{druidv1alpha1.TaskStateSucceeded}[0]
	etcdOpsTaskInstance.Status.LastOperation = &druidv1alpha1.EtcdOpsLastOperation{
		Phase:              druidv1alpha1.OperationPhaseRunning,
		State:              druidv1alpha1.OperationStateCompleted,
		Description:        "Snapshot created successfully",
		LastTransitionTime: &metav1.Time{Time: metav1.Now().Time},
	}
	etcdOpsTaskInstance.Status.LastErrors = nil

	g.Expect(cl.Status().Update(ctx, etcdOpsTaskInstance)).To(Succeed())
	t.Logf("updated task status to succeeded")

	// Verify the controller processes the successful state correctly
	g.Eventually(func() bool {
		if err := cl.Get(ctx, client.ObjectKeyFromObject(etcdOpsTaskInstance), etcdOpsTaskInstance); err != nil {
			t.Logf("failed to get etcdopstask: %v", err)
			return false
		}
		// Verify task remains in succeeded state
		if etcdOpsTaskInstance.Status.State != nil && *etcdOpsTaskInstance.Status.State == druidv1alpha1.TaskStateSucceeded {
			// Verify the last operation shows success
			if etcdOpsTaskInstance.Status.LastOperation != nil &&
				etcdOpsTaskInstance.Status.LastOperation.State == druidv1alpha1.OperationStateCompleted {
				t.Logf("task successfully completed with description: %s",
					etcdOpsTaskInstance.Status.LastOperation.Description)
				return true
			}
		}
		return false
	}, 10*time.Second, 1*time.Second).Should(BeTrue(), "task should maintain succeeded state")

	assertEtcdOpsTaskDeletedAfterTTL(ctx, g, t, cl, etcdOpsTaskInstance, druidv1alpha1.OperationPhaseRunning)
	t.Logf("test completed: etcdopstask successfully completed lifecycle and was deleted after TTL")
}

// testEtcdOpsTaskUnSuccessfulLifecycle tests the unsuccessful lifecycle of an EtcdOpsTas i.e where the run method fails.
func testEtcdOpsTaskUnSuccessfulLifecycle(t *testing.T, namespace string, reconcilerTestEnv ReconcilerTestEnv) {
	g := NewWithT(t)
	cl := reconcilerTestEnv.itTestEnv.GetClient()
	ctx := context.Background()

	// Setup ready etcd instance
	_ = setupReadyEtcdInstance(ctx, g, t, cl, namespace)

	// Create EtcdOpsTask and wait for it to be in progress
	etcdOpsTaskInstance := createEtcdOpsTaskAndWaitForInProgress(ctx, g, t, cl, namespace)

	// Wait for task to reach Running phase
	waitForTaskToReachRunningPhase(ctx, g, cl, etcdOpsTaskInstance)

	t.Logf("manually marking task as failed to simulate snapshot creation failure (HTTP 500 error)")

	// Simulate ErrCreateSnapshot scenario: Completed=true, Error=not nil
	etcdOpsTaskInstance.Status.State = &[]druidv1alpha1.TaskState{druidv1alpha1.TaskStateFailed}[0]
	etcdOpsTaskInstance.Status.LastOperation = &druidv1alpha1.EtcdOpsLastOperation{
		Phase:              druidv1alpha1.OperationPhaseRunning,
		State:              druidv1alpha1.OperationStateFailed,
		Description:        "Run Operation: Failed to create snapshot",
		LastTransitionTime: &metav1.Time{Time: metav1.Now().Time},
	}
	etcdOpsTaskInstance.Status.LastErrors = []druidv1alpha1.EtcdOpsTaskLastError{
		{
			Code:        "ERR_CREATE_SNAPSHOT",
			Description: "failed to create snapshot, status code: 500",
			ObservedAt:  metav1.Now(),
		},
	}

	g.Expect(cl.Status().Update(ctx, etcdOpsTaskInstance)).To(Succeed())
	t.Logf("updated task status to failed with ERR_CREATE_SNAPSHOT")

	// Verify the controller processes the failed state correctly
	g.Eventually(func() bool {
		if err := cl.Get(ctx, client.ObjectKeyFromObject(etcdOpsTaskInstance), etcdOpsTaskInstance); err != nil {
			return false
		}

		// Verify task remains in failed state
		if etcdOpsTaskInstance.Status.State != nil && *etcdOpsTaskInstance.Status.State == druidv1alpha1.TaskStateFailed {
			// Verify the last operation shows failure
			if etcdOpsTaskInstance.Status.LastOperation != nil &&
				etcdOpsTaskInstance.Status.LastOperation.State == druidv1alpha1.OperationStateFailed &&
				len(etcdOpsTaskInstance.Status.LastErrors) > 0 {
				lastError := etcdOpsTaskInstance.Status.LastErrors[0]
				if lastError.Code == "ERR_CREATE_SNAPSHOT" {
					return true
				}
			}
		}
		return false
	}, 10*time.Second, 1*time.Second).Should(BeTrue(), "task should maintain failed state with ERR_CREATE_SNAPSHOT")

	assertEtcdOpsTaskDeletedAfterTTL(ctx, g, t, cl, etcdOpsTaskInstance, druidv1alpha1.OperationPhaseRunning)
	t.Logf("test completed: etcdopstask failed as expected and was deleted after TTL")
}
