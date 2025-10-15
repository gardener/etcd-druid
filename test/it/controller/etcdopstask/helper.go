// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package etcdopstask

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/gardener/etcd-druid/api/config/v1alpha1"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/controller/etcdopstask"
	"github.com/gardener/etcd-druid/internal/controller/etcdopstask/handler"
	"github.com/gardener/etcd-druid/internal/controller/etcdopstask/handler/ondemandsnapshot"
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

// InjectHTTPClient injects an HTTP client into the reconciler's task handler registry for testing.
func (env *ReconcilerTestEnv) InjectHTTPClient(httpClient *http.Client) {
	registry := env.reconciler.GetTaskHandlerRegistry()
	registry.Register("OnDemandSnapshot", func(k8sclient client.Client, task *druidv1alpha1.EtcdOpsTask, _ *http.Client) (handler.Handler, error) {
		return ondemandsnapshot.New(k8sclient, task, httpClient)
	})
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
		reconciler = etcdopstask.NewReconciler(mgr, &v1alpha1.EtcdOpsTaskControllerConfiguration{
			ConcurrentSyncs: ptr.To(3),
			RequeueInterval: &metav1.Duration{Duration: 15 * time.Second},
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
					Type:               druidv1alpha1.OnDemandSnapshotTypeFull,
					TimeoutSecondsFull: ptr.To(int32(30)),
					IsFinal:            ptr.To(false),
				},
			},
			TTLSecondsAfterFinished: ptr.To(int32(20)),
			EtcdName:                ptr.To("test-etcd"),
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
		return etcdInstance.IsReady() == nil
	}, 5*time.Second, 100*time.Millisecond).Should(BeTrue())

	t.Logf("etcd instance is now ready")
	return etcdInstance
}

// assertEtcdOpsTaskStateAndErrorCode waits for the EtcdOpsTask to reach the specified state with the expected error code (or nil for success) in the given phase.
func assertEtcdOpsTaskStateAndErrorCode(ctx context.Context, g *WithT, t *testing.T, cl client.Client, etcdOpsTask *druidv1alpha1.EtcdOpsTask, expectedTaskState druidv1alpha1.TaskState, expectedOperationState druidv1alpha1.LastOperationState, expectedPhase druidv1alpha1.OperationPhase, expectedErrorCode *druidv1alpha1.ErrorCode) {
	g.Eventually(func() bool {
		if err := cl.Get(ctx, client.ObjectKeyFromObject(etcdOpsTask), etcdOpsTask); err != nil {
			return false
		}

		// Check if task state matches
		if etcdOpsTask.Status.State == nil || *etcdOpsTask.Status.State != expectedTaskState {
			return false
		}

		// Check if LastOperation exists and matches expected state
		if etcdOpsTask.Status.LastOperation == nil || etcdOpsTask.Status.LastOperation.State != expectedOperationState {
			return false
		}

		// Check if Phase exists and matches expected phase
		if etcdOpsTask.Status.Phase == nil || *etcdOpsTask.Status.Phase != expectedPhase {
			return false
		}

		// All basic checks passed, now check for error conditions
		if expectedErrorCode != nil {
			// We expect an error code, so check LastErrors
			if len(etcdOpsTask.Status.LastErrors) > 0 {
				for _, lastError := range etcdOpsTask.Status.LastErrors {
					if lastError.Code == *expectedErrorCode {
						t.Logf("task correctly reached state %s in phase %s with operation state %s and error code %s: %s", expectedTaskState, expectedPhase, expectedOperationState, *expectedErrorCode, lastError.Description)
						return true
					}
				}
			}
			return false
		} else {
			// No error expected, success case
			t.Logf("task correctly reached state %s in phase %s with operation state %s: %s", expectedTaskState, expectedPhase, expectedOperationState, etcdOpsTask.Status.LastOperation.Description)
			return true
		}
	}, 15*time.Second, 1*time.Second).Should(BeTrue(), func() string {
		if expectedErrorCode != nil {
			return fmt.Sprintf("task should reach state %s with error code %s", expectedTaskState, *expectedErrorCode)
		}
		return fmt.Sprintf("task should reach state %s successfully", expectedTaskState)
	}())
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
