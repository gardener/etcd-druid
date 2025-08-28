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

	expectedErrorCode := druidv1alpha1.ErrorCode("ERR_BACKUP_NOT_ENABLED")
	assertEtcdOpsTaskStateAndErrorCode(ctx, g, t, cl, etcdOpsTask, druidv1alpha1.TaskStateRejected, druidv1alpha1.OperationStateFailed, druidv1alpha1.OperationPhaseAdmit, &expectedErrorCode)

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

	expectedErrorCode := druidv1alpha1.ErrorCode("ERR_ETCD_NOT_READY")
	assertEtcdOpsTaskStateAndErrorCode(ctx, g, t, cl, etcdOpsTask, druidv1alpha1.TaskStateRejected, druidv1alpha1.OperationStateFailed, druidv1alpha1.OperationPhaseAdmit, &expectedErrorCode)
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

	expectedErrorCode := druidv1alpha1.ErrorCode("ERR_DUPLICATE_TASK")
	assertEtcdOpsTaskStateAndErrorCode(ctx, g, t, cl, duplicateEtcdOpsTaskInstance, druidv1alpha1.TaskStateRejected, druidv1alpha1.OperationStateFailed, druidv1alpha1.OperationPhaseAdmit, &expectedErrorCode)
	assertEtcdOpsTaskDeletedAfterTTL(ctx, g, t, cl, duplicateEtcdOpsTaskInstance, druidv1alpha1.OperationPhaseAdmit)

	t.Logf("test completed: duplicate etcdopstask was correctly rejected")
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

	assertEtcdOpsTaskStateAndErrorCode(ctx, g, t, cl, etcdOpsTaskInstance, druidv1alpha1.TaskStateSucceeded, druidv1alpha1.OperationStateCompleted, druidv1alpha1.OperationPhaseRunning, nil)
	assertEtcdOpsTaskDeletedAfterTTL(ctx, g, t, cl, etcdOpsTaskInstance, druidv1alpha1.OperationPhaseRunning)
	t.Logf("test completed: etcdopstask successfully completed lifecycle and was deleted after TTL")
}

// testEtcdOpsTaskUnSuccessfulLifecycle tests the unsuccessful lifecycle.
func testEtcdOpsTaskUnSuccessfulLifecycle(t *testing.T, namespace string, reconcilerTestEnv ReconcilerTestEnv) {
	g := NewWithT(t)

	cl := reconcilerTestEnv.itTestEnv.GetClient()
	ctx := context.Background()

	_ = setupReadyEtcdInstance(ctx, g, t, cl, namespace)

	etcdOpsTaskInstance := newTestTask(namespace, nil)
	g.Expect(cl.Create(ctx, etcdOpsTaskInstance)).To(Succeed())

	expectedErrorCode := druidv1alpha1.ErrorCode("ERR_CREATE_SNAPSHOT")
	assertEtcdOpsTaskStateAndErrorCode(ctx, g, t, cl, etcdOpsTaskInstance, druidv1alpha1.TaskStateFailed, druidv1alpha1.OperationStateFailed, druidv1alpha1.OperationPhaseRunning, &expectedErrorCode)
	assertEtcdOpsTaskDeletedAfterTTL(ctx, g, t, cl, etcdOpsTaskInstance, druidv1alpha1.OperationPhaseRunning)

	t.Logf("test completed: etcdopstask failed as expected and was deleted after TTL")
}
