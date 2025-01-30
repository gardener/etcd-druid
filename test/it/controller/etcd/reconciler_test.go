// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package etcd

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"
	"github.com/gardener/etcd-druid/internal/component"
	"github.com/gardener/etcd-druid/internal/utils/kubernetes"
	"github.com/gardener/etcd-druid/test/it/controller/assets"
	"github.com/gardener/etcd-druid/test/it/setup"
	testutils "github.com/gardener/etcd-druid/test/utils"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	testclock "k8s.io/utils/clock/testing"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/gomega"
)

var (
	sharedITTestEnv setup.DruidTestEnvironment
)

func TestMain(m *testing.M) {
	var (
		itTestEnvCloser setup.DruidTestEnvCloser
		err             error
	)
	sharedITTestEnv, itTestEnvCloser, err = setup.NewDruidTestEnvironment("etcd-reconciler", []string{assets.GetEtcdCrdPath()})
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to create integration test environment: %v\n", err)
		os.Exit(1)
	}

	// os.Exit() does not respect defer statements
	exitCode := m.Run()
	itTestEnvCloser()
	os.Exit(exitCode)
}

// ------------------------------ reconcile spec tests ------------------------------
func TestEtcdReconcileSpecWithNoAutoReconcile(t *testing.T) {
	reconcilerTestEnv := initializeEtcdReconcilerTestEnv(t, "etcd-controller-reconcile-with-no-auto-reconcile", sharedITTestEnv, false, testutils.NewTestClientBuilder())
	tests := []struct {
		name string
		fn   func(t *testing.T, testNamespace string, reconcilerTestEnv ReconcilerTestEnv)
	}{
		{"should add finalizer to etcd when etcd resource is created", testAddFinalizerToEtcd},
		{"should create all managed resources when etcd resource is created", testAllManagedResourcesAreCreated},
		{"should succeed only in creation of some resources and not all and should record error in lastErrors and lastOperation", testFailureToCreateAllResources},
		{"should not reconcile spec when reconciliation is suspended", testWhenReconciliationIsSuspended},
		{"should not reconcile upon etcd spec update when no reconcile operation annotation is set", testEtcdSpecUpdateWhenNoReconcileOperationAnnotationIsSet},
		{"should reconcile upon etcd spec update when reconcile operation annotation is set", testEtcdSpecUpdateWhenReconcileOperationAnnotationIsSet},
	}
	g := NewWithT(t)
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testNs := createTestNamespaceName(t)
			t.Logf("successfully create namespace: %s to run test => '%s'", testNs, t.Name())
			g.Expect(reconcilerTestEnv.itTestEnv.CreateTestNamespace(testNs)).To(Succeed())
			test.fn(t, testNs, reconcilerTestEnv)
		})
	}
}

func testAddFinalizerToEtcd(t *testing.T, testNs string, reconcilerTestEnv ReconcilerTestEnv) {
	const (
		timeout         = time.Minute * 2
		pollingInterval = time.Second * 2
	)
	// ***************** setup *****************
	g := NewWithT(t)
	etcdInstance := testutils.EtcdBuilderWithDefaults(testutils.TestEtcdName, testNs).
		WithReplicas(3).
		Build()
	etcdInstance.Spec.Backup.Store = &druidv1alpha1.StoreSpec{} // empty store spec since backups are not required for this test
	cl := reconcilerTestEnv.itTestEnv.GetClient()
	ctx := context.Background()
	// create etcdInstance resource
	g.Expect(cl.Create(ctx, etcdInstance)).To(Succeed())
	// ***************** test etcd spec reconciliation  *****************
	assertETCDFinalizer(t, cl, client.ObjectKeyFromObject(etcdInstance), true, timeout, pollingInterval)
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
	expectedLastOperation := &druidv1alpha1.LastOperation{
		Type:  druidv1alpha1.LastOperationTypeReconcile,
		State: druidv1alpha1.LastOperationStateSucceeded,
	}
	assertETCDLastOperationAndLastErrorsUpdatedSuccessfully(t, reconcilerTestEnv.itTestEnv.GetClient(), client.ObjectKeyFromObject(etcdInstance), expectedLastOperation, nil, 5*time.Second, 1*time.Second)
	assertETCDObservedGeneration(t, reconcilerTestEnv.itTestEnv.GetClient(), client.ObjectKeyFromObject(etcdInstance), ptr.To[int64](1), 30*time.Second, 1*time.Second)
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
		WithAnnotations(map[string]string{druidv1alpha1.DruidOperationAnnotation: druidv1alpha1.DruidOperationReconcile}).
		// The client service label value has an invalid value. ':' are not accepted as valid character in label values.
		// This should fail creation of client service and cause a requeue.
		WithEtcdClientServiceLabels(map[string]string{"invalid-label": "invalid-label:value"}).
		Build()

	ctx := context.Background()
	cl := reconcilerTestEnv.itTestEnv.GetClient()
	// create etcdInstance resource
	g.Expect(cl.Create(ctx, etcdInstance)).To(Succeed())
	// ***************** test etcd spec reconciliation  *****************
	componentKindCreated := []component.Kind{component.MemberLeaseKind}
	assertSelectedComponentsExists(ctx, t, reconcilerTestEnv, etcdInstance, componentKindCreated, timeout, pollingInterval)
	componentKindNotCreated := []component.Kind{
		component.SnapshotLeaseKind, // no backup store has been set
		component.ClientServiceKind,
		component.PeerServiceKind,
		component.ConfigMapKind,
		component.PodDisruptionBudgetKind,
		component.ServiceAccountKind,
		component.RoleKind,
		component.RoleBindingKind,
		component.StatefulSetKind,
	}
	assertComponentsDoNotExist(ctx, t, reconcilerTestEnv, etcdInstance, componentKindNotCreated, timeout, pollingInterval)
	expectedLastOperation := &druidv1alpha1.LastOperation{
		Type:  druidv1alpha1.LastOperationTypeReconcile,
		State: druidv1alpha1.LastOperationStateError,
	}
	assertETCDLastOperationAndLastErrorsUpdatedSuccessfully(t, cl, client.ObjectKeyFromObject(etcdInstance), expectedLastOperation, []string{"ERR_SYNC_CLIENT_SERVICE"}, 5*time.Second, 1*time.Second)
	assertETCDObservedGeneration(t, reconcilerTestEnv.itTestEnv.GetClient(), client.ObjectKeyFromObject(etcdInstance), nil, 30*time.Second, 1*time.Second)
}

func testWhenReconciliationIsSuspended(t *testing.T, testNs string, reconcilerTestEnv ReconcilerTestEnv) {
	// ***************** setup *****************
	g := NewWithT(t)
	etcdInstance := testutils.EtcdBuilderWithoutDefaults(testutils.TestEtcdName, testNs).
		WithClientTLS().
		WithPeerTLS().
		WithReplicas(3).
		WithAnnotations(map[string]string{
			druidv1alpha1.DruidOperationAnnotation:           druidv1alpha1.DruidOperationReconcile,
			druidv1alpha1.SuspendEtcdSpecReconcileAnnotation: "",
		}).Build()
	ctx := context.Background()
	cl := reconcilerTestEnv.itTestEnv.GetClient()
	// create etcdInstance resource
	g.Expect(cl.Create(ctx, etcdInstance)).To(Succeed())
	// ***************** test etcd spec reconciliation  *****************
	assertNoComponentsExist(ctx, t, reconcilerTestEnv, etcdInstance, 10*time.Second, 2*time.Second)
	assertReconcileSuspensionEventRecorded(ctx, t, cl, client.ObjectKeyFromObject(etcdInstance), 10*time.Second, 2*time.Second)
	assertETCDLastOperationAndLastErrorsUpdatedSuccessfully(t, cl, client.ObjectKeyFromObject(etcdInstance), nil, nil, 5*time.Second, 1*time.Second)
	assertETCDObservedGeneration(t, reconcilerTestEnv.itTestEnv.GetClient(), client.ObjectKeyFromObject(etcdInstance), nil, 30*time.Second, 1*time.Second)
	assertETCDOperationAnnotation(t, reconcilerTestEnv.itTestEnv.GetClient(), client.ObjectKeyFromObject(etcdInstance), true, 5*time.Second, 1*time.Second)
}

func testEtcdSpecUpdateWhenNoReconcileOperationAnnotationIsSet(t *testing.T, testNs string, reconcilerTestEnv ReconcilerTestEnv) {
	// ***************** setup *****************
	g := NewWithT(t)
	etcdInstance := testutils.EtcdBuilderWithoutDefaults(testutils.TestEtcdName, testNs).
		WithClientTLS().
		WithPeerTLS().
		WithReplicas(3).
		Build()
	ctx := context.Background()
	cl := reconcilerTestEnv.itTestEnv.GetClient()
	// create etcdInstance resource and assert successful reconciliation, and ensure that sts generation is 1
	createAndAssertEtcdReconciliation(ctx, t, reconcilerTestEnv, etcdInstance)
	assertStatefulSetGeneration(ctx, t, reconcilerTestEnv.itTestEnv.GetClient(), client.ObjectKeyFromObject(etcdInstance), 1, 30*time.Second, 2*time.Second)
	// get updated version of etcdInstance
	g.Expect(cl.Get(ctx, druidv1alpha1.GetNamespaceName(etcdInstance.ObjectMeta), etcdInstance)).To(Succeed())
	// update etcdInstance spec without reconcile operation annotation set
	originalEtcdInstance := etcdInstance.DeepCopy()
	metricsLevelExtensive := druidv1alpha1.Extensive
	etcdInstance.Spec.Etcd.Metrics = &metricsLevelExtensive
	g.Expect(cl.Patch(ctx, etcdInstance, client.MergeFrom(originalEtcdInstance))).To(Succeed())

	// ***************** test etcd spec reconciliation  *****************
	assertAllComponentsExists(ctx, t, reconcilerTestEnv, etcdInstance, 2*time.Second, 2*time.Second)
	_ = updateAndGetStsRevision(ctx, t, reconcilerTestEnv.itTestEnv.GetClient(), etcdInstance)
	assertETCDObservedGeneration(t, reconcilerTestEnv.itTestEnv.GetClient(), client.ObjectKeyFromObject(etcdInstance), ptr.To[int64](1), 30*time.Second, 1*time.Second)
	// ensure that sts generation does not change, ie, it should remain 1, as sts is not updated after etcd spec change without reconcile operation annotation
	assertStatefulSetGeneration(ctx, t, reconcilerTestEnv.itTestEnv.GetClient(), client.ObjectKeyFromObject(etcdInstance), 1, 30*time.Second, 2*time.Second)
}

func testEtcdSpecUpdateWhenReconcileOperationAnnotationIsSet(t *testing.T, testNs string, reconcilerTestEnv ReconcilerTestEnv) {
	// ***************** setup *****************
	g := NewWithT(t)
	etcdInstance := testutils.EtcdBuilderWithoutDefaults(testutils.TestEtcdName, testNs).
		WithClientTLS().
		WithPeerTLS().
		WithReplicas(3).
		Build()
	ctx := context.Background()
	cl := reconcilerTestEnv.itTestEnv.GetClient()
	// create etcdInstance resource and assert successful reconciliation, and ensure that sts generation is 1
	createAndAssertEtcdReconciliation(ctx, t, reconcilerTestEnv, etcdInstance)
	assertStatefulSetGeneration(ctx, t, reconcilerTestEnv.itTestEnv.GetClient(), client.ObjectKeyFromObject(etcdInstance), 1, 30*time.Second, 2*time.Second)
	// update member leases with peer-tls-enabled annotation set to true
	memberLeaseNames := druidv1alpha1.GetMemberLeaseNames(etcdInstance.ObjectMeta, etcdInstance.Spec.Replicas)
	t.Log("updating member leases with peer-tls-enabled annotation set to true")
	mlcs := []etcdMemberLeaseConfig{
		{name: memberLeaseNames[0], annotations: map[string]string{kubernetes.LeaseAnnotationKeyPeerURLTLSEnabled: "true"}},
		{name: memberLeaseNames[1], annotations: map[string]string{kubernetes.LeaseAnnotationKeyPeerURLTLSEnabled: "true"}},
		{name: memberLeaseNames[2], annotations: map[string]string{kubernetes.LeaseAnnotationKeyPeerURLTLSEnabled: "true"}},
	}
	updateMemberLeases(context.Background(), t, reconcilerTestEnv.itTestEnv.GetClient(), testNs, mlcs)
	// get latest version of etcdInstance
	g.Expect(cl.Get(ctx, druidv1alpha1.GetNamespaceName(etcdInstance.ObjectMeta), etcdInstance)).To(Succeed())
	// update etcdInstance spec with reconcile operation annotation also set
	originalEtcdInstance := etcdInstance.DeepCopy()
	metricsLevelExtensive := druidv1alpha1.Extensive
	etcdInstance.Spec.Etcd.Metrics = &metricsLevelExtensive
	etcdInstance.Annotations = map[string]string{
		druidv1alpha1.DruidOperationAnnotation: druidv1alpha1.DruidOperationReconcile,
	}
	g.Expect(cl.Patch(ctx, etcdInstance, client.MergeFrom(originalEtcdInstance))).To(Succeed())

	// ***************** test etcd spec reconciliation  *****************
	assertAllComponentsExists(ctx, t, reconcilerTestEnv, etcdInstance, 30*time.Minute, 2*time.Second)
	_ = updateAndGetStsRevision(ctx, t, reconcilerTestEnv.itTestEnv.GetClient(), etcdInstance)
	expectedLastOperation := &druidv1alpha1.LastOperation{
		Type:  druidv1alpha1.LastOperationTypeReconcile,
		State: druidv1alpha1.LastOperationStateSucceeded,
	}
	assertETCDLastOperationAndLastErrorsUpdatedSuccessfully(t, reconcilerTestEnv.itTestEnv.GetClient(), client.ObjectKeyFromObject(etcdInstance), expectedLastOperation, nil, 5*time.Second, 1*time.Second)
	assertETCDObservedGeneration(t, reconcilerTestEnv.itTestEnv.GetClient(), client.ObjectKeyFromObject(etcdInstance), ptr.To[int64](2), 30*time.Second, 1*time.Second)
	assertETCDOperationAnnotation(t, reconcilerTestEnv.itTestEnv.GetClient(), client.ObjectKeyFromObject(etcdInstance), false, 5*time.Second, 1*time.Second)
	// ensure that sts generation is updated to 2, since reconciliation of the etcd spec change causes an update of the sts spec
	assertStatefulSetGeneration(ctx, t, reconcilerTestEnv.itTestEnv.GetClient(), client.ObjectKeyFromObject(etcdInstance), 2, 30*time.Second, 2*time.Second)
}

// ------------------------------ reconcile deletion tests ------------------------------

func TestEtcdDeletion(t *testing.T) {
	tests := []struct {
		name string
		fn   func(t *testing.T, testNamespace string)
	}{
		{"test deletion of all etcd resources when etcd marked for deletion", testDeletionOfAllEtcdResourcesWhenEtcdMarkedForDeletion},
		{"test partial deletion failure of etcd resources when etcd marked for deletion", testPartialDeletionFailureOfEtcdResourcesWhenEtcdMarkedForDeletion},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testNs := createTestNamespaceName(t)
			test.fn(t, testNs)
		})
	}
}

func testDeletionOfAllEtcdResourcesWhenEtcdMarkedForDeletion(t *testing.T, testNs string) {
	g := NewWithT(t)
	reconcilerTestEnv := initializeEtcdReconcilerTestEnv(t, "etcd-controller-deletion", sharedITTestEnv, false, testutils.NewTestClientBuilder())
	// ---------------------------- create test namespace ---------------------------
	t.Logf("successfully create namespace: %s to run test => '%s'", testNs, t.Name())
	g.Expect(sharedITTestEnv.CreateTestNamespace(testNs)).To(Succeed())
	// ---------------------------- create etcd instance --------------------------
	etcdInstance := testutils.EtcdBuilderWithoutDefaults(testutils.TestEtcdName, testNs).
		WithClientTLS().
		WithPeerTLS().
		WithReplicas(3).
		WithAnnotations(map[string]string{
			druidv1alpha1.DruidOperationAnnotation: druidv1alpha1.DruidOperationReconcile,
		}).Build()
	ctx := context.Background()
	cl := reconcilerTestEnv.itTestEnv.GetClient()
	createAndAssertEtcdAndAllManagedResources(ctx, t, reconcilerTestEnv, etcdInstance)

	// ***************** test etcd deletion flow  *****************
	// mark etcd for deletion
	g.Expect(cl.Delete(ctx, etcdInstance)).To(Succeed())
	t.Logf("successfully marked etcd instance for deletion: %s, waiting for resources to be removed...", etcdInstance.Name)
	assertNoComponentsExist(ctx, t, reconcilerTestEnv, etcdInstance, 2*time.Minute, 2*time.Second)
	t.Logf("successfully deleted all resources for etcd instance: %s, waiting for finalizer to be removed from etcd...", etcdInstance.Name)
	assertETCDFinalizer(t, cl, client.ObjectKeyFromObject(etcdInstance), false, 2*time.Minute, 2*time.Second)
}

func testPartialDeletionFailureOfEtcdResourcesWhenEtcdMarkedForDeletion(t *testing.T, testNs string) {
	// ********************************** setup **********************************
	etcdInstance := testutils.EtcdBuilderWithoutDefaults(testutils.TestEtcdName, testNs).
		WithClientTLS().
		WithPeerTLS().
		WithReplicas(3).
		WithDefaultBackup().
		WithAnnotations(map[string]string{
			druidv1alpha1.DruidOperationAnnotation: druidv1alpha1.DruidOperationReconcile,
		}).
		Build()
	// create the test client builder and record errors for delete operations for client service and snapshot lease.
	testClientBuilder := testutils.NewTestClientBuilder().
		RecordErrorForObjects(testutils.ClientMethodDelete, testutils.TestAPIInternalErr, client.ObjectKey{Name: druidv1alpha1.GetClientServiceName(etcdInstance.ObjectMeta), Namespace: etcdInstance.Namespace}).
		RecordErrorForObjectsMatchingLabels(testutils.ClientMethodDeleteAll, etcdInstance.Namespace, map[string]string{
			druidv1alpha1.LabelComponentKey: common.ComponentNameSnapshotLease,
			druidv1alpha1.LabelManagedByKey: druidv1alpha1.LabelManagedByValue,
			druidv1alpha1.LabelPartOfKey:    etcdInstance.Name,
		}, testutils.TestAPIInternalErr)

	g := NewWithT(t)

	// A different IT test environment is required due to a different clientBuilder which is used to create the manager.
	itTestEnv, itTestEnvCloser, err := setup.NewDruidTestEnvironment("etcd-reconciler", []string{assets.GetEtcdCrdPath()})
	g.Expect(err).ToNot(HaveOccurred())
	defer itTestEnvCloser()
	reconcilerTestEnv := initializeEtcdReconcilerTestEnv(t, "etcd-controller-deletion-2", itTestEnv, false, testClientBuilder)

	// ---------------------------- create test namespace ---------------------------
	t.Logf("successfully create namespace: %s to run test => '%s'", testNs, t.Name())
	g.Expect(itTestEnv.CreateTestNamespace(testNs)).To(Succeed())

	// ---------------------------- create etcd instance --------------------------
	ctx := context.Background()
	cl := itTestEnv.GetClient()

	// ensure that the backup store is enabled and backup secrets are created
	g.Expect(etcdInstance.Spec.Backup.Store).ToNot(BeNil())
	g.Expect(etcdInstance.Spec.Backup.Store.SecretRef).ToNot(BeNil())
	g.Expect(testutils.CreateSecrets(ctx, cl, testNs, etcdInstance.Spec.Backup.Store.SecretRef.Name)).To(Succeed())
	t.Logf("successfully created backup secrets for etcd instance: %s", etcdInstance.Name)

	createAndAssertEtcdAndAllManagedResources(ctx, t, reconcilerTestEnv, etcdInstance)

	// ******************************* test etcd deletion flow *******************************
	// mark etcd for deletion
	g.Expect(cl.Delete(ctx, etcdInstance)).To(Succeed())
	t.Logf("successfully marked etcd instance for deletion: %s, waiting for resources to be removed...", etcdInstance.Name)
	// assert removal of all components except client service and snapshot lease.
	assertComponentsDoNotExist(ctx, t, reconcilerTestEnv, etcdInstance, []component.Kind{
		component.MemberLeaseKind,
		component.PeerServiceKind,
		component.ConfigMapKind,
		component.PodDisruptionBudgetKind,
		component.ServiceAccountKind,
		component.RoleKind,
		component.RoleBindingKind,
		component.StatefulSetKind}, 2*time.Minute, 2*time.Second)

	assertSelectedComponentsExists(ctx, t, reconcilerTestEnv, etcdInstance, []component.Kind{component.ClientServiceKind, component.SnapshotLeaseKind}, 2*time.Minute, 2*time.Second)
	// assert that the last operation and last errors are updated correctly.
	expectedLastOperation := &druidv1alpha1.LastOperation{
		Type:  druidv1alpha1.LastOperationTypeDelete,
		State: druidv1alpha1.LastOperationStateError,
	}
	assertETCDLastOperationAndLastErrorsUpdatedSuccessfully(t, cl, client.ObjectKeyFromObject(etcdInstance), expectedLastOperation, []string{"ERR_DELETE_CLIENT_SERVICE", "ERR_DELETE_SNAPSHOT_LEASE"}, 2*time.Minute, 2*time.Second)
	// assert that the finalizer has not been removed as all resources have not been deleted yet.
	assertETCDFinalizer(t, cl, client.ObjectKeyFromObject(etcdInstance), true, 10*time.Second, 2*time.Second)
}

// ------------------------------ reconcile status tests ------------------------------

func TestEtcdStatusReconciliation(t *testing.T) {
	reconcilerTestEnv := initializeEtcdReconcilerTestEnv(t, "etcd-controller-reconcile-status-test", sharedITTestEnv, false, testutils.NewTestClientBuilder())
	tests := []struct {
		name string
		fn   func(t *testing.T, etcd *druidv1alpha1.Etcd, reconcilerTestEnv ReconcilerTestEnv)
	}{
		{"test status when all member leases are active", testConditionsAndMembersWhenAllMemberLeasesAreActive},
		{"test all-members-updated condition when all sts replicas are updated", testConditionsWhenStatefulSetReplicasHaveBeenUpdated},
		{"test data volume condition reflects pvc error event", testEtcdStatusReflectsPVCErrorEvent},
		{"test status when all sts replicas are ready", testEtcdStatusIsInSyncWithStatefulSetStatusWhenAllReplicasAreReady},
		{"test status when not all sts replicas are ready", testEtcdStatusIsInSyncWithStatefulSetStatusWhenNotAllReplicasAreReady},
		{"test status when sts current revision is older than update revision", testEtcdStatusIsInSyncWithStatefulSetStatusWhenCurrentRevisionIsOlderThanUpdateRevision},
		/*
			Additional tests to check the conditions and member status should be added when we solve https://github.com/gardener/etcd-druid/issues/645
			Currently only one happy-state test has been added as a template for other tests to follow once the conditions are refactored.
			Writing additional tests for member status and conditions would be a waste of time as they will have to be modified again.
		*/
	}

	ctx := context.Background()
	g := NewWithT(t)
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// --------------------------- create test namespace ---------------------------
			testNs := createTestNamespaceName(t)
			g.Expect(reconcilerTestEnv.itTestEnv.CreateTestNamespace(testNs)).To(Succeed())
			t.Logf("successfully create namespace: %s to run test => '%s'", testNs, t.Name())
			// ---------------------------- create etcd instance --------------------------
			etcdInstance := testutils.EtcdBuilderWithoutDefaults(testutils.TestEtcdName, testNs).
				WithClientTLS().
				WithPeerTLS().
				WithReplicas(3).
				WithAnnotations(map[string]string{
					druidv1alpha1.DruidOperationAnnotation: druidv1alpha1.DruidOperationReconcile,
				}).Build()
			createAndAssertEtcdAndAllManagedResources(ctx, t, reconcilerTestEnv, etcdInstance)
			test.fn(t, etcdInstance, reconcilerTestEnv)
		})
	}
}

func testConditionsAndMembersWhenAllMemberLeasesAreActive(t *testing.T, etcd *druidv1alpha1.Etcd, reconcilerTestEnv ReconcilerTestEnv) {
	memberLeaseNames := druidv1alpha1.GetMemberLeaseNames(etcd.ObjectMeta, etcd.Spec.Replicas)
	testNs := etcd.Namespace
	clock := testclock.NewFakeClock(time.Now().Round(time.Second))
	g := NewWithT(t)
	mlcs := []etcdMemberLeaseConfig{
		{name: memberLeaseNames[0], memberID: testutils.GenerateRandomAlphanumericString(g, 8), role: druidv1alpha1.EtcdRoleMember, renewTime: &metav1.MicroTime{Time: clock.Now().Add(-time.Second * 30)}},
		{name: memberLeaseNames[1], memberID: testutils.GenerateRandomAlphanumericString(g, 8), role: druidv1alpha1.EtcdRoleLeader, renewTime: &metav1.MicroTime{Time: clock.Now()}},
		{name: memberLeaseNames[2], memberID: testutils.GenerateRandomAlphanumericString(g, 8), role: druidv1alpha1.EtcdRoleMember, renewTime: &metav1.MicroTime{Time: clock.Now().Add(-time.Second * 30)}},
	}
	updateMemberLeases(context.Background(), t, reconcilerTestEnv.itTestEnv.GetClient(), testNs, mlcs)
	// ******************************* test etcd status update flow *******************************
	expectedConditions := []druidv1alpha1.Condition{
		{Type: druidv1alpha1.ConditionTypeReady, Status: druidv1alpha1.ConditionTrue},
		{Type: druidv1alpha1.ConditionTypeAllMembersReady, Status: druidv1alpha1.ConditionTrue},
		{Type: druidv1alpha1.ConditionTypeDataVolumesReady, Status: druidv1alpha1.ConditionTrue},
	}
	cl := reconcilerTestEnv.itTestEnv.GetClient()
	assertETCDStatusConditions(t, cl, client.ObjectKeyFromObject(etcd), expectedConditions, 2*time.Minute, 2*time.Second)
	expectedMemberStatuses := []druidv1alpha1.EtcdMemberStatus{
		{Name: mlcs[0].name, ID: ptr.To(mlcs[0].memberID), Role: &mlcs[0].role, Status: druidv1alpha1.EtcdMemberStatusReady},
		{Name: mlcs[1].name, ID: ptr.To(mlcs[1].memberID), Role: &mlcs[1].role, Status: druidv1alpha1.EtcdMemberStatusReady},
		{Name: mlcs[2].name, ID: ptr.To(mlcs[2].memberID), Role: &mlcs[2].role, Status: druidv1alpha1.EtcdMemberStatusReady},
	}
	assertETCDMemberStatuses(t, cl, client.ObjectKeyFromObject(etcd), expectedMemberStatuses, 2*time.Minute, 2*time.Second)
}

func testConditionsWhenStatefulSetReplicasHaveBeenUpdated(t *testing.T, etcd *druidv1alpha1.Etcd, reconcilerTestEnv ReconcilerTestEnv) {
	g := NewWithT(t)
	cl := reconcilerTestEnv.itTestEnv.GetClient()
	ctx := context.Background()
	sts := &appsv1.StatefulSet{}
	g.Expect(cl.Get(ctx, client.ObjectKey{Name: etcd.Name, Namespace: etcd.Namespace}, sts)).To(Succeed())
	stsCopy := sts.DeepCopy()
	stsCopy.Status.ObservedGeneration = stsCopy.Generation
	stsCopy.Status.CurrentRevision = fmt.Sprintf("%s-%s", stsCopy.Name, testutils.GenerateRandomAlphanumericString(g, 2))
	stsCopy.Status.UpdateRevision = stsCopy.Status.CurrentRevision
	stsCopy.Status.UpdatedReplicas = *stsCopy.Spec.Replicas
	g.Expect(cl.Status().Update(ctx, stsCopy)).To(Succeed())
	// ******************************* test etcd status update flow *******************************
	expectedConditions := []druidv1alpha1.Condition{
		{Type: druidv1alpha1.ConditionTypeAllMembersUpdated, Status: druidv1alpha1.ConditionTrue},
	}
	assertETCDStatusConditions(t, cl, client.ObjectKeyFromObject(etcd), expectedConditions, 5*time.Minute, 2*time.Second)
}

func testEtcdStatusReflectsPVCErrorEvent(t *testing.T, etcd *druidv1alpha1.Etcd, reconcilerTestEnv ReconcilerTestEnv) {
	g := NewWithT(t)
	testNs := etcd.Namespace
	cl := reconcilerTestEnv.itTestEnv.GetClient()
	ctx := context.Background()
	sts := &appsv1.StatefulSet{}
	g.Expect(cl.Get(ctx, client.ObjectKey{Name: etcd.Name, Namespace: testNs}, sts)).To(Succeed())
	g.Expect(sts.Spec.VolumeClaimTemplates).To(HaveLen(1))
	// create the pvcs
	pvcs := createPVCs(ctx, t, cl, sts)
	// create the pvc warning event for one of the pvc
	targetPvc := pvcs[0]
	targetPVName := fmt.Sprintf("pv-%s", testutils.GenerateRandomAlphanumericString(g, 16))
	const eventReason = "FailedAttachVolume"
	eventMessage := fmt.Sprintf("Multi-Attach error for volume %s. Volume is already exclusively attached to one node and can't be attached to another", targetPVName)
	createPVCWarningEvent(ctx, t, cl, testNs, targetPvc.Name, eventReason, eventMessage)
	// ******************************* test etcd status update flow *******************************
	expectedConditions := []druidv1alpha1.Condition{
		{Type: druidv1alpha1.ConditionTypeDataVolumesReady, Status: druidv1alpha1.ConditionFalse, Reason: eventReason, Message: eventMessage},
	}
	assertETCDStatusConditions(t, cl, client.ObjectKeyFromObject(etcd), expectedConditions, 5*time.Minute, 2*time.Second)
}

func testEtcdStatusIsInSyncWithStatefulSetStatusWhenAllReplicasAreReady(t *testing.T, etcd *druidv1alpha1.Etcd, reconcilerTestEnv ReconcilerTestEnv) {
	g := NewWithT(t)
	cl := reconcilerTestEnv.itTestEnv.GetClient()
	ctx := reconcilerTestEnv.itTestEnv.GetContext()
	sts := &appsv1.StatefulSet{}
	g.Expect(cl.Get(ctx, client.ObjectKey{Name: etcd.Name, Namespace: etcd.Namespace}, sts)).To(Succeed())
	stsCopy := sts.DeepCopy()
	stsReplicas := *sts.Spec.Replicas
	stsCopy.Status.ReadyReplicas = stsReplicas
	stsCopy.Status.CurrentReplicas = stsReplicas
	stsCopy.Status.Replicas = stsReplicas
	stsCopy.Status.ObservedGeneration = stsCopy.Generation
	stsCopy.Status.CurrentRevision = fmt.Sprintf("%s-%s", sts.Name, testutils.GenerateRandomAlphanumericString(g, 2))
	stsCopy.Status.UpdateRevision = stsCopy.Status.CurrentRevision
	stsCopy.Status.UpdatedReplicas = stsReplicas
	g.Expect(cl.Status().Update(ctx, stsCopy)).To(Succeed())
	// assert etcd status
	assertETCDStatusFieldsDerivedFromStatefulSet(ctx, t, cl, client.ObjectKeyFromObject(etcd), client.ObjectKeyFromObject(stsCopy), 2*time.Minute, 2*time.Second, true)
}

func testEtcdStatusIsInSyncWithStatefulSetStatusWhenNotAllReplicasAreReady(t *testing.T, etcd *druidv1alpha1.Etcd, reconcilerTestEnv ReconcilerTestEnv) {
	g := NewWithT(t)
	cl := reconcilerTestEnv.itTestEnv.GetClient()
	ctx := reconcilerTestEnv.itTestEnv.GetContext()
	sts := &appsv1.StatefulSet{}
	g.Expect(cl.Get(ctx, client.ObjectKey{Name: etcd.Name, Namespace: etcd.Namespace}, sts)).To(Succeed())
	stsCopy := sts.DeepCopy()
	stsReplicas := *sts.Spec.Replicas
	stsCopy.Status.ReadyReplicas = stsReplicas - 1
	stsCopy.Status.CurrentReplicas = stsReplicas
	stsCopy.Status.Replicas = stsReplicas
	stsCopy.Status.ObservedGeneration = stsCopy.Generation
	stsCopy.Status.CurrentRevision = fmt.Sprintf("%s-%s", sts.Name, testutils.GenerateRandomAlphanumericString(g, 2))
	stsCopy.Status.UpdateRevision = stsCopy.Status.CurrentRevision
	stsCopy.Status.UpdatedReplicas = stsReplicas
	g.Expect(cl.Status().Update(ctx, stsCopy)).To(Succeed())
	// assert etcd status
	assertETCDStatusFieldsDerivedFromStatefulSet(ctx, t, cl, client.ObjectKeyFromObject(etcd), client.ObjectKeyFromObject(stsCopy), 2*time.Minute, 2*time.Second, false)
}

func testEtcdStatusIsInSyncWithStatefulSetStatusWhenCurrentRevisionIsOlderThanUpdateRevision(t *testing.T, etcd *druidv1alpha1.Etcd, reconcilerTestEnv ReconcilerTestEnv) {
	g := NewWithT(t)
	cl := reconcilerTestEnv.itTestEnv.GetClient()
	ctx := reconcilerTestEnv.itTestEnv.GetContext()
	sts := &appsv1.StatefulSet{}
	g.Expect(cl.Get(ctx, client.ObjectKey{Name: etcd.Name, Namespace: etcd.Namespace}, sts)).To(Succeed())
	stsCopy := sts.DeepCopy()
	stsReplicas := *sts.Spec.Replicas
	stsCopy.Status.ReadyReplicas = stsReplicas
	stsCopy.Status.CurrentReplicas = stsReplicas
	stsCopy.Status.Replicas = stsReplicas
	stsCopy.Status.ObservedGeneration = stsCopy.Generation
	stsCopy.Status.CurrentRevision = fmt.Sprintf("%s-%s", sts.Name, testutils.GenerateRandomAlphanumericString(g, 2))
	stsCopy.Status.UpdateRevision = fmt.Sprintf("%s-%s", sts.Name, testutils.GenerateRandomAlphanumericString(g, 2))
	stsCopy.Status.UpdatedReplicas = stsReplicas - 1
	g.Expect(cl.Status().Update(ctx, stsCopy)).To(Succeed())
	// assert etcd status
	assertETCDStatusFieldsDerivedFromStatefulSet(ctx, t, cl, client.ObjectKeyFromObject(etcd), client.ObjectKeyFromObject(stsCopy), 2*time.Minute, 2*time.Second, false)
}
