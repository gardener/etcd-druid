// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package etcd

import (
	"github.com/gardener/etcd-druid/api/core/v1alpha1"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"testing"

	mockmanager "github.com/gardener/etcd-druid/internal/mock/controller-runtime/manager"
	testutils "github.com/gardener/etcd-druid/test/utils"

	"go.uber.org/mock/gomock"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/event"

	. "github.com/onsi/gomega"
)

type predicateTestCase struct {
	name                 string
	etcdSpecChanged      bool
	etcdStatusChanged    bool
	previouslyReconciled bool
	lastOperationState   *druidv1alpha1.LastOperationState
	// expected behavior for different event types
	shouldAllowCreateEvent  bool
	shouldAllowDeleteEvent  bool
	shouldAllowGenericEvent bool
	shouldAllowUpdateEvent  bool
}

func TestBuildPredicateWithOnlyAutoReconcileEnabled(t *testing.T) {
	testCases := []predicateTestCase{
		{
			name:                    "only spec has changed and previous reconciliation is in progress",
			etcdSpecChanged:         true,
			lastOperationState:      ptr.To(druidv1alpha1.LastOperationStateProcessing),
			shouldAllowCreateEvent:  true,
			shouldAllowDeleteEvent:  true,
			shouldAllowGenericEvent: false,
			shouldAllowUpdateEvent:  true,
		},
		{
			name:                    "only spec has changed and previous reconciliation has completed",
			etcdSpecChanged:         true,
			previouslyReconciled:    true,
			lastOperationState:      ptr.To(druidv1alpha1.LastOperationStateSucceeded),
			shouldAllowCreateEvent:  true,
			shouldAllowDeleteEvent:  true,
			shouldAllowGenericEvent: false,
			shouldAllowUpdateEvent:  true,
		},
		{
			name:                    "only spec has changed and previous reconciliation has errored",
			etcdSpecChanged:         true,
			previouslyReconciled:    true,
			lastOperationState:      ptr.To(druidv1alpha1.LastOperationStateError),
			shouldAllowCreateEvent:  true,
			shouldAllowDeleteEvent:  true,
			shouldAllowGenericEvent: false,
			shouldAllowUpdateEvent:  true,
		},
		{
			name:                    "only status has changed",
			etcdStatusChanged:       true,
			previouslyReconciled:    true,
			shouldAllowCreateEvent:  true,
			shouldAllowDeleteEvent:  true,
			shouldAllowGenericEvent: false,
			shouldAllowUpdateEvent:  false,
		},
		{
			name:                    "both spec and status have changed and previous reconciliation is in progress",
			etcdSpecChanged:         true,
			etcdStatusChanged:       true,
			previouslyReconciled:    true,
			lastOperationState:      ptr.To(druidv1alpha1.LastOperationStateProcessing),
			shouldAllowCreateEvent:  true,
			shouldAllowDeleteEvent:  true,
			shouldAllowGenericEvent: false,
			shouldAllowUpdateEvent:  true,
		},
		{
			name:                    "neither spec nor status has changed",
			previouslyReconciled:    true,
			shouldAllowCreateEvent:  true,
			shouldAllowDeleteEvent:  true,
			shouldAllowGenericEvent: false,
			shouldAllowUpdateEvent:  false,
		},
		{
			// This case is described in https://github.com/gardener/etcd-druid/issues/898
			name:                    "for an existing Etcd resource, neither spec nor status has changed and it has never been reconciled before",
			previouslyReconciled:    false,
			shouldAllowCreateEvent:  true,
			shouldAllowDeleteEvent:  true,
			shouldAllowGenericEvent: false,
			shouldAllowUpdateEvent:  true,
		},
	}
	g := NewWithT(t)
	etcd := createEtcd()
	r := createReconciler(t, "etcd-controller-only-auto-reconcile-enabled", true)
	predicate := r.buildPredicate()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			updatedEtcd := updateEtcd(etcd, tc.etcdSpecChanged, tc.etcdStatusChanged, tc.previouslyReconciled, tc.lastOperationState, false)
			g.Expect(predicate.Create(event.CreateEvent{Object: updatedEtcd})).To(Equal(tc.shouldAllowCreateEvent))
			g.Expect(predicate.Delete(event.DeleteEvent{Object: updatedEtcd})).To(Equal(tc.shouldAllowDeleteEvent))
			g.Expect(predicate.Generic(event.GenericEvent{Object: updatedEtcd})).To(Equal(tc.shouldAllowGenericEvent))
			g.Expect(predicate.Update(event.UpdateEvent{ObjectOld: etcd, ObjectNew: updatedEtcd})).To(Equal(tc.shouldAllowUpdateEvent))
		})
	}
}

func TestBuildPredicateWithNoAutoReconcileAndNoReconcileAnnot(t *testing.T) {
	testCases := []predicateTestCase{
		{
			name:                    "only spec has changed",
			etcdSpecChanged:         true,
			previouslyReconciled:    true,
			shouldAllowCreateEvent:  true,
			shouldAllowDeleteEvent:  true,
			shouldAllowGenericEvent: false,
			shouldAllowUpdateEvent:  false,
		},
		{
			name:                    "only status has changed",
			etcdStatusChanged:       true,
			previouslyReconciled:    true,
			shouldAllowCreateEvent:  true,
			shouldAllowDeleteEvent:  true,
			shouldAllowGenericEvent: false,
			shouldAllowUpdateEvent:  false,
		},
		{
			name:                    "both spec and status have changed",
			etcdSpecChanged:         true,
			previouslyReconciled:    true,
			etcdStatusChanged:       true,
			shouldAllowCreateEvent:  true,
			shouldAllowDeleteEvent:  true,
			shouldAllowGenericEvent: false,
			shouldAllowUpdateEvent:  false,
		},
		{
			name:                    "neither spec nor status has changed",
			previouslyReconciled:    true,
			shouldAllowCreateEvent:  true,
			shouldAllowDeleteEvent:  true,
			shouldAllowGenericEvent: false,
			shouldAllowUpdateEvent:  false,
		},
		{
			// This case is described in https://github.com/gardener/etcd-druid/issues/898
			name:                    "for an existing Etcd resource, neither spec nor status has changed and it has never been reconciled before",
			previouslyReconciled:    false,
			shouldAllowCreateEvent:  true,
			shouldAllowDeleteEvent:  true,
			shouldAllowGenericEvent: false,
			shouldAllowUpdateEvent:  false,
		},
	}
	g := NewWithT(t)
	etcd := createEtcd()
	r := createReconciler(t, "etcd-controller-no-auto-reconcile-no-annotation", false)
	predicate := r.buildPredicate()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			updatedEtcd := updateEtcd(etcd, tc.etcdSpecChanged, tc.etcdStatusChanged, tc.previouslyReconciled, tc.lastOperationState, false)
			g.Expect(predicate.Create(event.CreateEvent{Object: updatedEtcd})).To(Equal(tc.shouldAllowCreateEvent))
			g.Expect(predicate.Delete(event.DeleteEvent{Object: updatedEtcd})).To(Equal(tc.shouldAllowDeleteEvent))
			g.Expect(predicate.Generic(event.GenericEvent{Object: updatedEtcd})).To(Equal(tc.shouldAllowGenericEvent))
			g.Expect(predicate.Update(event.UpdateEvent{ObjectOld: etcd, ObjectNew: updatedEtcd})).To(Equal(tc.shouldAllowUpdateEvent))
		})
	}
}

func TestBuildPredicateWithNoAutoReconcileButReconcileAnnotPresent(t *testing.T) {
	testCases := []predicateTestCase{
		{
			name:                    "only spec has changed and previous reconciliation is in progress",
			etcdSpecChanged:         true,
			lastOperationState:      ptr.To(druidv1alpha1.LastOperationStateProcessing),
			shouldAllowCreateEvent:  true,
			shouldAllowDeleteEvent:  true,
			shouldAllowGenericEvent: false,
			shouldAllowUpdateEvent:  true,
		},
		{
			name:                    "only spec has changed and previous reconciliation is completed",
			etcdSpecChanged:         true,
			lastOperationState:      ptr.To(druidv1alpha1.LastOperationStateSucceeded),
			previouslyReconciled:    true,
			shouldAllowCreateEvent:  true,
			shouldAllowDeleteEvent:  true,
			shouldAllowGenericEvent: false,
			shouldAllowUpdateEvent:  true,
		},
		{
			name:                    "only status has changed and previous reconciliation is in progress",
			etcdStatusChanged:       true,
			previouslyReconciled:    true,
			lastOperationState:      ptr.To(druidv1alpha1.LastOperationStateProcessing),
			shouldAllowCreateEvent:  true,
			shouldAllowDeleteEvent:  true,
			shouldAllowGenericEvent: false,
			shouldAllowUpdateEvent:  false,
		},
		{
			name:                    "only status has changed and previous reconciliation is completed",
			etcdStatusChanged:       true,
			previouslyReconciled:    true,
			lastOperationState:      ptr.To(druidv1alpha1.LastOperationStateSucceeded),
			shouldAllowCreateEvent:  true,
			shouldAllowDeleteEvent:  true,
			shouldAllowGenericEvent: false,
			shouldAllowUpdateEvent:  true,
		},
		{
			name:                    "both spec and status have changed",
			etcdSpecChanged:         true,
			etcdStatusChanged:       true,
			previouslyReconciled:    true,
			shouldAllowCreateEvent:  true,
			shouldAllowDeleteEvent:  true,
			shouldAllowGenericEvent: false,
			shouldAllowUpdateEvent:  true,
		},
		{
			name:                    "neither spec nor status has changed and previous reconciliation is in error",
			lastOperationState:      ptr.To(druidv1alpha1.LastOperationStateError),
			previouslyReconciled:    true,
			shouldAllowCreateEvent:  true,
			shouldAllowDeleteEvent:  true,
			shouldAllowGenericEvent: false,
			shouldAllowUpdateEvent:  false,
		},
		{
			name:                    "neither spec nor status has changed and previous reconciliation is completed",
			lastOperationState:      ptr.To(druidv1alpha1.LastOperationStateSucceeded),
			previouslyReconciled:    true,
			shouldAllowCreateEvent:  true,
			shouldAllowDeleteEvent:  true,
			shouldAllowGenericEvent: false,
			shouldAllowUpdateEvent:  true,
		},
		{
			// This case is described in https://github.com/gardener/etcd-druid/issues/898
			name:                    "for an existing Etcd resource, neither spec nor status has changed and it has never been reconciled before",
			previouslyReconciled:    false,
			shouldAllowCreateEvent:  true,
			shouldAllowDeleteEvent:  true,
			shouldAllowGenericEvent: false,
			shouldAllowUpdateEvent:  true,
		},
	}
	g := NewWithT(t)
	etcd := createEtcd()
	r := createReconciler(t, "etcd-controller-with-no-auto-reconcile-but-reconcile-annotation", false)
	predicate := r.buildPredicate()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			updatedEtcd := updateEtcd(etcd, tc.etcdSpecChanged, tc.etcdStatusChanged, tc.previouslyReconciled, tc.lastOperationState, true)
			g.Expect(predicate.Create(event.CreateEvent{Object: updatedEtcd})).To(Equal(tc.shouldAllowCreateEvent))
			g.Expect(predicate.Delete(event.DeleteEvent{Object: updatedEtcd})).To(Equal(tc.shouldAllowDeleteEvent))
			g.Expect(predicate.Generic(event.GenericEvent{Object: updatedEtcd})).To(Equal(tc.shouldAllowGenericEvent))
			g.Expect(predicate.Update(event.UpdateEvent{ObjectOld: etcd, ObjectNew: updatedEtcd})).To(Equal(tc.shouldAllowUpdateEvent))
		})
	}
}

func TestBuildPredicateWithAutoReconcileAndReconcileAnnotSet(t *testing.T) {
	testCases := []predicateTestCase{
		{
			name:                    "only spec has changed",
			etcdSpecChanged:         true,
			shouldAllowCreateEvent:  true,
			shouldAllowDeleteEvent:  true,
			shouldAllowGenericEvent: false,
			shouldAllowUpdateEvent:  true,
		},
		{
			name:                    "only status has changed and previous reconciliation is in progress",
			etcdStatusChanged:       true,
			lastOperationState:      ptr.To(druidv1alpha1.LastOperationStateProcessing),
			shouldAllowCreateEvent:  true,
			shouldAllowDeleteEvent:  true,
			shouldAllowGenericEvent: false,
			shouldAllowUpdateEvent:  false,
		},
		{
			name:                    "only status has changed and previous reconciliation is completed",
			etcdStatusChanged:       true,
			lastOperationState:      ptr.To(druidv1alpha1.LastOperationStateSucceeded),
			previouslyReconciled:    true,
			shouldAllowCreateEvent:  true,
			shouldAllowDeleteEvent:  true,
			shouldAllowGenericEvent: false,
			shouldAllowUpdateEvent:  true,
		},
		{
			name:                    "both spec and status have changed",
			etcdSpecChanged:         true,
			etcdStatusChanged:       true,
			previouslyReconciled:    true,
			shouldAllowCreateEvent:  true,
			shouldAllowDeleteEvent:  true,
			shouldAllowGenericEvent: false,
			shouldAllowUpdateEvent:  true,
		},
		{
			name:                    "neither spec nor status has changed and previous reconciliation is in error",
			lastOperationState:      ptr.To(druidv1alpha1.LastOperationStateError),
			previouslyReconciled:    true,
			shouldAllowCreateEvent:  true,
			shouldAllowDeleteEvent:  true,
			shouldAllowGenericEvent: false,
			shouldAllowUpdateEvent:  false,
		},
		{
			name:                    "neither spec nor status has changed and previous reconciliation is in progress",
			lastOperationState:      ptr.To(druidv1alpha1.LastOperationStateProcessing),
			shouldAllowCreateEvent:  true,
			shouldAllowDeleteEvent:  true,
			shouldAllowGenericEvent: false,
			shouldAllowUpdateEvent:  false,
		},
		{
			name:                    "neither spec nor status has changed and previous reconciliation is completed",
			lastOperationState:      ptr.To(druidv1alpha1.LastOperationStateSucceeded),
			previouslyReconciled:    true,
			shouldAllowCreateEvent:  true,
			shouldAllowDeleteEvent:  true,
			shouldAllowGenericEvent: false,
			shouldAllowUpdateEvent:  true,
		},
		{
			// This case is described in https://github.com/gardener/etcd-druid/issues/898
			name:                    "for an existing Etcd resource, neither spec nor status has changed and it has never been reconciled before",
			previouslyReconciled:    false,
			shouldAllowCreateEvent:  true,
			shouldAllowDeleteEvent:  true,
			shouldAllowGenericEvent: false,
			shouldAllowUpdateEvent:  true,
		},
	}
	g := NewWithT(t)
	etcd := createEtcd()
	r := createReconciler(t, "etcd-controller-with-auto-reconcile-and-reconcile-annotation", true)
	predicate := r.buildPredicate()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			updatedEtcd := updateEtcd(etcd, tc.etcdSpecChanged, tc.etcdStatusChanged, tc.previouslyReconciled, tc.lastOperationState, true)
			g.Expect(predicate.Create(event.CreateEvent{Object: updatedEtcd})).To(Equal(tc.shouldAllowCreateEvent))
			g.Expect(predicate.Delete(event.DeleteEvent{Object: updatedEtcd})).To(Equal(tc.shouldAllowDeleteEvent))
			g.Expect(predicate.Generic(event.GenericEvent{Object: updatedEtcd})).To(Equal(tc.shouldAllowGenericEvent))
			g.Expect(predicate.Update(event.UpdateEvent{ObjectOld: etcd, ObjectNew: updatedEtcd})).To(Equal(tc.shouldAllowUpdateEvent))
		})
	}
}

func createEtcd() *druidv1alpha1.Etcd {
	etcd := testutils.EtcdBuilderWithDefaults(testutils.TestEtcdName, testutils.TestNamespace).WithReplicas(3).Build()
	etcd.Status = druidv1alpha1.EtcdStatus{
		ObservedGeneration: nil,
		Etcd: &druidv1alpha1.CrossVersionObjectReference{
			Kind:       "StatefulSet",
			Name:       testutils.TestEtcdName,
			APIVersion: "apps/v1",
		},
		CurrentReplicas: 3,
		Replicas:        3,
		ReadyReplicas:   3,
		Ready:           ptr.To(true),
	}
	return etcd
}

func updateEtcd(originalEtcd *druidv1alpha1.Etcd, specChanged, statusChanged bool, previouslyReconciled bool, lastOpState *druidv1alpha1.LastOperationState, reconcileAnnotPresent bool) *druidv1alpha1.Etcd {
	newEtcd := originalEtcd.DeepCopy()
	annotations := make(map[string]string)
	if reconcileAnnotPresent {
		annotations[v1alpha1.DruidOperationAnnotation] = v1alpha1.DruidOperationReconcile
		newEtcd.SetAnnotations(annotations)
	}
	if specChanged {
		// made a single change to the spec
		newEtcd.Spec.Backup.Image = ptr.To("eu.gcr.io/gardener-project/gardener/etcdbrctl-distroless:v1.0.0")
		newEtcd.Generation++
	}
	if statusChanged {
		// made a single change to the status
		newEtcd.Status.ReadyReplicas = 2
		newEtcd.Status.Ready = ptr.To(false)
	}
	if previouslyReconciled && originalEtcd.Status.ObservedGeneration == nil {
		newEtcd.Status.ObservedGeneration = ptr.To[int64](1)
	}
	if lastOpState != nil {
		newEtcd.Status.LastOperation = &druidv1alpha1.LastOperation{
			Type:  druidv1alpha1.LastOperationTypeReconcile,
			State: *lastOpState,
		}
	}
	return newEtcd
}

func createReconciler(t *testing.T, controllerName string, enableEtcdSpecAutoReconcile bool) *Reconciler {
	g := NewWithT(t)
	mockCtrl := gomock.NewController(t)
	mgr := mockmanager.NewMockManager(mockCtrl)
	mgr.EXPECT().GetClient().AnyTimes().Return(testutils.NewTestClientBuilder().Build())
	mgr.EXPECT().GetEventRecorderFor(gomock.Any()).AnyTimes().Return(nil)
	etcdConfig := Config{
		EnableEtcdSpecAutoReconcile: enableEtcdSpecAutoReconcile,
	}
	r, err := NewReconcilerWithImageVector(mgr, controllerName, &etcdConfig, nil)
	g.Expect(err).NotTo(HaveOccurred())
	return r
}
