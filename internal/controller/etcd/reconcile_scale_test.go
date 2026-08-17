// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package etcd

import (
	"context"
	"fmt"
	"testing"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	clientkubernetes "github.com/gardener/etcd-druid/internal/client/kubernetes"
	"github.com/gardener/etcd-druid/internal/component"
	ctrlutils "github.com/gardener/etcd-druid/internal/controller/utils"
	testutils "github.com/gardener/etcd-druid/test/utils"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	. "github.com/onsi/gomega"
)

const (
	scaleTestEtcdName   = "etcd-scale"
	scaleTestNamespace  = "test-ns"
	scaleTestEtcdUID    = types.UID("etcd-scale-uid")
	scaleTestBootstrapA = "etcd-source-0"
	scaleTestBootstrapB = "etcd-source-1"
)

// TestDetermineScaleOperation exercises the pure detection logic (spec vs the
// observed StatefulSet and status), which decides the ScaleOperationComplete
// condition status/reason. It covers all four detection outcomes plus the edge
// cases (no StatefulSet, transitions to/from zero, and BootstrapMembersRemoval
// taking precedence over a replicas comparison). The condition uses positive
// polarity: True/NoScaleOperation is converged, and an in-flight operation is
// False with the reason naming it.
func TestDetermineScaleOperation(t *testing.T) {
	bootstrapStatus := func(names ...string) *druidv1alpha1.BootstrapWithExistingClusterStatus {
		members := make([]druidv1alpha1.BootstrapJoinedMember, 0, len(names))
		for _, n := range names {
			members = append(members, druidv1alpha1.BootstrapJoinedMember{Name: n})
		}
		return &druidv1alpha1.BootstrapWithExistingClusterStatus{Members: members}
	}
	specBootstrap := func(names ...string) *druidv1alpha1.BootstrapWithExistingCluster {
		members := make([]druidv1alpha1.BootstrapExistingMember, 0, len(names))
		for _, n := range names {
			members = append(members, druidv1alpha1.BootstrapExistingMember{Name: n})
		}
		return &druidv1alpha1.BootstrapWithExistingCluster{Members: members}
	}

	tests := []struct {
		name string
		// specReplicas is the desired replica count on the Etcd resource.
		specReplicas int32
		// stsReplicas, when non-nil, is the observed StatefulSet's spec.replicas;
		// nil means no StatefulSet exists yet.
		stsReplicas *int32
		// statusBootstrap and specBootstrap drive the BootstrapMembersRemoval path.
		statusBootstrap *druidv1alpha1.BootstrapWithExistingClusterStatus
		specBootstrap   *druidv1alpha1.BootstrapWithExistingCluster
		wantStatus      druidv1alpha1.ConditionStatus
		wantReason      string
	}{
		{
			name:         "no StatefulSet yet (fresh cluster) -> NoScaleOperation",
			specReplicas: 3,
			stsReplicas:  nil,
			wantStatus:   druidv1alpha1.ConditionTrue,
			wantReason:   druidv1alpha1.ScaleOperationReasonNoScaleOperation,
		},
		{
			name:         "spec < observed -> ScalingIn",
			specReplicas: 3,
			stsReplicas:  ptr.To(int32(5)),
			wantStatus:   druidv1alpha1.ConditionFalse,
			wantReason:   druidv1alpha1.ScaleOperationReasonScalingIn,
		},
		{
			name:         "spec > observed -> ScalingOut",
			specReplicas: 5,
			stsReplicas:  ptr.To(int32(3)),
			wantStatus:   druidv1alpha1.ConditionFalse,
			wantReason:   druidv1alpha1.ScaleOperationReasonScalingOut,
		},
		{
			name:         "spec == observed -> NoScaleOperation",
			specReplicas: 3,
			stsReplicas:  ptr.To(int32(3)),
			wantStatus:   druidv1alpha1.ConditionTrue,
			wantReason:   druidv1alpha1.ScaleOperationReasonNoScaleOperation,
		},
		{
			name:         "spec.replicas == 0 (hibernation) -> NoScaleOperation, not ScalingIn",
			specReplicas: 0,
			stsReplicas:  ptr.To(int32(3)),
			wantStatus:   druidv1alpha1.ConditionTrue,
			wantReason:   druidv1alpha1.ScaleOperationReasonNoScaleOperation,
		},
		{
			name:         "observed == 0 (wake-up) -> NoScaleOperation, not ScalingOut",
			specReplicas: 3,
			stsReplicas:  ptr.To(int32(0)),
			wantStatus:   druidv1alpha1.ConditionTrue,
			wantReason:   druidv1alpha1.ScaleOperationReasonNoScaleOperation,
		},
		{
			name:            "joined member no longer in spec -> BootstrapMembersRemoval",
			specReplicas:    3,
			stsReplicas:     ptr.To(int32(3)),
			statusBootstrap: bootstrapStatus(scaleTestBootstrapA, scaleTestBootstrapB),
			specBootstrap:   specBootstrap(scaleTestBootstrapA),
			wantStatus:      druidv1alpha1.ConditionFalse,
			wantReason:      druidv1alpha1.ScaleOperationReasonBootstrapMembersRemoval,
		},
		{
			name:            "spec bootstrap unset but members joined -> BootstrapMembersRemoval",
			specReplicas:    3,
			stsReplicas:     ptr.To(int32(3)),
			statusBootstrap: bootstrapStatus(scaleTestBootstrapA),
			specBootstrap:   nil,
			wantStatus:      druidv1alpha1.ConditionFalse,
			wantReason:      druidv1alpha1.ScaleOperationReasonBootstrapMembersRemoval,
		},
		{
			name:            "all joined members still in spec -> falls through to replicas comparison",
			specReplicas:    3,
			stsReplicas:     ptr.To(int32(3)),
			statusBootstrap: bootstrapStatus(scaleTestBootstrapA),
			specBootstrap:   specBootstrap(scaleTestBootstrapA),
			wantStatus:      druidv1alpha1.ConditionTrue,
			wantReason:      druidv1alpha1.ScaleOperationReasonNoScaleOperation,
		},
		{
			name:            "BootstrapMembersRemoval takes precedence over a concurrent scale-out",
			specReplicas:    5,
			stsReplicas:     ptr.To(int32(3)),
			statusBootstrap: bootstrapStatus(scaleTestBootstrapA, scaleTestBootstrapB),
			specBootstrap:   specBootstrap(scaleTestBootstrapA),
			wantStatus:      druidv1alpha1.ConditionFalse,
			wantReason:      druidv1alpha1.ScaleOperationReasonBootstrapMembersRemoval,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			etcd := testutils.EtcdBuilderWithoutDefaults(scaleTestEtcdName, scaleTestNamespace).
				WithReplicas(tc.specReplicas).
				Build()
			etcd.UID = scaleTestEtcdUID
			etcd.Spec.Etcd.BootstrapWithExistingCluster = tc.specBootstrap
			etcd.Status.BootstrapWithExistingCluster = tc.statusBootstrap

			builder := fakeclient.NewClientBuilder().WithScheme(clientkubernetes.Scheme)
			if tc.stsReplicas != nil {
				sts := testutils.CreateStatefulSet(druidv1alpha1.GetStatefulSetName(etcd.ObjectMeta), scaleTestNamespace, scaleTestEtcdUID, *tc.stsReplicas)
				sts.Spec.Replicas = tc.stsReplicas
				builder = builder.WithObjects(sts)
			}
			cl := builder.Build()
			r := &Reconciler{client: cl}

			gotStatus, gotReason := r.determineScaleOperation(newScaleTestOperatorContext(), etcd)
			g.Expect(gotStatus).To(Equal(tc.wantStatus))
			g.Expect(gotReason).To(Equal(tc.wantReason))
		})
	}
}

func TestDetermineScaleOperationPreservesExistingConditionOnStatefulSetGetError(t *testing.T) {
	g := NewWithT(t)

	etcd := testutils.EtcdBuilderWithoutDefaults(scaleTestEtcdName, scaleTestNamespace).
		WithReplicas(3).
		Build()
	etcd.UID = scaleTestEtcdUID
	etcd.Status.Conditions = []druidv1alpha1.Condition{{
		Type:   druidv1alpha1.ConditionTypeScaleOperationComplete,
		Status: druidv1alpha1.ConditionFalse,
		Reason: druidv1alpha1.ScaleOperationReasonScalingIn,
	}}

	cl := fakeclient.NewClientBuilder().
		WithScheme(clientkubernetes.Scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, cl client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if _, ok := obj.(*appsv1.StatefulSet); ok {
					return fmt.Errorf("transient apiserver failure")
				}
				return cl.Get(ctx, key, obj, opts...)
			},
		}).
		Build()
	r := &Reconciler{client: cl}

	gotStatus, gotReason := r.determineScaleOperation(newScaleTestOperatorContext(), etcd)
	g.Expect(gotStatus).To(Equal(druidv1alpha1.ConditionFalse))
	g.Expect(gotReason).To(Equal(druidv1alpha1.ScaleOperationReasonScalingIn))
}

// TestDetectAndRecordScaleOperation exercises the full reconcile step: it must
// patch the ScaleOperationComplete condition onto the Etcd status subresource
// when there is an operation to record, and it must be a no-op (no condition
// added) for a brand-new resource with no scale operation in progress.
func TestDetectAndRecordScaleOperation(t *testing.T) {
	tests := []struct {
		name         string
		specReplicas int32
		stsReplicas  *int32
		// wantConditionPresent indicates whether the ScaleOperationComplete
		// condition should exist on the status after the step runs.
		wantConditionPresent bool
		wantStatus           druidv1alpha1.ConditionStatus
		wantReason           string
	}{
		{
			name:                 "scale-in records the condition",
			specReplicas:         3,
			stsReplicas:          ptr.To(int32(5)),
			wantConditionPresent: true,
			wantStatus:           druidv1alpha1.ConditionFalse,
			wantReason:           druidv1alpha1.ScaleOperationReasonScalingIn,
		},
		{
			name:                 "scale-out records the condition",
			specReplicas:         5,
			stsReplicas:          ptr.To(int32(3)),
			wantConditionPresent: true,
			wantStatus:           druidv1alpha1.ConditionFalse,
			wantReason:           druidv1alpha1.ScaleOperationReasonScalingOut,
		},
		{
			name:                 "no scale operation on a fresh resource adds no condition",
			specReplicas:         3,
			stsReplicas:          nil,
			wantConditionPresent: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			etcd := testutils.EtcdBuilderWithoutDefaults(scaleTestEtcdName, scaleTestNamespace).
				WithReplicas(tc.specReplicas).
				Build()
			etcd.UID = scaleTestEtcdUID

			builder := fakeclient.NewClientBuilder().
				WithScheme(clientkubernetes.Scheme).
				WithObjects(etcd).
				WithStatusSubresource(etcd)
			if tc.stsReplicas != nil {
				sts := testutils.CreateStatefulSet(druidv1alpha1.GetStatefulSetName(etcd.ObjectMeta), scaleTestNamespace, scaleTestEtcdUID, *tc.stsReplicas)
				sts.Spec.Replicas = tc.stsReplicas
				builder = builder.WithObjects(sts)
			}
			cl := builder.Build()
			r := &Reconciler{client: cl}

			res := r.detectAndRecordScaleOperation(newScaleTestOperatorContext(), etcd)
			g.Expect(res.HasErrors()).To(BeFalse())

			// Re-read the persisted Etcd to assert the condition was patched onto
			// the status subresource (not just mutated in memory).
			persisted := &druidv1alpha1.Etcd{}
			g.Expect(cl.Get(context.Background(), types.NamespacedName{Name: scaleTestEtcdName, Namespace: scaleTestNamespace}, persisted)).To(Succeed())

			idx := findScaleCondition(persisted.Status.Conditions)
			if !tc.wantConditionPresent {
				g.Expect(idx).To(BeNumerically("<", 0), "no ScaleOperationComplete condition should be recorded")
				return
			}
			g.Expect(idx).To(BeNumerically(">=", 0), "ScaleOperationComplete condition should be recorded")
			cond := persisted.Status.Conditions[idx]
			g.Expect(cond.Status).To(Equal(tc.wantStatus))
			g.Expect(cond.Reason).To(Equal(tc.wantReason))
			g.Expect(cond.LastTransitionTime.IsZero()).To(BeFalse())
			g.Expect(cond.Message).NotTo(BeEmpty())
		})
	}
}

// TestDetectAndRecordScaleOperationIsIdempotent verifies that a second call to
// detectAndRecordScaleOperation with the same state issues no Status().Patch.
// The guard in scaleConditionNeedsUpdate should short-circuit when the existing
// condition already matches the determined (status, reason) pair.
func TestDetectAndRecordScaleOperationIsIdempotent(t *testing.T) {
	g := NewWithT(t)

	etcd := testutils.EtcdBuilderWithoutDefaults(scaleTestEtcdName, scaleTestNamespace).
		WithReplicas(3).
		Build()
	etcd.UID = scaleTestEtcdUID
	// Pre-seed a condition that exactly matches what a 5→3 scale-in would produce.
	etcd.Status.Conditions = []druidv1alpha1.Condition{{
		Type:   druidv1alpha1.ConditionTypeScaleOperationComplete,
		Status: druidv1alpha1.ConditionFalse,
		Reason: druidv1alpha1.ScaleOperationReasonScalingIn,
	}}

	patchCount := 0
	cl := fakeclient.NewClientBuilder().
		WithScheme(clientkubernetes.Scheme).
		WithObjects(etcd).
		WithStatusSubresource(etcd).
		WithInterceptorFuncs(interceptor.Funcs{
			SubResourcePatch: func(ctx context.Context, cl client.Client, subResourceName string, obj client.Object, patch client.Patch, opts ...client.SubResourcePatchOption) error {
				patchCount++
				return cl.SubResource(subResourceName).Patch(ctx, obj, patch, opts...)
			},
		}).
		Build()

	sts := testutils.CreateStatefulSet(druidv1alpha1.GetStatefulSetName(etcd.ObjectMeta), scaleTestNamespace, scaleTestEtcdUID, 5)
	sts.Spec.Replicas = ptr.To(int32(5))
	g.Expect(cl.Create(context.Background(), sts)).To(Succeed())

	r := &Reconciler{client: cl}
	res := r.detectAndRecordScaleOperation(newScaleTestOperatorContext(), etcd)
	g.Expect(res.HasErrors()).To(BeFalse())
	g.Expect(patchCount).To(Equal(0), "no Status().Patch should be issued when condition already matches")
}

// TestPruneBootstrapMembersStatus verifies that joined source members no longer
// present in spec.etcd.bootstrapWithExistingCluster are dropped from
// status.bootstrapWithExistingCluster.members, that the whole status field is
// cleared once nothing remains, and that it is a no-op when every joined member
// is still in spec (or nothing has joined at all).
func TestPruneBootstrapMembersStatus(t *testing.T) {
	joined := func(names ...string) []druidv1alpha1.BootstrapJoinedMember {
		members := make([]druidv1alpha1.BootstrapJoinedMember, 0, len(names))
		for _, n := range names {
			members = append(members, druidv1alpha1.BootstrapJoinedMember{Name: n})
		}
		return members
	}
	specBootstrap := func(names ...string) *druidv1alpha1.BootstrapWithExistingCluster {
		members := make([]druidv1alpha1.BootstrapExistingMember, 0, len(names))
		for _, n := range names {
			members = append(members, druidv1alpha1.BootstrapExistingMember{Name: n})
		}
		return &druidv1alpha1.BootstrapWithExistingCluster{Members: members}
	}

	tests := []struct {
		name string
		// statusJoined are the members recorded as joined in status (nil = no field).
		statusJoined []druidv1alpha1.BootstrapJoinedMember
		// specBootstrap is spec.etcd.bootstrapWithExistingCluster (nil = unset).
		specBootstrap *druidv1alpha1.BootstrapWithExistingCluster
		// wantRetained are the member names expected to remain in status; nil means
		// the whole status.bootstrapWithExistingCluster field should be cleared.
		wantRetained []string
		wantCleared  bool
	}{
		{
			name:          "no status members -> no-op (field stays nil)",
			statusJoined:  nil,
			specBootstrap: specBootstrap(scaleTestBootstrapA),
			wantCleared:   true,
		},
		{
			name:          "all joined members still in spec -> retained unchanged",
			statusJoined:  joined(scaleTestBootstrapA, scaleTestBootstrapB),
			specBootstrap: specBootstrap(scaleTestBootstrapA, scaleTestBootstrapB),
			wantRetained:  []string{scaleTestBootstrapA, scaleTestBootstrapB},
		},
		{
			name:          "one joined member removed from spec -> pruned, other retained",
			statusJoined:  joined(scaleTestBootstrapA, scaleTestBootstrapB),
			specBootstrap: specBootstrap(scaleTestBootstrapA),
			wantRetained:  []string{scaleTestBootstrapA},
		},
		{
			name:          "spec bootstrap unset -> whole status field cleared",
			statusJoined:  joined(scaleTestBootstrapA, scaleTestBootstrapB),
			specBootstrap: nil,
			wantCleared:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			etcd := testutils.EtcdBuilderWithoutDefaults(scaleTestEtcdName, scaleTestNamespace).
				WithReplicas(3).
				Build()
			etcd.UID = scaleTestEtcdUID
			etcd.Spec.Etcd.BootstrapWithExistingCluster = tc.specBootstrap
			if tc.statusJoined != nil {
				etcd.Status.BootstrapWithExistingCluster = &druidv1alpha1.BootstrapWithExistingClusterStatus{Members: tc.statusJoined}
			}

			cl := fakeclient.NewClientBuilder().
				WithScheme(clientkubernetes.Scheme).
				WithObjects(etcd).
				WithStatusSubresource(etcd).
				Build()
			r := &Reconciler{client: cl}

			res := r.pruneBootstrapMembersStatus(newScaleTestOperatorContext(), etcd)
			g.Expect(res.HasErrors()).To(BeFalse())

			persisted := &druidv1alpha1.Etcd{}
			g.Expect(cl.Get(context.Background(), types.NamespacedName{Name: scaleTestEtcdName, Namespace: scaleTestNamespace}, persisted)).To(Succeed())

			if tc.wantCleared {
				g.Expect(persisted.Status.BootstrapWithExistingCluster).To(BeNil())
				return
			}
			g.Expect(persisted.Status.BootstrapWithExistingCluster).NotTo(BeNil())
			gotNames := make([]string, 0, len(persisted.Status.BootstrapWithExistingCluster.Members))
			for _, m := range persisted.Status.BootstrapWithExistingCluster.Members {
				gotNames = append(gotNames, m.Name)
			}
			g.Expect(gotNames).To(ConsistOf(tc.wantRetained))
		})
	}
}

// TestRecordReconcileSuccessMarksScaleConditionComplete verifies that a completed
// spec reconciliation marks an in-flight ScaleOperationComplete condition back to
// True/NoScaleOperation, and that it does not add the condition when none was
// recorded.
func TestRecordReconcileSuccessClearsScaleCondition(t *testing.T) {
	tests := []struct {
		name string
		// existingCondition, when non-nil, seeds a ScaleOperationComplete
		// condition on the status before the success step runs.
		existingCondition    *druidv1alpha1.Condition
		wantConditionPresent bool
		wantStatus           druidv1alpha1.ConditionStatus
		wantReason           string
	}{
		{
			name: "in-flight scale-in condition is marked complete (True/NoScaleOperation)",
			existingCondition: &druidv1alpha1.Condition{
				Type:   druidv1alpha1.ConditionTypeScaleOperationComplete,
				Status: druidv1alpha1.ConditionFalse,
				Reason: druidv1alpha1.ScaleOperationReasonScalingIn,
			},
			wantConditionPresent: true,
			wantStatus:           druidv1alpha1.ConditionTrue,
			wantReason:           druidv1alpha1.ScaleOperationReasonNoScaleOperation,
		},
		{
			name:                 "no condition recorded -> success does not add one",
			existingCondition:    nil,
			wantConditionPresent: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			etcd := testutils.EtcdBuilderWithoutDefaults(scaleTestEtcdName, scaleTestNamespace).
				WithReplicas(3).
				Build()
			etcd.UID = scaleTestEtcdUID
			if tc.existingCondition != nil {
				etcd.Status.Conditions = []druidv1alpha1.Condition{*tc.existingCondition}
			}

			cl := fakeclient.NewClientBuilder().
				WithScheme(clientkubernetes.Scheme).
				WithObjects(etcd).
				WithStatusSubresource(etcd).
				Build()
			r := &Reconciler{
				client:            cl,
				lastOpErrRecorder: ctrlutils.NewLastOperationAndLastErrorsRecorder(cl, logr.Discard()),
			}

			res := r.recordReconcileSuccessOperation(newScaleTestOperatorContext(), etcd)
			g.Expect(res.HasErrors()).To(BeFalse())

			persisted := &druidv1alpha1.Etcd{}
			g.Expect(cl.Get(context.Background(), types.NamespacedName{Name: scaleTestEtcdName, Namespace: scaleTestNamespace}, persisted)).To(Succeed())

			idx := findScaleCondition(persisted.Status.Conditions)
			if !tc.wantConditionPresent {
				g.Expect(idx).To(BeNumerically("<", 0))
				return
			}
			g.Expect(idx).To(BeNumerically(">=", 0))
			cond := persisted.Status.Conditions[idx]
			g.Expect(cond.Status).To(Equal(tc.wantStatus))
			g.Expect(cond.Reason).To(Equal(tc.wantReason))
		})
	}
}

func findScaleCondition(conditions []druidv1alpha1.Condition) int {
	for i, c := range conditions {
		if c.Type == druidv1alpha1.ConditionTypeScaleOperationComplete {
			return i
		}
	}
	return -1
}

func newScaleTestOperatorContext() component.OperatorContext {
	return component.NewOperatorContext(context.Background(), logr.Discard(), "test-run")
}
