// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package etcd

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"
	"github.com/gardener/etcd-druid/internal/component"
	"github.com/gardener/etcd-druid/internal/controller/etcd"
	"github.com/gardener/etcd-druid/internal/utils"
	"github.com/gardener/etcd-druid/test/it/setup"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	allComponentKinds = []component.Kind{
		component.MemberLeaseKind,
		component.SnapshotLeaseKind,
		component.ClientServiceKind,
		component.PeerServiceKind,
		component.ConfigMapKind,
		component.PodDisruptionBudgetKind,
		component.ServiceAccountKind,
		component.RoleKind,
		component.RoleBindingKind,
		component.StatefulSetKind,
	}
)

type componentCreatedAssertionFn func(ctx component.OperatorContext, t *testing.T, opRegistry component.Registry, etcd *druidv1alpha1.Etcd, timeout, pollInterval time.Duration)

// ReconcilerTestEnv represents the test environment for the etcd reconciler.
type ReconcilerTestEnv struct {
	itTestEnv  setup.IntegrationTestEnv
	reconciler *etcd.Reconciler
}

func getKindToComponentCreatedAssertionFns() map[component.Kind]componentCreatedAssertionFn {
	return map[component.Kind]componentCreatedAssertionFn{
		component.MemberLeaseKind:         assertMemberLeasesCreated,
		component.SnapshotLeaseKind:       assertSnapshotLeasesCreated,
		component.ClientServiceKind:       assertClientServiceCreated,
		component.PeerServiceKind:         assertPeerServiceCreated,
		component.ConfigMapKind:           assertConfigMapCreated,
		component.PodDisruptionBudgetKind: assertPDBCreated,
		component.ServiceAccountKind:      assertServiceAccountCreated,
		component.RoleKind:                assertRoleCreated,
		component.RoleBindingKind:         assertRoleBindingCreated,
		component.StatefulSetKind:         assertStatefulSetCreated,
	}
}

func assertNoComponentsExist(ctx context.Context, t *testing.T, rtEnv ReconcilerTestEnv, etcd *druidv1alpha1.Etcd, timeout, pollInterval time.Duration) {
	assertComponentsDoNotExist(ctx, t, rtEnv, etcd, allComponentKinds, timeout, pollInterval)
}

// assertAllComponentsExists asserts that all components of the etcd resource are created successfully eventually.
func assertAllComponentsExists(ctx context.Context, t *testing.T, rtEnv ReconcilerTestEnv, etcd *druidv1alpha1.Etcd, timeout, pollInterval time.Duration) {
	kindToAssertionFns := getKindToComponentCreatedAssertionFns()
	assertFns := make([]componentCreatedAssertionFn, 0, len(kindToAssertionFns))
	for _, assertFn := range kindToAssertionFns {
		assertFns = append(assertFns, assertFn)
	}
	doAssertComponentsExist(ctx, t, rtEnv, etcd, assertFns, timeout, pollInterval)
}

func assertSelectedComponentsExists(ctx context.Context, t *testing.T, rtEnv ReconcilerTestEnv, etcd *druidv1alpha1.Etcd, componentKinds []component.Kind, timeout, pollInterval time.Duration) {
	g := NewWithT(t)
	assertFns := make([]componentCreatedAssertionFn, 0, len(componentKinds))
	for _, kind := range componentKinds {
		assertFn, ok := getKindToComponentCreatedAssertionFns()[kind]
		g.Expect(ok).To(BeTrue(), fmt.Sprintf("assertion function for %s not found", kind))
		assertFns = append(assertFns, assertFn)
	}
	doAssertComponentsExist(ctx, t, rtEnv, etcd, assertFns, timeout, pollInterval)
}

func assertComponentsDoNotExist(ctx context.Context, t *testing.T, rtEnv ReconcilerTestEnv, etcd *druidv1alpha1.Etcd, componentKinds []component.Kind, timeout, pollInterval time.Duration) {
	opRegistry := rtEnv.reconciler.GetOperatorRegistry()
	opCtx := component.NewOperatorContext(ctx, rtEnv.itTestEnv.GetLogger(), t.Name())
	for _, kind := range componentKinds {
		assertResourceCreation(opCtx, t, opRegistry, kind, etcd, []string{}, timeout, pollInterval)
	}
}

func doAssertComponentsExist(ctx context.Context, t *testing.T, rtEnv ReconcilerTestEnv, etcd *druidv1alpha1.Etcd, assertionFns []componentCreatedAssertionFn, timeout, pollInterval time.Duration) {
	opRegistry := rtEnv.reconciler.GetOperatorRegistry()
	opCtx := component.NewOperatorContext(ctx, rtEnv.itTestEnv.GetLogger(), t.Name())
	wg := sync.WaitGroup{}
	wg.Add(len(assertionFns))
	for _, assertFn := range assertionFns {
		assertFn := assertFn
		go func() {
			defer wg.Done()
			assertFn(opCtx, t, opRegistry, etcd, timeout, pollInterval)
		}()
	}
	wg.Wait()
}

func assertResourceCreation(ctx component.OperatorContext, t *testing.T, opRegistry component.Registry, kind component.Kind, etcd *druidv1alpha1.Etcd, expectedResourceNames []string, timeout, pollInterval time.Duration) {
	g := NewWithT(t)
	op := opRegistry.GetOperator(kind)
	checkFn := func() error {
		actualResourceNames, err := op.GetExistingResourceNames(ctx, etcd.ObjectMeta)
		if err != nil {
			return err
		}
		if len(actualResourceNames) > len(expectedResourceNames) {
			return fmt.Errorf("expected only %d %s, found %v", len(expectedResourceNames), kind, actualResourceNames)
		}
		slices.Sort(actualResourceNames)
		slices.Sort(expectedResourceNames)
		if !slices.Equal(actualResourceNames, expectedResourceNames) {
			return fmt.Errorf("expected %s: %v, found %v instead", kind, expectedResourceNames, actualResourceNames)
		}
		msg := utils.IfConditionOr(len(expectedResourceNames) == 0,
			fmt.Sprintf("%s: %v does not exist", kind, expectedResourceNames),
			fmt.Sprintf("%s: %v exists", kind, expectedResourceNames))
		t.Log(msg)
		return nil
	}
	g.Eventually(checkFn).Within(timeout).WithPolling(pollInterval).WithContext(ctx).Should(BeNil())
}

func assertMemberLeasesCreated(ctx component.OperatorContext, t *testing.T, opRegistry component.Registry, etcd *druidv1alpha1.Etcd, timeout, pollInterval time.Duration) {
	expectedMemberLeaseNames := make([]string, 0, etcd.Spec.Replicas)
	for i := 0; i < int(etcd.Spec.Replicas); i++ {
		expectedMemberLeaseNames = append(expectedMemberLeaseNames, fmt.Sprintf("%s-%d", etcd.Name, i))
	}
	assertResourceCreation(ctx, t, opRegistry, component.MemberLeaseKind, etcd, expectedMemberLeaseNames, timeout, pollInterval)
}

func assertSnapshotLeasesCreated(ctx component.OperatorContext, t *testing.T, opRegistry component.Registry, etcd *druidv1alpha1.Etcd, timeout, pollInterval time.Duration) {
	expectedSnapshotLeaseNames := make([]string, 0, 2)
	if etcd.IsBackupStoreEnabled() {
		expectedSnapshotLeaseNames = []string{druidv1alpha1.GetDeltaSnapshotLeaseName(etcd.ObjectMeta), druidv1alpha1.GetFullSnapshotLeaseName(etcd.ObjectMeta)}
	}
	assertResourceCreation(ctx, t, opRegistry, component.SnapshotLeaseKind, etcd, expectedSnapshotLeaseNames, timeout, pollInterval)
}

func assertClientServiceCreated(ctx component.OperatorContext, t *testing.T, opRegistry component.Registry, etcd *druidv1alpha1.Etcd, timeout, pollInterval time.Duration) {
	expectedClientServiceNames := []string{druidv1alpha1.GetClientServiceName(etcd.ObjectMeta)}
	assertResourceCreation(ctx, t, opRegistry, component.ClientServiceKind, etcd, expectedClientServiceNames, timeout, pollInterval)
}

func assertPeerServiceCreated(ctx component.OperatorContext, t *testing.T, opRegistry component.Registry, etcd *druidv1alpha1.Etcd, timeout, pollInterval time.Duration) {
	expectedPeerServiceNames := []string{druidv1alpha1.GetPeerServiceName(etcd.ObjectMeta)}
	assertResourceCreation(ctx, t, opRegistry, component.PeerServiceKind, etcd, expectedPeerServiceNames, timeout, pollInterval)
}

func assertConfigMapCreated(ctx component.OperatorContext, t *testing.T, opRegistry component.Registry, etcd *druidv1alpha1.Etcd, timeout, pollInterval time.Duration) {
	expectedConfigMapNames := []string{druidv1alpha1.GetConfigMapName(etcd.ObjectMeta)}
	assertResourceCreation(ctx, t, opRegistry, component.ConfigMapKind, etcd, expectedConfigMapNames, timeout, pollInterval)
}

func assertPDBCreated(ctx component.OperatorContext, t *testing.T, opRegistry component.Registry, etcd *druidv1alpha1.Etcd, timeout, pollInterval time.Duration) {
	expectedPDBNames := []string{etcd.Name}
	assertResourceCreation(ctx, t, opRegistry, component.PodDisruptionBudgetKind, etcd, expectedPDBNames, timeout, pollInterval)
}

func assertServiceAccountCreated(ctx component.OperatorContext, t *testing.T, opRegistry component.Registry, etcd *druidv1alpha1.Etcd, timeout, pollInterval time.Duration) {
	expectedServiceAccountNames := []string{druidv1alpha1.GetServiceAccountName(etcd.ObjectMeta)}
	assertResourceCreation(ctx, t, opRegistry, component.ServiceAccountKind, etcd, expectedServiceAccountNames, timeout, pollInterval)
}

func assertRoleCreated(ctx component.OperatorContext, t *testing.T, opRegistry component.Registry, etcd *druidv1alpha1.Etcd, timeout, pollInterval time.Duration) {
	expectedRoleNames := []string{druidv1alpha1.GetRoleName(etcd.ObjectMeta)}
	assertResourceCreation(ctx, t, opRegistry, component.RoleKind, etcd, expectedRoleNames, timeout, pollInterval)
}

func assertRoleBindingCreated(ctx component.OperatorContext, t *testing.T, opRegistry component.Registry, etcd *druidv1alpha1.Etcd, timeout, pollInterval time.Duration) {
	expectedRoleBindingNames := []string{druidv1alpha1.GetRoleBindingName(etcd.ObjectMeta)}
	assertResourceCreation(ctx, t, opRegistry, component.RoleBindingKind, etcd, expectedRoleBindingNames, timeout, pollInterval)
}

func assertStatefulSetCreated(ctx component.OperatorContext, t *testing.T, opRegistry component.Registry, etcd *druidv1alpha1.Etcd, timeout, pollInterval time.Duration) {
	expectedSTSNames := []string{etcd.Name}
	assertResourceCreation(ctx, t, opRegistry, component.StatefulSetKind, etcd, expectedSTSNames, timeout, pollInterval)
}

func assertStatefulSetGeneration(ctx context.Context, t *testing.T, cl client.Client, stsObjectKey client.ObjectKey, expectedGeneration int64, timeout, pollInterval time.Duration) {
	g := NewWithT(t)
	checkFn := func() error {
		sts := &appsv1.StatefulSet{}
		if err := cl.Get(ctx, stsObjectKey, sts); err != nil {
			return err
		}
		if sts.Generation != expectedGeneration {
			return fmt.Errorf("expected sts generation to be %d, got %d", expectedGeneration, sts.Generation)
		}
		return nil
	}
	g.Eventually(checkFn).Within(timeout).WithPolling(pollInterval).WithContext(ctx).Should(BeNil())
}

func assertETCDObservedGeneration(t *testing.T, cl client.Client, etcdObjectKey client.ObjectKey, expectedObservedGeneration *int64, timeout, pollInterval time.Duration) {
	g := NewWithT(t)
	checkFn := func() error {
		etcdInstance := &druidv1alpha1.Etcd{}
		err := cl.Get(context.Background(), etcdObjectKey, etcdInstance)
		if err != nil {
			return err
		}
		if expectedObservedGeneration == nil {
			if etcdInstance.Status.ObservedGeneration != nil {
				return fmt.Errorf("expected observedGeneration to be nil, found %v", etcdInstance.Status.ObservedGeneration)
			}
			return nil
		} else {
			if etcdInstance.Status.ObservedGeneration == nil {
				return fmt.Errorf("expected observedGeneration to be %v, found nil", *expectedObservedGeneration)
			}
			if *etcdInstance.Status.ObservedGeneration != *expectedObservedGeneration {
				return fmt.Errorf("expected observedGeneration to be %d, found %d", *expectedObservedGeneration, *etcdInstance.Status.ObservedGeneration)
			}
		}
		return nil
	}
	g.Eventually(checkFn).Within(timeout).WithPolling(pollInterval).Should(BeNil())
	t.Logf("observedGeneration correctly set to %s", logPointerTypeToString[int64](expectedObservedGeneration))
}

func assertETCDLastOperationAndLastErrorsUpdatedSuccessfully(t *testing.T, cl client.Client, etcdObjectKey client.ObjectKey, expectedLastOperation *druidv1alpha1.LastOperation, expectedLastErrorCodes []string, timeout, pollInterval time.Duration) {
	g := NewWithT(t)
	checkFn := func() error {
		etcdInstance := &druidv1alpha1.Etcd{}
		err := cl.Get(context.Background(), etcdObjectKey, etcdInstance)
		if err != nil {
			return err
		}
		actualLastOperation := etcdInstance.Status.LastOperation
		if actualLastOperation == nil {
			if expectedLastOperation != nil {
				return fmt.Errorf("expected lastOperation to be %s, found nil", expectedLastOperation)
			}
			return nil
		}
		if actualLastOperation.Type != expectedLastOperation.Type &&
			etcdInstance.Status.LastOperation.State != expectedLastOperation.State {
			return fmt.Errorf("expected lastOperation to be %s, found %s", expectedLastOperation, etcdInstance.Status.LastOperation)
		}

		// For comparing last errors, it is sufficient to compare their length and their error codes.
		slices.Sort(expectedLastErrorCodes)
		actualErrorCodes := getErrorCodesFromLastErrors(etcdInstance.Status.LastErrors)
		slices.Sort(actualErrorCodes)

		if !slices.Equal(expectedLastErrorCodes, actualErrorCodes) {
			return fmt.Errorf("expected lastErrors to be %v, found %v", expectedLastErrorCodes, actualErrorCodes)
		}
		return nil
	}
	g.Eventually(checkFn).Within(timeout).WithPolling(pollInterval).Should(BeNil())
	t.Log("lastOperation and lastErrors updated successfully")
}

func assertETCDOperationAnnotation(t *testing.T, cl client.Client, etcdObjectKey client.ObjectKey, expectedAnnotationToBePresent bool, timeout, pollInterval time.Duration) {
	g := NewWithT(t)
	checkFn := func() error {
		etcdInstance := &druidv1alpha1.Etcd{}
		err := cl.Get(context.Background(), etcdObjectKey, etcdInstance)
		if err != nil {
			return err
		}
		if metav1.HasAnnotation(etcdInstance.ObjectMeta, v1beta1constants.GardenerOperation) != expectedAnnotationToBePresent {
			return fmt.Errorf("expected reconcile operation annotation to be removed, found %v", v1beta1constants.GardenerOperation)
		}
		return nil
	}
	g.Eventually(checkFn).Within(timeout).WithPolling(pollInterval).Should(BeNil())
	msg := utils.IfConditionOr(expectedAnnotationToBePresent, "reconcile operation annotation present", "reconcile operation annotation removed")
	t.Log(msg)
}

func assertReconcileSuspensionEventRecorded(ctx context.Context, t *testing.T, cl client.Client, etcdObjectKey client.ObjectKey, timeout, pollInterval time.Duration) {
	g := NewWithT(t)
	checkFn := func() error {
		events := &corev1.EventList{}
		if err := cl.List(ctx, events, client.InNamespace(etcdObjectKey.Namespace), client.MatchingFields{"involvedObject.name": etcdObjectKey.Name, "type": "Warning"}); err != nil {
			return err
		}
		if len(events.Items) != 1 {
			return fmt.Errorf("expected 1 event, found %d", len(events.Items))
		}
		if events.Items[0].Reason != "SpecReconciliationSkipped" {
			return fmt.Errorf("expected event reason to be SpecReconciliationSkipped, found %s", events.Items[0].Reason)
		}
		return nil
	}
	g.Eventually(checkFn).Within(timeout).WithPolling(pollInterval).Should(BeNil())
}

func assertETCDFinalizer(t *testing.T, cl client.Client, etcdObjectKey client.ObjectKey, expectedFinalizerPresent bool, timeout, pollInterval time.Duration) {
	g := NewWithT(t)
	checkFn := func() error {
		etcdInstance := &druidv1alpha1.Etcd{}
		if err := cl.Get(context.Background(), etcdObjectKey, etcdInstance); err != nil {
			if apierrors.IsNotFound(err) {
				// Once the finalizer is removed, the etcd resource will be removed by k8s very quickly.
				// If we find that the resource was indeed removed then this check will pass.
				return nil
			}
			return err
		}
		finalizerPresent := slices.Contains(etcdInstance.ObjectMeta.Finalizers, common.FinalizerName)
		if expectedFinalizerPresent != finalizerPresent {
			return fmt.Errorf("expected finalizer to be %s, found %v", utils.IfConditionOr(expectedFinalizerPresent, "present", "removed"), etcdInstance.ObjectMeta.Finalizers)
		}
		return nil
	}
	g.Eventually(checkFn).Within(timeout).WithPolling(pollInterval).Should(BeNil())
	msg := utils.IfConditionOr(expectedFinalizerPresent, "finalizer present", "finalizer removed")
	t.Log(msg)
}

func assertETCDMemberStatuses(t *testing.T, cl client.Client, etcdObjectKey client.ObjectKey, expectedMembers []druidv1alpha1.EtcdMemberStatus, timeout, pollInterval time.Duration) {
	g := NewWithT(t)
	checkFn := func() error {
		etcdInstance := &druidv1alpha1.Etcd{}
		if err := cl.Get(context.Background(), etcdObjectKey, etcdInstance); err != nil {
			return err
		}
		actualMembers := etcdInstance.Status.Members
		if len(actualMembers) != len(expectedMembers) {
			return fmt.Errorf("expected %d members, found %d", len(expectedMembers), len(actualMembers))
		}
		var memberErr error
		for _, expectedMember := range expectedMembers {
			foundMember, err := findMatchingMemberStatus(actualMembers, expectedMember)
			if err != nil {
				memberErr = errors.Join(memberErr, err)
			} else {
				t.Logf("member with [id:%s, name:%s, role:%s, status:%s] matches expected member", logPointerTypeToString(foundMember.ID), foundMember.Name, logPointerTypeToString(foundMember.Role), foundMember.Status)
			}
		}
		return memberErr
	}
	g.Eventually(checkFn).Within(timeout).WithPolling(pollInterval).Should(BeNil())
	t.Logf("asserted that etcd member statuses matches expected members: %v", expectedMembers)
}

func assertETCDStatusConditions(t *testing.T, cl client.Client, etcdObjectKey client.ObjectKey, expectedConditions []druidv1alpha1.Condition, timeout, pollInterval time.Duration) {
	g := NewWithT(t)
	checkFn := func() error {
		etcdInstance := &druidv1alpha1.Etcd{}
		if err := cl.Get(context.Background(), etcdObjectKey, etcdInstance); err != nil {
			return err
		}
		actualConditions := etcdInstance.Status.Conditions
		var condErr error
		for _, expectedCondition := range expectedConditions {
			foundCondition, err := findMatchingCondition(actualConditions, expectedCondition)
			if err != nil {
				condErr = errors.Join(condErr, err)
			} else {
				t.Logf("found condition with [type:%s, status:%s] matches expected condition", foundCondition.Type, foundCondition.Status)
			}
		}
		if condErr != nil {
			t.Logf("not all conditions matched: %v. will retry matching all expected conditions", condErr)
		}
		return condErr
	}
	g.Eventually(checkFn).Within(timeout).WithPolling(pollInterval).Should(BeNil())
	t.Logf("asserted that etcd status conditions matches expected conditions: %v", expectedConditions)
}

func assertETCDStatusFieldsDerivedFromStatefulSet(ctx context.Context, t *testing.T, cl client.Client, etcdObjectKey client.ObjectKey, stsObjectKey client.ObjectKey, timeout, pollInterval time.Duration, expectedReadyStatus bool) {
	g := NewWithT(t)
	checkFn := func() error {
		sts := &appsv1.StatefulSet{}
		if err := cl.Get(ctx, stsObjectKey, sts); err != nil {
			return err
		}
		etcdInstance := &druidv1alpha1.Etcd{}
		if err := cl.Get(context.Background(), etcdObjectKey, etcdInstance); err != nil {
			return err
		}
		if etcdInstance.Status.ReadyReplicas != sts.Status.ReadyReplicas {
			return fmt.Errorf("expected readyReplicas to be %d, found %d", sts.Status.ReadyReplicas, etcdInstance.Status.ReadyReplicas)
		}
		if etcdInstance.Status.CurrentReplicas != sts.Status.CurrentReplicas {
			return fmt.Errorf("expected currentReplicas to be %d, found %d", sts.Status.CurrentReplicas, etcdInstance.Status.CurrentReplicas)
		}
		if etcdInstance.Status.Replicas != *sts.Spec.Replicas {
			return fmt.Errorf("expected replicas to be %d, found %d", sts.Spec.Replicas, etcdInstance.Status.Replicas)
		}
		if utils.TypeDeref(etcdInstance.Status.Ready, false) != expectedReadyStatus {
			return fmt.Errorf("expected ready to be %t, found %s", expectedReadyStatus, logPointerTypeToString[bool](etcdInstance.Status.Ready))
		}
		return nil
	}
	g.Eventually(checkFn).WithContext(ctx).Within(timeout).WithPolling(pollInterval).Should(BeNil())
	t.Logf("asserted that etcd status fields are correctly derived from statefulset: %s", stsObjectKey.Name)
}

func getErrorCodesFromLastErrors(lastErrors []druidv1alpha1.LastError) []string {
	errorCodes := make([]string, 0, len(lastErrors))
	for _, lastErr := range lastErrors {
		errorCodes = append(errorCodes, string(lastErr.Code))
	}
	return errorCodes
}

func logPointerTypeToString[T any](val *T) string {
	if val == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%v", *val)
}

func findMatchingCondition(actualConditions []druidv1alpha1.Condition, conditionToMatch druidv1alpha1.Condition) (*druidv1alpha1.Condition, error) {
	for _, actualCondition := range actualConditions {
		if actualCondition.Type == conditionToMatch.Type {
			if actualCondition.Status != conditionToMatch.Status {
				return nil, fmt.Errorf("for condition type: %s, expected status to be %s, found %s", conditionToMatch.Type, conditionToMatch.Status, actualCondition.Status)
			}
			return &actualCondition, nil
		}
	}
	return nil, fmt.Errorf("condition type: %s not found", conditionToMatch.Type)
}

func findMatchingMemberStatus(actualMembers []druidv1alpha1.EtcdMemberStatus, memberStatusToMatch druidv1alpha1.EtcdMemberStatus) (*druidv1alpha1.EtcdMemberStatus, error) {
	for _, actualMember := range actualMembers {
		if *actualMember.ID == *memberStatusToMatch.ID {
			if actualMember.Name != memberStatusToMatch.Name {
				return nil, fmt.Errorf("for member with id: %s, expected name to be %s, found %s", logPointerTypeToString(actualMember.ID), memberStatusToMatch.Name, actualMember.Name)
			}
			if *actualMember.Role != *memberStatusToMatch.Role {
				return nil, fmt.Errorf("for member with id: %s, expected role to be %s, found %s", logPointerTypeToString(actualMember.ID), logPointerTypeToString(memberStatusToMatch.Role), logPointerTypeToString(actualMember.Role))
			}
			if actualMember.Status != memberStatusToMatch.Status {
				return nil, fmt.Errorf("for member with id: %s, expected status to be %s, found %s", logPointerTypeToString(actualMember.ID), memberStatusToMatch.Status, actualMember.Status)
			}
			return &actualMember, nil
		}
	}
	return nil, fmt.Errorf("member with id: %s not found", logPointerTypeToString(memberStatusToMatch.ID))
}
