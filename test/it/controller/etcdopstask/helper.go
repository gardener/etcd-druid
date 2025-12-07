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
	ItTestEnv  setup.DruidTestEnvironment
	Reconciler *etcdopstask.Reconciler
}

// InjectHTTPClient injects an HTTP client into the reconciler's task handler registry for testing.
func (env *ReconcilerTestEnv) InjectHTTPClient(httpClient *http.Client) {
	registry := env.Reconciler.GetTaskHandlerRegistry()
	registry.Register("OnDemandSnapshot", func(k8sclient client.Client, task *druidv1alpha1.EtcdOpsTask, _ *http.Client) (handler.Handler, error) {
		return ondemandsnapshot.New(k8sclient, task, httpClient)
	})
}

// initializeEtcdOpsTaskReconcilerTestEnv sets up the test environment
func InitializeEtcdOpsTaskReconcilerTestEnv(t *testing.T, itTestEnv setup.DruidTestEnvironment, clientBuilder *testutils.TestClientBuilder) ReconcilerTestEnv {
	g := NewWithT(&testing.T{})
	var (
		reconciler *etcdopstask.Reconciler
	)

	g.Expect(itTestEnv.CreateManager(clientBuilder)).To(Succeed())

	itTestEnv.RegisterReconciler(func(mgr manager.Manager) {
		reconciler = etcdopstask.NewReconciler(mgr, &druidconfigv1alpha1.EtcdOpsTaskControllerConfiguration{
			ConcurrentSyncs: ptr.To(3),
			RequeueInterval: &metav1.Duration{Duration: 5 * time.Second},
		})
		g.Expect(reconciler.RegisterWithManager(mgr)).To(Succeed())
	})

	g.Expect(itTestEnv.StartManager()).To(Succeed())
	if t != nil {
		t.Log("successfully registered etcdopstask reconciler with manager and started manager")
	}

	return ReconcilerTestEnv{
		ItTestEnv:  itTestEnv,
		Reconciler: reconciler,
	}
}

// deployReadyEtcd creates an etcd instance and sets it to ready status
func DeployReadyEtcd(ctx context.Context, g *WithT, t *testing.T, cl client.Client, namespace string) *druidv1alpha1.Etcd {
	etcdInstance := testutils.EtcdBuilderWithDefaults("test-etcd", namespace).Build()

	g.Expect(cl.Create(ctx, etcdInstance)).To(Succeed())
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

	g.Eventually(func() bool {
		if err := cl.Get(ctx, client.ObjectKeyFromObject(etcdInstance), etcdInstance); err != nil {
			return false
		}
		return etcdInstance.IsReady()
	}, 5*time.Second, 100*time.Millisecond).Should(BeTrue())

	t.Logf("etcd instance is now ready")
	return etcdInstance
}
