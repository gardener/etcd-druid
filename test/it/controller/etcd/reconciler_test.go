package etcd

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/controller/etcd"
	"github.com/gardener/etcd-druid/internal/features"
	"github.com/gardener/etcd-druid/internal/operator"
	"github.com/gardener/etcd-druid/test/it/controller/assets"
	"github.com/gardener/etcd-druid/test/it/setup"
	testutils "github.com/gardener/etcd-druid/test/utils"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	. "github.com/onsi/gomega"
	"k8s.io/component-base/featuregate"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const testNamespacePrefix = "etcd-reconciler-test-"

/*
	status update check:
		* create etcd resource and let sts be created
		* assert the etcd status
		* update sts status and then assert etcd status again

	deletion flow
	spec reconcile flow

*/

func TestEtcdReconcilerSpecWithoutAutoReconcile(t *testing.T) {
	reconcilerTestEnv := initializeEtcdReconcilerTestEnv(t, false)
	defer reconcilerTestEnv.itTestEnvCloser()

	tests := []struct {
		name string
		fn   func(t *testing.T, testNamespace string, reconcilerTestEnv ReconcilerTestEnv)
	}{
		//{"should create all managed resources when etcd resource is created", testAllManagedResourcesAreCreated},
		{"should succeed only in creation of some resources and not all and should record error in lastErrors and lastOperation", testFailureToCreateAllResources},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testNs := createTestNamespaceName(t)
			t.Logf("successfully create namespace: %s to run test => '%s'", testNs, t.Name())
			reconcilerTestEnv.itTestEnv.CreateTestNamespace(testNs)
			test.fn(t, testNs, reconcilerTestEnv)
		})
	}
}

func testAllManagedResourcesAreCreated(t *testing.T, testNs string, reconcilerTestEnv ReconcilerTestEnv) {
	const (
		timeout         = time.Minute * 2
		pollingInterval = time.Second * 2
	)
	// ***************** setup *****************
	g := NewWithT(t)
	etcdInstance := testutils.EtcdBuilderWithDefaults(testutils.TestEtcdName, testNs).
		WithClientTLS().
		WithPeerTLS().
		WithReplicas(3).
		WithAnnotations(map[string]string{v1beta1constants.GardenerOperation: v1beta1constants.GardenerOperationReconcile}).
		Build()
	g.Expect(etcdInstance.Spec.Backup.Store).ToNot(BeNil())
	g.Expect(etcdInstance.Spec.Backup.Store.SecretRef).ToNot(BeNil())
	cl := reconcilerTestEnv.itTestEnv.GetClient()
	ctx := context.Background()
	// create backup secrets
	g.Expect(testutils.CreateSecrets(ctx, cl, testNs, etcdInstance.Spec.Backup.Store.SecretRef.Name)).To(Succeed())
	// create etcdInstance resource
	g.Expect(cl.Create(ctx, etcdInstance)).To(Succeed())
	// ***************** test etcd spec reconciliation  *****************
	// It is sufficient to test that the resources are created as part of the sync. The configuration of each
	// resource is now extensively covered in the unit tests for the respective component operator.
	assertAllComponentsCreatedSuccessfully(ctx, t, reconcilerTestEnv, etcdInstance, timeout, pollingInterval)
	assertETCDObservedGeneration(t, reconcilerTestEnv.itTestEnv.GetClient(), client.ObjectKeyFromObject(etcdInstance), pointer.Int64(1), 5*time.Second, 1*time.Second)
	expectedLastOperation := druidv1alpha1.LastOperation{
		Type:  druidv1alpha1.LastOperationTypeReconcile,
		State: druidv1alpha1.LastOperationStateSucceeded,
	}
	assertETCDLastOperationAndLastErrorsUpdatedSuccessfully(t, reconcilerTestEnv.itTestEnv.GetClient(), client.ObjectKeyFromObject(etcdInstance), expectedLastOperation, nil, 5*time.Second, 1*time.Second)
	assertETCDOperationAnnotationRemovedSuccessfully(t, reconcilerTestEnv.itTestEnv.GetClient(), client.ObjectKeyFromObject(etcdInstance), 5*time.Second, 1*time.Second)
}

func testFailureToCreateAllResources(t *testing.T, testNs string, reconcilerTestEnv ReconcilerTestEnv) {
	const (
		timeout         = time.Minute * 3
		pollingInterval = time.Second * 2
	)
	// ***************** setup *****************
	g := NewWithT(t)
	etcdInstance := testutils.EtcdBuilderWithoutDefaults(testutils.TestEtcdName, testNs).
		WithClientTLS().
		WithPeerTLS().
		WithReplicas(3).
		WithAnnotations(map[string]string{v1beta1constants.GardenerOperation: v1beta1constants.GardenerOperationReconcile}).
		// The client service label value has an invalid value. ':' are not accepted as valid character in label values.
		// This should fail creation of client service and cause a requeue.
		WithEtcdClientServiceLabels(map[string]string{"invalid-label": "invalid-label:value"}).
		Build()

	ctx := context.Background()
	cl := reconcilerTestEnv.itTestEnv.GetClient()
	// create etcdInstance resource
	g.Expect(cl.Create(ctx, etcdInstance)).To(Succeed())
	// ***************** test etcd spec reconciliation  *****************
	componentKindCreated := []operator.Kind{operator.MemberLeaseKind}
	assertSelectedComponentsCreatedSuccessfully(ctx, t, reconcilerTestEnv, etcdInstance, componentKindCreated, timeout, pollingInterval)
	componentKindNotCreated := []operator.Kind{
		operator.SnapshotLeaseKind, // no backup store has been set
		operator.ClientServiceKind,
		operator.PeerServiceKind,
		operator.ConfigMapKind,
		operator.PodDisruptionBudgetKind,
		operator.ServiceAccountKind,
		operator.RoleKind,
		operator.RoleBindingKind,
		operator.StatefulSetKind,
	}
	assertComponentsNotCreatedSuccessfully(ctx, t, reconcilerTestEnv, etcdInstance, componentKindNotCreated, timeout, pollingInterval)
	assertETCDObservedGeneration(t, reconcilerTestEnv.itTestEnv.GetClient(), client.ObjectKeyFromObject(etcdInstance), nil, 5*time.Second, 1*time.Second)
	expectedLastOperation := druidv1alpha1.LastOperation{
		Type:  druidv1alpha1.LastOperationTypeReconcile,
		State: druidv1alpha1.LastOperationStateError,
	}
	expectedLastErrs := []druidv1alpha1.LastError{
		{
			Code: "ERR_SYNC_CLIENT_SERVICE",
		},
	}
	assertETCDLastOperationAndLastErrorsUpdatedSuccessfully(t, cl, client.ObjectKeyFromObject(etcdInstance), expectedLastOperation, expectedLastErrs, 5*time.Second, 1*time.Second)
}

func createTestNamespaceName(t *testing.T) string {
	b := make([]byte, 4)
	_, err := rand.Read(b)
	g := NewWithT(t)
	g.Expect(err).ToNot(HaveOccurred())
	namespaceSuffix := hex.EncodeToString(b)
	return fmt.Sprintf("%s-%s", testNamespacePrefix, namespaceSuffix)
}

func initializeEtcdReconcilerTestEnv(t *testing.T, autoReconcile bool) ReconcilerTestEnv {
	g := NewWithT(t)
	itTestEnv, itTestEnvCloser := setup.NewIntegrationTestEnv(t, "etcd-reconciler", []string{assets.GetEtcdCrdPath()})
	var (
		reconciler *etcd.Reconciler
		err        error
	)
	itTestEnv.RegisterReconciler(func(mgr manager.Manager) {
		reconciler, err = etcd.NewReconcilerWithImageVector(mgr,
			&etcd.Config{
				Workers:                            5,
				EnableEtcdSpecAutoReconcile:        autoReconcile,
				DisableEtcdServiceAccountAutomount: false,
				EtcdStatusSyncPeriod:               2 * time.Second,
				FeatureGates: map[featuregate.Feature]bool{
					features.UseEtcdWrapper: true,
				},
			}, assets.CreateImageVector(g))
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(reconciler.RegisterWithManager(mgr)).To(Succeed())
	})
	itTestEnv.StartManager()
	t.Log("successfully registered etcd reconciler with manager and started manager")
	return ReconcilerTestEnv{
		itTestEnv:       itTestEnv,
		itTestEnvCloser: itTestEnvCloser,
		reconciler:      reconciler,
	}
}
