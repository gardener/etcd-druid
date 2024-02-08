package etcd

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"
	"github.com/gardener/etcd-druid/internal/controller/etcd"
	"github.com/gardener/etcd-druid/internal/features"
	"github.com/gardener/etcd-druid/internal/operator"
	"github.com/gardener/etcd-druid/test/it/controller/assets"
	"github.com/gardener/etcd-druid/test/it/setup"
	testutils "github.com/gardener/etcd-druid/test/utils"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	"github.com/gardener/gardener/pkg/controllerutils"
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

	deletion flow -done
	spec reconcile flow -done
*/

// ------------------------------ reconcile spec tests ------------------------------
func TestEtcdReconcilerSpecWithoutAutoReconcile(t *testing.T) {
	itTestEnv, itTestEnvCloser := setup.NewIntegrationTestEnv(t, "etcd-reconciler", []string{assets.GetEtcdCrdPath()})
	defer itTestEnvCloser(t)
	reconcilerTestEnv := initializeEtcdReconcilerTestEnv(t, itTestEnv, false)

	tests := []struct {
		name string
		fn   func(t *testing.T, testNamespace string, reconcilerTestEnv ReconcilerTestEnv)
	}{
		{"should create all managed resources when etcd resource is created", testAllManagedResourcesAreCreated},
		{"should succeed only in creation of some resources and not all and should record error in lastErrors and lastOperation", testFailureToCreateAllResources},
		{"should not reconcile spec when reconciliation is suspended", testWhenReconciliationIsSuspended},
		{"should not reconcile when no reconcile operation annotation is set", testWhenNoReconcileOperationAnnotationIsSet},
	}

	t.Parallel()
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
	assertAllComponentsExists(ctx, t, reconcilerTestEnv, etcdInstance, timeout, pollingInterval)
	assertETCDObservedGeneration(t, reconcilerTestEnv.itTestEnv.GetClient(), client.ObjectKeyFromObject(etcdInstance), pointer.Int64(1), 5*time.Second, 1*time.Second)
	expectedLastOperation := &druidv1alpha1.LastOperation{
		Type:  druidv1alpha1.LastOperationTypeReconcile,
		State: druidv1alpha1.LastOperationStateSucceeded,
	}
	assertETCDLastOperationAndLastErrorsUpdatedSuccessfully(t, reconcilerTestEnv.itTestEnv.GetClient(), client.ObjectKeyFromObject(etcdInstance), expectedLastOperation, nil, 5*time.Second, 1*time.Second)
	assertETCDOperationAnnotation(t, reconcilerTestEnv.itTestEnv.GetClient(), client.ObjectKeyFromObject(etcdInstance), false, 5*time.Second, 1*time.Second)
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
	assertSelectedComponentsExists(ctx, t, reconcilerTestEnv, etcdInstance, componentKindCreated, timeout, pollingInterval)
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
	assertComponentsDoNotExist(ctx, t, reconcilerTestEnv, etcdInstance, componentKindNotCreated, timeout, pollingInterval)
	assertETCDObservedGeneration(t, reconcilerTestEnv.itTestEnv.GetClient(), client.ObjectKeyFromObject(etcdInstance), nil, 5*time.Second, 1*time.Second)
	expectedLastOperation := &druidv1alpha1.LastOperation{
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

func testWhenReconciliationIsSuspended(t *testing.T, testNs string, reconcilerTestEnv ReconcilerTestEnv) {
	// ***************** setup *****************
	g := NewWithT(t)
	etcdInstance := testutils.EtcdBuilderWithoutDefaults(testutils.TestEtcdName, testNs).
		WithClientTLS().
		WithPeerTLS().
		WithReplicas(3).
		WithAnnotations(map[string]string{
			v1beta1constants.GardenerOperation:               v1beta1constants.GardenerOperationReconcile,
			druidv1alpha1.SuspendEtcdSpecReconcileAnnotation: "true",
		}).Build()
	ctx := context.Background()
	cl := reconcilerTestEnv.itTestEnv.GetClient()
	// create etcdInstance resource
	g.Expect(cl.Create(ctx, etcdInstance)).To(Succeed())
	// ***************** test etcd spec reconciliation  *****************
	assertNoComponentsExist(ctx, t, reconcilerTestEnv, etcdInstance, 10*time.Second, 2*time.Second)
	assertReconcileSuspensionEventRecorded(ctx, t, cl, client.ObjectKeyFromObject(etcdInstance), 10*time.Second, 2*time.Second)
	assertETCDObservedGeneration(t, reconcilerTestEnv.itTestEnv.GetClient(), client.ObjectKeyFromObject(etcdInstance), nil, 5*time.Second, 1*time.Second)
	assertETCDLastOperationAndLastErrorsUpdatedSuccessfully(t, cl, client.ObjectKeyFromObject(etcdInstance), nil, nil, 5*time.Second, 1*time.Second)
	assertETCDOperationAnnotation(t, reconcilerTestEnv.itTestEnv.GetClient(), client.ObjectKeyFromObject(etcdInstance), true, 5*time.Second, 1*time.Second)
}

func testWhenNoReconcileOperationAnnotationIsSet(t *testing.T, testNs string, reconcilerTestEnv ReconcilerTestEnv) {
	// ***************** setup *****************
	g := NewWithT(t)
	etcdInstance := testutils.EtcdBuilderWithoutDefaults(testutils.TestEtcdName, testNs).
		WithClientTLS().
		WithPeerTLS().
		WithReplicas(3).
		Build()
	ctx := context.Background()
	cl := reconcilerTestEnv.itTestEnv.GetClient()
	// create etcdInstance resource
	g.Expect(cl.Create(ctx, etcdInstance)).To(Succeed())
	// ***************** test etcd spec reconciliation  *****************
	assertNoComponentsExist(ctx, t, reconcilerTestEnv, etcdInstance, 10*time.Second, 2*time.Second)
	assertETCDObservedGeneration(t, reconcilerTestEnv.itTestEnv.GetClient(), client.ObjectKeyFromObject(etcdInstance), nil, 5*time.Second, 1*time.Second)
	assertETCDLastOperationAndLastErrorsUpdatedSuccessfully(t, cl, client.ObjectKeyFromObject(etcdInstance), nil, nil, 5*time.Second, 1*time.Second)
}

// ------------------------------ reconcile deletion tests ------------------------------
func TestDeletionOfAllEtcdResourcesWhenEtcdMarkedForDeletion(t *testing.T) {
	// ***************** setup *****************
	itTestEnv, itTestEnvCloser := setup.NewIntegrationTestEnv(t, "etcd-reconciler", []string{assets.GetEtcdCrdPath()})
	defer itTestEnvCloser(t)
	reconcilerTestEnv := initializeEtcdReconcilerTestEnv(t, itTestEnv, false)

	// --------------------------- create test namespace ---------------------------
	testNs := createTestNamespaceName(t)
	itTestEnv.CreateTestNamespace(testNs)
	t.Logf("successfully create namespace: %s to run test => '%s'", testNs, t.Name())

	// ---------------------------- create etcd instance --------------------------
	g := NewWithT(t)
	etcdInstance := testutils.EtcdBuilderWithoutDefaults(testutils.TestEtcdName, testNs).
		WithClientTLS().
		WithPeerTLS().
		WithReplicas(3).
		WithAnnotations(map[string]string{
			v1beta1constants.GardenerOperation: v1beta1constants.GardenerOperationReconcile,
		}).Build()
	ctx := context.Background()
	cl := reconcilerTestEnv.itTestEnv.GetClient()
	createEtcdInstanceWithFinalizer(ctx, t, reconcilerTestEnv, etcdInstance)

	// ***************** test etcd deletion flow  *****************
	// mark etcd for deletion
	g.Expect(cl.Delete(ctx, etcdInstance)).To(Succeed())
	t.Logf("successfully marked etcd instance for deletion: %s, waiting for resources to be removed...", etcdInstance.Name)
	assertNoComponentsExist(ctx, t, reconcilerTestEnv, etcdInstance, 2*time.Minute, 2*time.Second)
	t.Logf("successfully deleted all resources for etcd instance: %s, waiting for finalizer to be removed from etcd...", etcdInstance.Name)
	assertETCDFinalizer(t, cl, client.ObjectKeyFromObject(etcdInstance), false, 2*time.Minute, 2*time.Second)
}

func TestPartialDeletionFailureOfEtcdResourcesWhenEtcdMarkedForDeletion(t *testing.T) {
	// ********************************** setup **********************************
	testNs := createTestNamespaceName(t)
	etcdInstance := testutils.EtcdBuilderWithoutDefaults(testutils.TestEtcdName, testNs).
		WithClientTLS().
		WithPeerTLS().
		WithReplicas(3).
		WithDefaultBackup().
		WithAnnotations(map[string]string{
			v1beta1constants.GardenerOperation: v1beta1constants.GardenerOperationReconcile,
		}).
		Build()
	// create the test client builder and record errors for delete operations for client service and snapshot lease.
	testClientBuilder := testutils.NewTestClientBuilder().
		RecordErrorForObjects(testutils.ClientMethodDelete, testutils.TestAPIInternalErr, client.ObjectKey{Name: etcdInstance.GetClientServiceName(), Namespace: etcdInstance.Namespace}).
		RecordErrorForObjectsMatchingLabels(testutils.ClientMethodDeleteAll, etcdInstance.Namespace, map[string]string{
			druidv1alpha1.LabelComponentKey: common.SnapshotLeaseComponentName,
			druidv1alpha1.LabelManagedByKey: druidv1alpha1.LabelManagedByValue,
			druidv1alpha1.LabelPartOfKey:    etcdInstance.Name,
		}, testutils.TestAPIInternalErr)
	// create the integration test environment with the test client builder
	itTestEnv, itTestEnvCloser := setup.NewIntegrationTestEnvWithClientBuilder(t, "etcd-reconciler", []string{assets.GetEtcdCrdPath()}, testClientBuilder)
	defer itTestEnvCloser(t)
	reconcilerTestEnv := initializeEtcdReconcilerTestEnv(t, itTestEnv, false)

	// create test namespace
	itTestEnv.CreateTestNamespace(testNs)
	t.Logf("successfully create namespace: %s to run test => '%s'", testNs, t.Name())

	g := NewWithT(t)
	ctx := context.Background()
	cl := itTestEnv.GetClient()

	// ensure that the backup store is enabled and backup secrets are created
	g.Expect(etcdInstance.Spec.Backup.Store).ToNot(BeNil())
	g.Expect(etcdInstance.Spec.Backup.Store.SecretRef).ToNot(BeNil())
	g.Expect(testutils.CreateSecrets(ctx, cl, testNs, etcdInstance.Spec.Backup.Store.SecretRef.Name)).To(Succeed())
	t.Logf("successfully created backup secrets for etcd instance: %s", etcdInstance.Name)

	createEtcdInstanceWithFinalizer(ctx, t, reconcilerTestEnv, etcdInstance)

	// ******************************* test etcd deletion flow *******************************
	// mark etcd for deletion
	g.Expect(cl.Delete(ctx, etcdInstance)).To(Succeed())
	t.Logf("successfully marked etcd instance for deletion: %s, waiting for resources to be removed...", etcdInstance.Name)
	// assert removal of all components except client service and snapshot lease.
	assertComponentsDoNotExist(ctx, t, reconcilerTestEnv, etcdInstance, []operator.Kind{
		operator.MemberLeaseKind,
		operator.PeerServiceKind,
		operator.ConfigMapKind,
		operator.PodDisruptionBudgetKind,
		operator.ServiceAccountKind,
		operator.RoleKind,
		operator.RoleBindingKind,
		operator.StatefulSetKind}, 2*time.Minute, 2*time.Second)
	assertSelectedComponentsExists(ctx, t, reconcilerTestEnv, etcdInstance, []operator.Kind{operator.ClientServiceKind, operator.SnapshotLeaseKind}, 2*time.Minute, 2*time.Second)
	// assert that the last operation and last errors are updated correctly.
	expectedLastOperation := &druidv1alpha1.LastOperation{
		Type:  druidv1alpha1.LastOperationTypeDelete,
		State: druidv1alpha1.LastOperationStateError,
	}
	expectedLastErrors := []druidv1alpha1.LastError{
		{
			Code: "ERR_DELETE_CLIENT_SERVICE",
		},
		{
			Code: "ERR_DELETE_SNAPSHOT_LEASE",
		},
	}
	assertETCDLastOperationAndLastErrorsUpdatedSuccessfully(t, cl, client.ObjectKeyFromObject(etcdInstance), expectedLastOperation, expectedLastErrors, 2*time.Minute, 2*time.Second)
	// assert that the finalizer has not been removed as all resources have not been deleted yet.
	assertETCDFinalizer(t, cl, client.ObjectKeyFromObject(etcdInstance), true, 10*time.Second, 2*time.Second)
}

// ------------------------------ reconcile status tests ------------------------------

// -------------------------  Helper functions  -------------------------
func createTestNamespaceName(t *testing.T) string {
	b := make([]byte, 4)
	_, err := rand.Read(b)
	g := NewWithT(t)
	g.Expect(err).ToNot(HaveOccurred())
	namespaceSuffix := hex.EncodeToString(b)
	return fmt.Sprintf("%s-%s", testNamespacePrefix, namespaceSuffix)
}

func initializeEtcdReconcilerTestEnv(t *testing.T, itTestEnv setup.IntegrationTestEnv, autoReconcile bool) ReconcilerTestEnv {
	g := NewWithT(t)
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
		itTestEnv:  itTestEnv,
		reconciler: reconciler,
	}
}

func createEtcdInstanceWithFinalizer(ctx context.Context, t *testing.T, reconcilerTestEnv ReconcilerTestEnv, etcdInstance *druidv1alpha1.Etcd) {
	g := NewWithT(t)
	cl := reconcilerTestEnv.itTestEnv.GetClient()
	// create etcdInstance resource
	g.Expect(cl.Create(ctx, etcdInstance)).To(Succeed())
	t.Logf("trigggered creation of etcd instance: %s, waiting for resources to be created...", etcdInstance.Name)
	// ascertain that all etcd resources are created
	assertAllComponentsExists(ctx, t, reconcilerTestEnv, etcdInstance, 3*time.Minute, 2*time.Second)
	t.Logf("successfully created all resources for etcd instance: %s", etcdInstance.Name)
	// add finalizer
	addFinalizer(ctx, g, cl, client.ObjectKeyFromObject(etcdInstance))
	t.Logf("successfully added finalizer to etcd instance: %s", etcdInstance.Name)
}

func addFinalizer(ctx context.Context, g *WithT, cl client.Client, etcdObjectKey client.ObjectKey) {
	etcdInstance := &druidv1alpha1.Etcd{}
	g.Expect(cl.Get(ctx, etcdObjectKey, etcdInstance)).To(Succeed())
	g.Expect(controllerutils.AddFinalizers(ctx, cl, etcdInstance, common.FinalizerName)).To(Succeed())
}
