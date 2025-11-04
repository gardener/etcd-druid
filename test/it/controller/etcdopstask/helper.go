// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package etcdopstask

import (
	"context"
	"net/http"
	"testing"
	"time"

	druidconfigv1alpha1 "github.com/gardener/etcd-druid/api/config/v1alpha1"
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
		reconciler = etcdopstask.NewReconciler(mgr, &druidconfigv1alpha1.EtcdOpsTaskControllerConfiguration{
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
					TimeoutSecondsFull: ptr.To(int32(150)),
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
