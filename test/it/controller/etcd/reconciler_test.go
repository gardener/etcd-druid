// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package etcd

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
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
	appsv1 "k8s.io/api/apps/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/component-base/featuregate"
	testclock "k8s.io/utils/clock/testing"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const testNamespacePrefix = "etcd-reconciler-test-"

var (
	sharedITTestEnv setup.IntegrationTestEnv
)

func TestMain(m *testing.M) {
	var (
		itTestEnvCloser setup.IntegrationTestEnvCloser
		err             error
	)
	sharedITTestEnv, itTestEnvCloser, err = setup.NewIntegrationTestEnv("etcd-reconciler", []string{assets.GetEtcdCrdPath()})
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to create integration test environment: %v\n", err)
		os.Exit(1)
	}
	defer itTestEnvCloser()
	os.Exit(m.Run())
}

// ------------------------------ reconcile spec tests ------------------------------
func TestEtcdReconcileSpecWithNoAutoReconcile(t *testing.T) {
	reconcilerTestEnv := initializeEtcdReconcilerTestEnv(t, sharedITTestEnv, false, testutils.NewTestClientBuilder())
	tests := []struct {
		name string
		fn   func(t *testing.T, testNamespace string, reconcilerTestEnv ReconcilerTestEnv)
	}{
		{"should create all managed resources when etcd resource is created", testAllManagedResourcesAreCreated},
		{"should succeed only in creation of some resources and not all and should record error in lastErrors and lastOperation", testFailureToCreateAllResources},
		{"should not reconcile spec when reconciliation is suspended", testWhenReconciliationIsSuspended},
		{"should not reconcile when no reconcile operation annotation is set", testWhenNoReconcileOperationAnnotationIsSet},
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
	assertETCDLastOperationAndLastErrorsUpdatedSuccessfully(t, cl, client.ObjectKeyFromObject(etcdInstance), expectedLastOperation, []string{"ERR_SYNC_CLIENT_SERVICE"}, 5*time.Second, 1*time.Second)
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
	reconcilerTestEnv := initializeEtcdReconcilerTestEnv(t, sharedITTestEnv, false, testutils.NewTestClientBuilder())
	// ---------------------------- create test namespace ---------------------------
	t.Logf("successfully create namespace: %s to run test => '%s'", testNs, t.Name())
	g.Expect(sharedITTestEnv.CreateTestNamespace(testNs)).To(Succeed())
	// ---------------------------- create etcd instance --------------------------
	etcdInstance := testutils.EtcdBuilderWithoutDefaults(testutils.TestEtcdName, testNs).
		WithClientTLS().
		WithPeerTLS().
		WithReplicas(3).
		WithAnnotations(map[string]string{
			v1beta1constants.GardenerOperation: v1beta1constants.GardenerOperationReconcile,
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

	g := NewWithT(t)

	// A different IT test environment is required due to a different clientBuilder which is used to create the manager.
	itTestEnv, itTestEnvCloser, err := setup.NewIntegrationTestEnv("etcd-reconciler", []string{assets.GetEtcdCrdPath()})
	g.Expect(err).ToNot(HaveOccurred())
	defer itTestEnvCloser()
	reconcilerTestEnv := initializeEtcdReconcilerTestEnv(t, itTestEnv, false, testClientBuilder)

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
	assertETCDLastOperationAndLastErrorsUpdatedSuccessfully(t, cl, client.ObjectKeyFromObject(etcdInstance), expectedLastOperation, []string{"ERR_DELETE_CLIENT_SERVICE", "ERR_DELETE_SNAPSHOT_LEASE"}, 2*time.Minute, 2*time.Second)
	// assert that the finalizer has not been removed as all resources have not been deleted yet.
	assertETCDFinalizer(t, cl, client.ObjectKeyFromObject(etcdInstance), true, 10*time.Second, 2*time.Second)
}

// ------------------------------ reconcile status tests ------------------------------

func TestEtcdStatusReconciliation(t *testing.T) {
	reconcilerTestEnv := initializeEtcdReconcilerTestEnv(t, sharedITTestEnv, false, testutils.NewTestClientBuilder())
	tests := []struct {
		name string
		fn   func(t *testing.T, etcd *druidv1alpha1.Etcd, reconcilerTestEnv ReconcilerTestEnv)
	}{
		{"check assertions when all member leases are active", testConditionsAndMembersWhenAllMemberLeasesAreActive},
		{"assert that data volume condition reflects pvc error event", testEtcdStatusReflectsPVCErrorEvent},
		{"test when all sts replicas are ready", testEtcdStatusIsInSyncWithStatefulSetStatusWhenAllReplicasAreReady},
		{"test when not all sts replicas are ready", testEtcdStatusIsInSyncWithStatefulSetStatusWhenNotAllReplicasAreReady},
		{"test when sts current revision is older than update revision", testEtcdStatusIsInSyncWithStatefulSetStatusWhenCurrentRevisionIsOlderThanUpdateRevision},
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
					v1beta1constants.GardenerOperation: v1beta1constants.GardenerOperationReconcile,
				}).Build()
			createAndAssertEtcdAndAllManagedResources(ctx, t, reconcilerTestEnv, etcdInstance)
			test.fn(t, etcdInstance, reconcilerTestEnv)
		})
	}
}

func testConditionsAndMembersWhenAllMemberLeasesAreActive(t *testing.T, etcd *druidv1alpha1.Etcd, reconcilerTestEnv ReconcilerTestEnv) {
	memberLeaseNames := etcd.GetMemberLeaseNames()
	testNs := etcd.Namespace
	clock := testclock.NewFakeClock(time.Now().Round(time.Second))
	mlcs := []etcdMemberLeaseConfig{
		{name: memberLeaseNames[0], memberID: generateRandomAlphanumericString(t, 8), role: druidv1alpha1.EtcdRoleMember, renewTime: &metav1.MicroTime{Time: clock.Now().Add(-time.Second * 30)}},
		{name: memberLeaseNames[1], memberID: generateRandomAlphanumericString(t, 8), role: druidv1alpha1.EtcdRoleLeader, renewTime: &metav1.MicroTime{Time: clock.Now()}},
		{name: memberLeaseNames[2], memberID: generateRandomAlphanumericString(t, 8), role: druidv1alpha1.EtcdRoleMember, renewTime: &metav1.MicroTime{Time: clock.Now().Add(-time.Second * 30)}},
	}
	updateMemberLeaseSpec(context.Background(), t, reconcilerTestEnv.itTestEnv.GetClient(), testNs, mlcs)
	// ******************************* test etcd status update flow *******************************
	expectedConditions := []druidv1alpha1.Condition{
		{Type: druidv1alpha1.ConditionTypeReady, Status: druidv1alpha1.ConditionTrue},
		{Type: druidv1alpha1.ConditionTypeAllMembersReady, Status: druidv1alpha1.ConditionTrue},
		{Type: druidv1alpha1.ConditionTypeDataVolumesReady, Status: druidv1alpha1.ConditionTrue},
	}
	cl := reconcilerTestEnv.itTestEnv.GetClient()
	assertETCDStatusConditions(t, cl, client.ObjectKeyFromObject(etcd), expectedConditions, 2*time.Minute, 2*time.Second)
	expectedMemberStatuses := []druidv1alpha1.EtcdMemberStatus{
		{Name: mlcs[0].name, ID: pointer.String(mlcs[0].memberID), Role: &mlcs[0].role, Status: druidv1alpha1.EtcdMemberStatusReady},
		{Name: mlcs[1].name, ID: pointer.String(mlcs[1].memberID), Role: &mlcs[1].role, Status: druidv1alpha1.EtcdMemberStatusReady},
		{Name: mlcs[2].name, ID: pointer.String(mlcs[2].memberID), Role: &mlcs[2].role, Status: druidv1alpha1.EtcdMemberStatusReady},
	}
	assertETCDMemberStatuses(t, cl, client.ObjectKeyFromObject(etcd), expectedMemberStatuses, 2*time.Minute, 2*time.Second)
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
	targetPVName := fmt.Sprintf("pv-%s", generateRandomAlphanumericString(t, 16))
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
	stsCopy.Status.CurrentRevision = fmt.Sprintf("%s-%s", sts.Name, generateRandomAlphanumericString(t, 2))
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
	stsCopy.Status.CurrentRevision = fmt.Sprintf("%s-%s", sts.Name, generateRandomAlphanumericString(t, 2))
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
	stsCopy.Status.CurrentRevision = fmt.Sprintf("%s-%s", sts.Name, generateRandomAlphanumericString(t, 2))
	stsCopy.Status.UpdateRevision = fmt.Sprintf("%s-%s", sts.Name, generateRandomAlphanumericString(t, 2))
	stsCopy.Status.UpdatedReplicas = stsReplicas - 1
	g.Expect(cl.Status().Update(ctx, stsCopy)).To(Succeed())
	// assert etcd status
	assertETCDStatusFieldsDerivedFromStatefulSet(ctx, t, cl, client.ObjectKeyFromObject(etcd), client.ObjectKeyFromObject(stsCopy), 2*time.Minute, 2*time.Second, false)
}

// -------------------------  Helper functions  -------------------------
func createTestNamespaceName(t *testing.T) string {
	namespaceSuffix := generateRandomAlphanumericString(t, 4)
	return fmt.Sprintf("%s-%s", testNamespacePrefix, namespaceSuffix)
}

func generateRandomAlphanumericString(t *testing.T, length int) string {
	b := make([]byte, length)
	_, err := rand.Read(b)
	g := NewWithT(t)
	g.Expect(err).ToNot(HaveOccurred())
	return hex.EncodeToString(b)
}

func initializeEtcdReconcilerTestEnv(t *testing.T, itTestEnv setup.IntegrationTestEnv, autoReconcile bool, clientBuilder *testutils.TestClientBuilder) ReconcilerTestEnv {
	g := NewWithT(t)
	var (
		reconciler *etcd.Reconciler
		err        error
	)
	g.Expect(itTestEnv.CreateManager(clientBuilder)).To(Succeed())
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
				EtcdMember: etcd.MemberConfig{
					NotReadyThreshold: 5 * time.Minute,
					UnknownThreshold:  1 * time.Minute,
				},
			}, assets.CreateImageVector(g))
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(reconciler.RegisterWithManager(mgr)).To(Succeed())
	})
	g.Expect(itTestEnv.StartManager()).To(Succeed())
	t.Log("successfully registered etcd reconciler with manager and started manager")
	return ReconcilerTestEnv{
		itTestEnv:  itTestEnv,
		reconciler: reconciler,
	}
}

func createAndAssertEtcdAndAllManagedResources(ctx context.Context, t *testing.T, reconcilerTestEnv ReconcilerTestEnv, etcdInstance *druidv1alpha1.Etcd) {
	g := NewWithT(t)
	cl := reconcilerTestEnv.itTestEnv.GetClient()
	// create etcdInstance resource
	g.Expect(cl.Create(ctx, etcdInstance)).To(Succeed())
	t.Logf("trigggered creation of etcd instance: {name: %s, namespace: %s}, waiting for resources to be created...", etcdInstance.Name, etcdInstance.Namespace)
	// ascertain that all etcd resources are created
	assertAllComponentsExists(ctx, t, reconcilerTestEnv, etcdInstance, 3*time.Minute, 2*time.Second)
	t.Logf("successfully created all resources for etcd instance: {name: %s, namespace: %s}", etcdInstance.Name, etcdInstance.Namespace)
	// add finalizer
	addFinalizer(ctx, g, cl, client.ObjectKeyFromObject(etcdInstance))
	t.Logf("successfully added finalizer to etcd instance: {name: %s, namespace: %s}", etcdInstance.Name, etcdInstance.Namespace)
}

func addFinalizer(ctx context.Context, g *WithT, cl client.Client, etcdObjectKey client.ObjectKey) {
	etcdInstance := &druidv1alpha1.Etcd{}
	g.Expect(cl.Get(ctx, etcdObjectKey, etcdInstance)).To(Succeed())
	g.Expect(controllerutils.AddFinalizers(ctx, cl, etcdInstance, common.FinalizerName)).To(Succeed())
}

type etcdMemberLeaseConfig struct {
	name      string
	memberID  string
	role      druidv1alpha1.EtcdRole
	renewTime *metav1.MicroTime
}

func updateMemberLeaseSpec(ctx context.Context, t *testing.T, cl client.Client, namespace string, memberLeaseConfigs []etcdMemberLeaseConfig) {
	g := NewWithT(t)
	for _, config := range memberLeaseConfigs {
		lease := &coordinationv1.Lease{}
		g.Expect(cl.Get(ctx, client.ObjectKey{Name: config.name, Namespace: namespace}, lease)).To(Succeed())
		updatedLease := lease.DeepCopy()
		updatedLease.Spec.HolderIdentity = pointer.String(fmt.Sprintf("%s:%s", config.memberID, config.role))
		updatedLease.Spec.RenewTime = config.renewTime
		g.Expect(cl.Update(ctx, updatedLease)).To(Succeed())
		t.Logf("successfully updated member lease %s with holderIdentity: %s", config.name, *updatedLease.Spec.HolderIdentity)
	}
}

func createPVCs(ctx context.Context, t *testing.T, cl client.Client, sts *appsv1.StatefulSet) []*corev1.PersistentVolumeClaim {
	g := NewWithT(t)
	pvcs := make([]*corev1.PersistentVolumeClaim, 0, int(*sts.Spec.Replicas))
	volClaimName := sts.Spec.VolumeClaimTemplates[0].Name
	for i := 0; i < int(*sts.Spec.Replicas); i++ {
		pvc := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%s-%s-%d", volClaimName, sts.Name, i),
				Namespace: sts.Namespace,
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse("1Gi"),
					},
				},
			},
		}
		g.Expect(cl.Create(ctx, pvc)).To(Succeed())
		pvcs = append(pvcs, pvc)
		t.Logf("successfully created pvc: %s", pvc.Name)
	}
	return pvcs
}

func createPVCWarningEvent(ctx context.Context, t *testing.T, cl client.Client, namespace, pvcName, reason, message string) {
	g := NewWithT(t)
	event := &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("%s-event-", pvcName),
			Namespace:    namespace,
		},
		InvolvedObject: corev1.ObjectReference{
			Kind:       "PersistentVolumeClaim",
			Name:       pvcName,
			Namespace:  namespace,
			APIVersion: "v1",
		},
		Reason:  reason,
		Message: message,
		Type:    corev1.EventTypeWarning,
	}
	g.Expect(cl.Create(ctx, event)).To(Succeed())
	t.Logf("successfully created warning event for pvc: %s", pvcName)
}
