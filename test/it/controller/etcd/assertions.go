package etcd

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/controller/etcd"
	"github.com/gardener/etcd-druid/internal/operator"
	"github.com/gardener/etcd-druid/internal/operator/component"
	"github.com/gardener/etcd-druid/internal/utils"
	"github.com/gardener/etcd-druid/test/it/setup"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type componentCreatedAssertionFn func(ctx component.OperatorContext, t *testing.T, opRegistry operator.Registry, etcd *druidv1alpha1.Etcd, timeout, pollInterval time.Duration)

// ReconcilerTestEnv represents the test environment for the etcd reconciler.
type ReconcilerTestEnv struct {
	itTestEnv       setup.IntegrationTestEnv
	itTestEnvCloser setup.IntegrationTestEnvCloser
	reconciler      *etcd.Reconciler
}

func getKindToComponentCreatedAssertionFns() map[operator.Kind]componentCreatedAssertionFn {
	return map[operator.Kind]componentCreatedAssertionFn{
		operator.MemberLeaseKind:         assertMemberLeasesCreated,
		operator.SnapshotLeaseKind:       assertSnapshotLeasesCreated,
		operator.ClientServiceKind:       assertClientServiceCreated,
		operator.PeerServiceKind:         assertPeerServiceCreated,
		operator.ConfigMapKind:           assertConfigMapCreated,
		operator.PodDisruptionBudgetKind: assertPDBCreated,
		operator.ServiceAccountKind:      assertServiceAccountCreated,
		operator.RoleKind:                assertRoleCreated,
		operator.RoleBindingKind:         assertRoleBindingCreated,
		operator.StatefulSetKind:         assertStatefulSetCreated,
	}
}

// assertAllComponentsCreatedSuccessfully asserts that all components of the etcd resource are created successfully eventually.
func assertAllComponentsCreatedSuccessfully(ctx context.Context, t *testing.T, rtEnv ReconcilerTestEnv, etcd *druidv1alpha1.Etcd, timeout, pollInterval time.Duration) {
	kindToAssertionFns := getKindToComponentCreatedAssertionFns()
	assertFns := make([]componentCreatedAssertionFn, 0, len(kindToAssertionFns))
	for _, assertFn := range kindToAssertionFns {
		assertFns = append(assertFns, assertFn)
	}
	doAssertComponentsCreatedSuccessfully(ctx, t, rtEnv, etcd, assertFns, timeout, pollInterval)
}

func assertSelectedComponentsCreatedSuccessfully(ctx context.Context, t *testing.T, rtEnv ReconcilerTestEnv, etcd *druidv1alpha1.Etcd, componentKinds []operator.Kind, timeout, pollInterval time.Duration) {
	g := NewWithT(t)
	assertFns := make([]componentCreatedAssertionFn, 0, len(componentKinds))
	for _, kind := range componentKinds {
		assertFn, ok := getKindToComponentCreatedAssertionFns()[kind]
		g.Expect(ok).To(BeTrue(), fmt.Sprintf("assertion function for %s not found", kind))
		assertFns = append(assertFns, assertFn)
	}
	doAssertComponentsCreatedSuccessfully(ctx, t, rtEnv, etcd, assertFns, timeout, pollInterval)
}

func assertComponentsNotCreatedSuccessfully(ctx context.Context, t *testing.T, rtEnv ReconcilerTestEnv, etcd *druidv1alpha1.Etcd, componentKinds []operator.Kind, timeout, pollInterval time.Duration) {
	opRegistry := rtEnv.reconciler.GetOperatorRegistry()
	opCtx := component.NewOperatorContext(ctx, rtEnv.itTestEnv.GetLogger(), t.Name())
	for _, kind := range componentKinds {
		assertResourceCreation(opCtx, t, opRegistry, kind, etcd, []string{}, timeout, pollInterval)
	}
}

func doAssertComponentsCreatedSuccessfully(ctx context.Context, t *testing.T, rtEnv ReconcilerTestEnv, etcd *druidv1alpha1.Etcd, assertionFns []componentCreatedAssertionFn, timeout, pollInterval time.Duration) {
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

func assertResourceCreation(ctx component.OperatorContext, t *testing.T, opRegistry operator.Registry, kind operator.Kind, etcd *druidv1alpha1.Etcd, expectedResourceNames []string, timeout, pollInterval time.Duration) {
	g := NewWithT(t)
	op := opRegistry.GetOperator(kind)
	checkFn := func() error {
		actualResourceNames, err := op.GetExistingResourceNames(ctx, etcd)
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
		msg := utils.IfConditionOr[string](len(expectedResourceNames) == 0,
			fmt.Sprintf("%s: %v not created successfully", kind, expectedResourceNames),
			fmt.Sprintf("%s: %v created successfully", kind, expectedResourceNames))
		t.Log(msg)
		return nil
	}
	g.Eventually(checkFn).Within(timeout).WithPolling(pollInterval).WithContext(ctx).Should(BeNil())
}

func assertMemberLeasesCreated(ctx component.OperatorContext, t *testing.T, opRegistry operator.Registry, etcd *druidv1alpha1.Etcd, timeout, pollInterval time.Duration) {
	expectedMemberLeaseNames := make([]string, 0, etcd.Spec.Replicas)
	for i := 0; i < int(etcd.Spec.Replicas); i++ {
		expectedMemberLeaseNames = append(expectedMemberLeaseNames, fmt.Sprintf("%s-%d", etcd.Name, i))
	}
	assertResourceCreation(ctx, t, opRegistry, operator.MemberLeaseKind, etcd, expectedMemberLeaseNames, timeout, pollInterval)
}

func assertSnapshotLeasesCreated(ctx component.OperatorContext, t *testing.T, opRegistry operator.Registry, etcd *druidv1alpha1.Etcd, timeout, pollInterval time.Duration) {
	expectedSnapshotLeaseNames := make([]string, 0, 2)
	if etcd.IsBackupStoreEnabled() {
		expectedSnapshotLeaseNames = []string{etcd.GetDeltaSnapshotLeaseName(), etcd.GetFullSnapshotLeaseName()}
	}
	assertResourceCreation(ctx, t, opRegistry, operator.SnapshotLeaseKind, etcd, expectedSnapshotLeaseNames, timeout, pollInterval)
}

func assertClientServiceCreated(ctx component.OperatorContext, t *testing.T, opRegistry operator.Registry, etcd *druidv1alpha1.Etcd, timeout, pollInterval time.Duration) {
	expectedClientServiceNames := []string{etcd.GetClientServiceName()}
	assertResourceCreation(ctx, t, opRegistry, operator.ClientServiceKind, etcd, expectedClientServiceNames, timeout, pollInterval)
}

func assertPeerServiceCreated(ctx component.OperatorContext, t *testing.T, opRegistry operator.Registry, etcd *druidv1alpha1.Etcd, timeout, pollInterval time.Duration) {
	expectedPeerServiceNames := []string{etcd.GetPeerServiceName()}
	assertResourceCreation(ctx, t, opRegistry, operator.PeerServiceKind, etcd, expectedPeerServiceNames, timeout, pollInterval)
}

func assertConfigMapCreated(ctx component.OperatorContext, t *testing.T, opRegistry operator.Registry, etcd *druidv1alpha1.Etcd, timeout, pollInterval time.Duration) {
	expectedConfigMapNames := []string{etcd.GetConfigMapName()}
	assertResourceCreation(ctx, t, opRegistry, operator.ConfigMapKind, etcd, expectedConfigMapNames, timeout, pollInterval)
}

func assertPDBCreated(ctx component.OperatorContext, t *testing.T, opRegistry operator.Registry, etcd *druidv1alpha1.Etcd, timeout, pollInterval time.Duration) {
	expectedPDBNames := []string{etcd.Name}
	assertResourceCreation(ctx, t, opRegistry, operator.PodDisruptionBudgetKind, etcd, expectedPDBNames, timeout, pollInterval)
}

func assertServiceAccountCreated(ctx component.OperatorContext, t *testing.T, opRegistry operator.Registry, etcd *druidv1alpha1.Etcd, timeout, pollInterval time.Duration) {
	expectedServiceAccountNames := []string{etcd.GetServiceAccountName()}
	assertResourceCreation(ctx, t, opRegistry, operator.ServiceAccountKind, etcd, expectedServiceAccountNames, timeout, pollInterval)
}

func assertRoleCreated(ctx component.OperatorContext, t *testing.T, opRegistry operator.Registry, etcd *druidv1alpha1.Etcd, timeout, pollInterval time.Duration) {
	expectedRoleNames := []string{etcd.GetRoleName()}
	assertResourceCreation(ctx, t, opRegistry, operator.RoleKind, etcd, expectedRoleNames, timeout, pollInterval)
}

func assertRoleBindingCreated(ctx component.OperatorContext, t *testing.T, opRegistry operator.Registry, etcd *druidv1alpha1.Etcd, timeout, pollInterval time.Duration) {
	expectedRoleBindingNames := []string{etcd.GetRoleBindingName()}
	assertResourceCreation(ctx, t, opRegistry, operator.RoleBindingKind, etcd, expectedRoleBindingNames, timeout, pollInterval)
}

func assertStatefulSetCreated(ctx component.OperatorContext, t *testing.T, opRegistry operator.Registry, etcd *druidv1alpha1.Etcd, timeout, pollInterval time.Duration) {
	expectedSTSNames := []string{etcd.Name}
	assertResourceCreation(ctx, t, opRegistry, operator.StatefulSetKind, etcd, expectedSTSNames, timeout, pollInterval)
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

func assertETCDLastOperationAndLastErrorsUpdatedSuccessfully(t *testing.T, cl client.Client, etcdObjectKey client.ObjectKey, expectedLastOperation druidv1alpha1.LastOperation, expectedLastErrors []druidv1alpha1.LastError, timeout, pollInterval time.Duration) {
	g := NewWithT(t)
	checkFn := func() error {
		etcdInstance := &druidv1alpha1.Etcd{}
		err := cl.Get(context.Background(), etcdObjectKey, etcdInstance)
		if err != nil {
			return err
		}
		if etcdInstance.Status.LastOperation.Type != expectedLastOperation.Type &&
			etcdInstance.Status.LastOperation.State != expectedLastOperation.State {
			return fmt.Errorf("expected lastOperation to be %s, found %s", expectedLastOperation, etcdInstance.Status.LastOperation)
		}

		// For comparing last errors, it is sufficient to compare their length and their error codes.
		expectedErrorCodes := getErrorCodesFromLastErrors(expectedLastErrors)
		slices.Sort(expectedErrorCodes)
		actualErrorCodes := getErrorCodesFromLastErrors(etcdInstance.Status.LastErrors)
		slices.Sort(actualErrorCodes)

		if !slices.Equal(expectedErrorCodes, actualErrorCodes) {
			return fmt.Errorf("expected lastErrors to be %v, found %v", expectedLastErrors, etcdInstance.Status.LastErrors)
		}
		return nil
	}
	g.Eventually(checkFn).Within(timeout).WithPolling(pollInterval).Should(BeNil())
	t.Log("lastOperation and lastErrors updated successfully")
}

func assertETCDOperationAnnotationRemovedSuccessfully(t *testing.T, cl client.Client, etcdObjectKey client.ObjectKey, timeout, pollInterval time.Duration) {
	g := NewWithT(t)
	checkFn := func() error {
		etcdInstance := &druidv1alpha1.Etcd{}
		err := cl.Get(context.Background(), etcdObjectKey, etcdInstance)
		if err != nil {
			return err
		}
		if metav1.HasAnnotation(etcdInstance.ObjectMeta, v1beta1constants.GardenerOperation) {
			return fmt.Errorf("expected reconcile operation annotation to be removed, found %v", v1beta1constants.GardenerOperation)
		}
		return nil
	}
	g.Eventually(checkFn).Within(timeout).WithPolling(pollInterval).Should(BeNil())
	t.Log("reconcile operation annotation removed successfully")
}

func getErrorCodesFromLastErrors(lastErrors []druidv1alpha1.LastError) []druidv1alpha1.ErrorCode {
	errorCodes := make([]druidv1alpha1.ErrorCode, 0, len(lastErrors))
	for _, lastErr := range lastErrors {
		errorCodes = append(errorCodes, lastErr.Code)
	}
	return errorCodes
}

func logPointerTypeToString[T any](val *T) string {
	if val == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%v", *val)
}
