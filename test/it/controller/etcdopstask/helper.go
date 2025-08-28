package etcdopstask

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/gardener/etcd-druid/api/config/v1alpha1"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/controller/etcdopstask"
	"github.com/gardener/etcd-druid/internal/task/ondemandsnapshot"
	"github.com/gardener/etcd-druid/test/it/setup"
	testutils "github.com/gardener/etcd-druid/test/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	. "github.com/onsi/gomega"
)

// ReconcilerTestEnv encapsulates the test environment required to test the EtcdOpsTask reconciler.
type ReconcilerTestEnv struct {
	itTestEnv  setup.DruidTestEnvironment
	reconciler *etcdopstask.Reconciler
}

// initializeEtcdOpsTaskReconcilerTestEnv sets up the test environment
func initializeEtcdOpsTaskReconcilerTestEnv(t *testing.T, itTestEnv setup.DruidTestEnvironment, clientBuilder *testutils.TestClientBuilder) ReconcilerTestEnv {
	var g *WithT
	if t != nil {
		g = NewWithT(t)
	} else {
		g = NewWithT(&testing.T{})
	}
	var (
		reconciler *etcdopstask.Reconciler
	)

	g.Expect(itTestEnv.CreateManager(clientBuilder)).To(Succeed())

	itTestEnv.RegisterReconciler(func(mgr manager.Manager) {
		reconciler = etcdopstask.New(mgr, &v1alpha1.EtcdOpsTaskControllerConfiguration{
			ConcurrentSyncs: ptr.To(3),
			RequeueInterval: metav1.Duration{Duration: 15 * time.Second},
		})
		g.Expect(reconciler.RegisterWithManager(mgr)).To(Succeed())
	})

	g.Expect(itTestEnv.StartManager()).To(Succeed())
	if t != nil {
		t.Log("successfully registered etcdopstask reconciler with manager and started manager")
	}

	return ReconcilerTestEnv{
		itTestEnv:  itTestEnv,
		reconciler: reconciler,
	}
}

// newTestTask creates a new EtcdOpsTask instance.
func newTestTask(namespace string, state *druidv1alpha1.TaskState) *druidv1alpha1.EtcdOpsTask {
	ts := &druidv1alpha1.EtcdOpsTask{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-task",
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
			TTLSecondsAfterFinished: ptr.To(int32(20)),
			EtcdRef: &druidv1alpha1.EtcdReference{
				Name:      "test-etcd",
				Namespace: namespace,
			},
		},
		Status: druidv1alpha1.EtcdOpsTaskStatus{},
	}
	if state != nil {
		ts.Status.State = state
	}
	return ts
}

// setupReadyEtcdInstance creates an etcd instance and sets it to ready status
func setupReadyEtcdInstance(ctx context.Context, g *WithT, t *testing.T, cl client.Client, namespace string) *druidv1alpha1.Etcd {
	etcdInstance := testutils.EtcdBuilderWithDefaults("test-etcd", namespace).Build()

	g.Expect(cl.Create(ctx, etcdInstance)).To(Succeed())
	// Set etcd as ready
	etcdInstance.Status.Conditions = []druidv1alpha1.Condition{
		{
			Type:               druidv1alpha1.ConditionTypeReady,
			Status:             druidv1alpha1.ConditionTrue,
			Message:            "Etcd is ready for operations",
			LastTransitionTime: metav1.Now(),
			LastUpdateTime:     metav1.Now(),
		},
	}
	g.Expect(cl.Status().Update(ctx, etcdInstance)).To(Succeed())
	t.Logf("updated etcd instance status to ready")

	// Verify etcd readiness
	g.Eventually(func() bool {
		if err := cl.Get(ctx, client.ObjectKeyFromObject(etcdInstance), etcdInstance); err != nil {
			return false
		}
		return ondemandsnapshot.CheckEtcdReadiness(ctx, etcdInstance) == nil
	}, 5*time.Second, 100*time.Millisecond).Should(BeTrue())

	t.Logf("etcd instance is now ready")
	return etcdInstance
}

// createEtcdOpsTaskAndWaitForInProgress creates an EtcdOpsTask and waits for it to be admitted and transition to InProgress
func createEtcdOpsTaskAndWaitForInProgress(ctx context.Context, g *WithT, t *testing.T, cl client.Client, namespace string) *druidv1alpha1.EtcdOpsTask {
	etcdOpsTaskInstance := newTestTask(namespace, nil)
	g.Expect(cl.Create(ctx, etcdOpsTaskInstance)).To(Succeed())

	// Wait for task to be admitted and transition to InProgress
	g.Eventually(func() bool {
		if err := cl.Get(ctx, client.ObjectKeyFromObject(etcdOpsTaskInstance), etcdOpsTaskInstance); err != nil {
			return false
		}

		if etcdOpsTaskInstance.Status.State != nil && *etcdOpsTaskInstance.Status.State == druidv1alpha1.TaskStateInProgress {
			return true
		}
		return false
	}, 10*time.Second, 1*time.Second).Should(BeTrue(), "task should be admitted and transition to InProgress")

	return etcdOpsTaskInstance
}

// waitForTaskToReachRunningPhase waits for the task to reach the Running phase
func waitForTaskToReachRunningPhase(ctx context.Context, g *WithT, cl client.Client, etcdOpsTaskInstance *druidv1alpha1.EtcdOpsTask) {
	g.Eventually(func() bool {
		if err := cl.Get(ctx, client.ObjectKeyFromObject(etcdOpsTaskInstance), etcdOpsTaskInstance); err != nil {
			return false
		}

		if etcdOpsTaskInstance.Status.LastOperation != nil {
			return etcdOpsTaskInstance.Status.LastOperation.Phase == druidv1alpha1.OperationPhaseRunning
		}
		return false
	}, 10*time.Second, 1*time.Second).Should(BeTrue(), "task should reach Running phase")
}

// assertEtcdOpsTaskRejectionAndErrorCode waits for the EtcdOpsTask to be rejected with the specified error code in the given phase.
func assertEtcdOpsTaskRejectionAndErrorCode(ctx context.Context, g *WithT, t *testing.T, cl client.Client, etcdOpsTask *druidv1alpha1.EtcdOpsTask, expectedPhase druidv1alpha1.OperationPhase, expectedErrorCode druidv1alpha1.ErrorCode) {
	g.Eventually(func() bool {
		if err := cl.Get(ctx, client.ObjectKeyFromObject(etcdOpsTask), etcdOpsTask); err != nil {
			return false
		}

		// Check if task state is rejected
		if etcdOpsTask.Status.State != nil && *etcdOpsTask.Status.State == druidv1alpha1.TaskStateRejected {
			// Check if the last operation indicates failure in the expected phase
			if etcdOpsTask.Status.LastOperation != nil &&
				etcdOpsTask.Status.LastOperation.Phase == expectedPhase &&
				etcdOpsTask.Status.LastOperation.State == druidv1alpha1.OperationStateFailed {

				// Check if the error matches the expected error code
				if len(etcdOpsTask.Status.LastErrors) > 0 {
					for _, lastError := range etcdOpsTask.Status.LastErrors {
						if lastError.Code == expectedErrorCode {
							t.Logf("task correctly rejected in phase %s with error code %s: %s", expectedPhase, expectedErrorCode, lastError.Description)
							return true
						}
					}
				}
			}
		}
		return false
	}, 10*time.Second, 1*time.Second).Should(BeTrue(), fmt.Sprintf("task should be rejected with error code %s", expectedErrorCode))
}

// assertEtcdOpsTaskDeletedAfterTTL waits for the EtcdOpsTask to be deleted after TTL expires for tasks that reached the specified phase.
func assertEtcdOpsTaskDeletedAfterTTL(ctx context.Context, g *WithT, t *testing.T, cl client.Client, etcdOpsTask *druidv1alpha1.EtcdOpsTask, expectedFinalPhase druidv1alpha1.OperationPhase) {
	t.Logf("waiting for task to reach final phase: %s", expectedFinalPhase)
	err := cl.Get(ctx, client.ObjectKeyFromObject(etcdOpsTask), etcdOpsTask)
	g.Expect(err).NotTo(HaveOccurred())

	// Calculate timeout based on TTL plus some buffer time for processing
	ttlSeconds := int32(10)
	if etcdOpsTask.Spec.TTLSecondsAfterFinished != nil {
		ttlSeconds = *etcdOpsTask.Spec.TTLSecondsAfterFinished
	}

	// Add buffer time for processing delays
	timeoutDuration := time.Duration(ttlSeconds+10) * time.Second

	g.Eventually(func() bool {
		err := cl.Get(ctx, client.ObjectKeyFromObject(etcdOpsTask), etcdOpsTask)
		if err != nil {
			if client.IgnoreNotFound(err) == nil {
				t.Logf("task %s/%s was successfully deleted after TTL expiry", etcdOpsTask.Namespace, etcdOpsTask.Name)
				return true
			}
			t.Logf("error checking task deletion: %v", err)
			return false
		}
		return false
	}, timeoutDuration, 2*time.Second).Should(BeTrue(), "task should be deleted after TTL expires")
}
