// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package etcd

import (
	"context"
	"fmt"
	"testing"
	"time"

	druidapicommon "github.com/gardener/etcd-druid/api/common"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	etcdclientfake "github.com/gardener/etcd-druid/internal/client/etcd/fake"
	clientkubernetes "github.com/gardener/etcd-druid/internal/client/kubernetes"
	"github.com/gardener/etcd-druid/internal/component"
	"github.com/gardener/etcd-druid/internal/component/statefulset"
	ctrlutils "github.com/gardener/etcd-druid/internal/controller/utils"
	druiderr "github.com/gardener/etcd-druid/internal/errors"
	etcdmember "github.com/gardener/etcd-druid/internal/etcd"
	testutils "github.com/gardener/etcd-druid/test/utils"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

// TestDetermineScaleOperationInProgress verifies that determineScaleOperationInProgress
// returns the False condition for the in-progress scale operation across scale-in,
// scale-out, bootstrap-removal, zero-replica, and no-op scenarios; no-op cases return nil.
func TestDetermineScaleOperationInProgress(t *testing.T) {
	t.Parallel()
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
		expectedReason  string
	}{
		{
			name:         "no StatefulSet yet (fresh cluster) -> nil",
			specReplicas: 3,
			stsReplicas:  nil,
		},
		{
			name:           "spec < observed -> ScalingIn",
			specReplicas:   3,
			stsReplicas:    ptr.To(int32(5)),
			expectedReason: druidv1alpha1.ScaleOperationReasonScalingIn,
		},
		{
			name:           "spec > observed -> ScalingOut",
			specReplicas:   5,
			stsReplicas:    ptr.To(int32(3)),
			expectedReason: druidv1alpha1.ScaleOperationReasonScalingOut,
		},
		{
			name:         "spec == observed -> nil",
			specReplicas: 3,
			stsReplicas:  ptr.To(int32(3)),
		},
		{
			name:         "spec.replicas == 0 -> nil, not ScalingIn",
			specReplicas: 0,
			stsReplicas:  ptr.To(int32(3)),
		},
		{
			name:         "observed == 0 (wake-up) -> nil, not ScalingOut",
			specReplicas: 3,
			stsReplicas:  ptr.To(int32(0)),
		},
		{
			name:            "joined member no longer in spec -> BootstrapMembersRemoval",
			specReplicas:    3,
			stsReplicas:     ptr.To(int32(3)),
			statusBootstrap: bootstrapStatus(scaleTestBootstrapA, scaleTestBootstrapB),
			specBootstrap:   specBootstrap(scaleTestBootstrapA),
			expectedReason:  druidv1alpha1.ScaleOperationReasonBootstrapMembersRemoval,
		},
		{
			name:            "spec bootstrap unset but members joined -> BootstrapMembersRemoval",
			specReplicas:    3,
			stsReplicas:     ptr.To(int32(3)),
			statusBootstrap: bootstrapStatus(scaleTestBootstrapA),
			specBootstrap:   nil,
			expectedReason:  druidv1alpha1.ScaleOperationReasonBootstrapMembersRemoval,
		},
		{
			name:            "all joined members still in spec -> falls through to replicas comparison",
			specReplicas:    3,
			stsReplicas:     ptr.To(int32(3)),
			statusBootstrap: bootstrapStatus(scaleTestBootstrapA),
			specBootstrap:   specBootstrap(scaleTestBootstrapA),
		},
		{
			name:            "BootstrapMembersRemoval takes precedence over a concurrent scale-out",
			specReplicas:    5,
			stsReplicas:     ptr.To(int32(3)),
			statusBootstrap: bootstrapStatus(scaleTestBootstrapA, scaleTestBootstrapB),
			specBootstrap:   specBootstrap(scaleTestBootstrapA),
			expectedReason:  druidv1alpha1.ScaleOperationReasonBootstrapMembersRemoval,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
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

			got, err := r.determineScaleOperationInProgress(newScaleTestOperatorContext(), etcd)
			g.Expect(err).NotTo(HaveOccurred())
			if tc.expectedReason == "" {
				g.Expect(got).To(BeNil())
				return
			}
			g.Expect(got).NotTo(BeNil())
			g.Expect(got.Type).To(Equal(druidv1alpha1.ConditionTypeScaleOperationComplete))
			g.Expect(got.Status).To(Equal(druidv1alpha1.ConditionFalse))
			g.Expect(got.Reason).To(Equal(tc.expectedReason))
			g.Expect(got.Message).NotTo(BeEmpty())
		})
	}
}

// TestDetermineScaleOperationInProgressErrorsOnStatefulSetGetError verifies that a
// StatefulSet Get failure is returned as an error, so the caller requeues and
// retries instead of acting on a stale scale condition.
func TestDetermineScaleOperationInProgressErrorsOnStatefulSetGetError(t *testing.T) {
	t.Parallel()
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

	_, err := r.determineScaleOperationInProgress(newScaleTestOperatorContext(), etcd)
	g.Expect(err).To(HaveOccurred())
}

// TestDetectAndRecordScaleOperationInProgress exercises the full reconcile step: it must
// patch the ScaleOperationComplete condition onto the Etcd status subresource
// when there is an in-progress operation to record, and it must be a no-op (no condition
// added) for a brand-new resource with no scale operation in progress.
func TestDetectAndRecordScaleOperationInProgress(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		specReplicas int32
		stsReplicas  *int32
		// statusBootstrap and specBootstrap drive the BootstrapMembersRemoval path.
		statusBootstrap *druidv1alpha1.BootstrapWithExistingClusterStatus
		specBootstrap   *druidv1alpha1.BootstrapWithExistingCluster
		// wantConditionPresent indicates whether the ScaleOperationComplete
		// condition should exist on the status after the step runs.
		wantConditionPresent bool
		expectedStatus       druidv1alpha1.ConditionStatus
		expectedReason       string
	}{
		{
			name:                 "scale-in records the condition",
			specReplicas:         3,
			stsReplicas:          ptr.To(int32(5)),
			wantConditionPresent: true,
			expectedStatus:       druidv1alpha1.ConditionFalse,
			expectedReason:       druidv1alpha1.ScaleOperationReasonScalingIn,
		},
		{
			name:                 "scale-out records the condition",
			specReplicas:         5,
			stsReplicas:          ptr.To(int32(3)),
			wantConditionPresent: true,
			expectedStatus:       druidv1alpha1.ConditionFalse,
			expectedReason:       druidv1alpha1.ScaleOperationReasonScalingOut,
		},
		{
			name:         "bootstrap decommission candidates present records BootstrapMembersRemoval",
			specReplicas: 3,
			stsReplicas:  ptr.To(int32(3)),
			statusBootstrap: &druidv1alpha1.BootstrapWithExistingClusterStatus{Members: []druidv1alpha1.BootstrapJoinedMember{
				{Name: scaleTestBootstrapA},
				{Name: scaleTestBootstrapB},
			}},
			specBootstrap: &druidv1alpha1.BootstrapWithExistingCluster{Members: []druidv1alpha1.BootstrapExistingMember{
				{Name: scaleTestBootstrapA},
			}},
			wantConditionPresent: true,
			expectedStatus:       druidv1alpha1.ConditionFalse,
			expectedReason:       druidv1alpha1.ScaleOperationReasonBootstrapMembersRemoval,
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
			t.Parallel()
			g := NewWithT(t)

			etcd := testutils.EtcdBuilderWithoutDefaults(scaleTestEtcdName, scaleTestNamespace).
				WithReplicas(tc.specReplicas).
				Build()
			etcd.UID = scaleTestEtcdUID
			etcd.Spec.Etcd.BootstrapWithExistingCluster = tc.specBootstrap
			etcd.Status.BootstrapWithExistingCluster = tc.statusBootstrap

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

			res := r.detectAndRecordScaleOperationInProgress(newScaleTestOperatorContext(), etcd)
			g.Expect(res.HasErrors()).To(BeFalse())

			// Re-read the persisted Etcd to assert the condition was patched onto
			// the status subresource (not just mutated in memory).
			persisted := &druidv1alpha1.Etcd{}
			g.Expect(cl.Get(context.Background(), types.NamespacedName{Name: scaleTestEtcdName, Namespace: scaleTestNamespace}, persisted)).To(Succeed())

			cond := druidv1alpha1.GetScaleOperationCompleteCondition(persisted)
			if !tc.wantConditionPresent {
				g.Expect(cond).To(BeNil(), "no ScaleOperationComplete condition should be recorded")
				return
			}
			g.Expect(cond).NotTo(BeNil(), "ScaleOperationComplete condition should be recorded")
			g.Expect(cond.Status).To(Equal(tc.expectedStatus))
			g.Expect(cond.Reason).To(Equal(tc.expectedReason))
			g.Expect(cond.LastTransitionTime.IsZero()).To(BeFalse())
			g.Expect(cond.Message).NotTo(BeEmpty())
		})
	}
}

// TestDetectAndRecordScaleOperationInProgressIsIdempotent verifies that a second call to
// detectAndRecordScaleOperationInProgress with the same state issues no Status().Patch.
// The guard in scaleConditionNeedsUpdate should short-circuit when the existing
// condition already matches the determined condition.
func TestDetectAndRecordScaleOperationInProgressIsIdempotent(t *testing.T) {
	t.Parallel()
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
	res := r.detectAndRecordScaleOperationInProgress(newScaleTestOperatorContext(), etcd)
	g.Expect(res.HasErrors()).To(BeFalse())
	g.Expect(patchCount).To(Equal(0), "no Status().Patch should be issued when condition already matches")
}

// TestNewScaleOperationCondition verifies that newScaleOperationCondition builds a
// ScaleOperationComplete condition with the given status and reason and the
// message for that reason.
func TestNewScaleOperationCondition(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name            string
		status          druidv1alpha1.ConditionStatus
		reason          string
		expectedMessage string
	}{
		{
			name:            "scale-in",
			status:          druidv1alpha1.ConditionFalse,
			reason:          druidv1alpha1.ScaleOperationReasonScalingIn,
			expectedMessage: "A scale-in of the etcd cluster is in progress.",
		},
		{
			name:            "scale-out",
			status:          druidv1alpha1.ConditionFalse,
			reason:          druidv1alpha1.ScaleOperationReasonScalingOut,
			expectedMessage: "A scale-out of the etcd cluster is in progress.",
		},
		{
			name:            "bootstrap members removal",
			status:          druidv1alpha1.ConditionFalse,
			reason:          druidv1alpha1.ScaleOperationReasonBootstrapMembersRemoval,
			expectedMessage: "Removal of source members joined via bootstrapWithExistingCluster is in progress.",
		},
		{
			name:            "completed",
			status:          druidv1alpha1.ConditionTrue,
			reason:          druidv1alpha1.ScaleOperationReasonNoScaleOperation,
			expectedMessage: "No scale operation is in progress.",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g := NewWithT(t)
			cond := newScaleOperationCondition(tc.status, tc.reason)
			g.Expect(cond).To(Equal(&druidv1alpha1.Condition{
				Type:    druidv1alpha1.ConditionTypeScaleOperationComplete,
				Status:  tc.status,
				Reason:  tc.reason,
				Message: tc.expectedMessage,
			}))
		})
	}
}

// TestScaleConditionNeedsUpdate verifies that scaleConditionNeedsUpdate reports an
// update only when the recorded ScaleOperationComplete condition is absent or
// differs from the desired condition in status or reason.
func TestScaleConditionNeedsUpdate(t *testing.T) {
	t.Parallel()
	scalingIn := newScaleOperationCondition(druidv1alpha1.ConditionFalse, druidv1alpha1.ScaleOperationReasonScalingIn)
	tests := []struct {
		name           string
		existing       *druidv1alpha1.Condition
		desired        *druidv1alpha1.Condition
		expectedUpdate bool
	}{
		{
			name:           "no condition recorded",
			existing:       nil,
			desired:        scalingIn,
			expectedUpdate: true,
		},
		{
			name:           "same status and reason",
			existing:       newScaleOperationCondition(druidv1alpha1.ConditionFalse, druidv1alpha1.ScaleOperationReasonScalingIn),
			desired:        scalingIn,
			expectedUpdate: false,
		},
		{
			name:           "same status, different reason",
			existing:       newScaleOperationCondition(druidv1alpha1.ConditionFalse, druidv1alpha1.ScaleOperationReasonScalingOut),
			desired:        scalingIn,
			expectedUpdate: true,
		},
		{
			name:           "different status",
			existing:       newScaleOperationCondition(druidv1alpha1.ConditionTrue, druidv1alpha1.ScaleOperationReasonNoScaleOperation),
			desired:        scalingIn,
			expectedUpdate: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g := NewWithT(t)
			etcd := testutils.EtcdBuilderWithoutDefaults(scaleTestEtcdName, scaleTestNamespace).Build()
			if tc.existing != nil {
				etcd.Status.Conditions = []druidv1alpha1.Condition{*tc.existing}
			}
			g.Expect(scaleConditionNeedsUpdate(etcd, tc.desired)).To(Equal(tc.expectedUpdate))
		})
	}
}

// TestUpsertScaleOperationCondition verifies that upsertScaleOperationCondition
// appends the condition when absent and updates it in place otherwise, advancing
// LastTransitionTime only when the status changes and leaving other conditions
// untouched.
func TestUpsertScaleOperationCondition(t *testing.T) {
	t.Parallel()
	past := metav1.NewTime(metav1.Now().Add(-time.Hour))
	otherCondition := druidv1alpha1.Condition{Type: druidv1alpha1.ConditionTypeReady, Status: druidv1alpha1.ConditionTrue}
	tests := []struct {
		name                   string
		existing               *druidv1alpha1.Condition
		desired                *druidv1alpha1.Condition
		expectTransitionUpdate bool
	}{
		{
			name:                   "absent condition is appended",
			existing:               nil,
			desired:                newScaleOperationCondition(druidv1alpha1.ConditionFalse, druidv1alpha1.ScaleOperationReasonScalingIn),
			expectTransitionUpdate: true,
		},
		{
			name: "status change advances LastTransitionTime",
			existing: &druidv1alpha1.Condition{
				Type:               druidv1alpha1.ConditionTypeScaleOperationComplete,
				Status:             druidv1alpha1.ConditionFalse,
				Reason:             druidv1alpha1.ScaleOperationReasonScalingIn,
				LastTransitionTime: past,
				LastUpdateTime:     past,
			},
			desired:                newScaleOperationCondition(druidv1alpha1.ConditionTrue, druidv1alpha1.ScaleOperationReasonNoScaleOperation),
			expectTransitionUpdate: true,
		},
		{
			name: "reason change with the same status keeps LastTransitionTime",
			existing: &druidv1alpha1.Condition{
				Type:               druidv1alpha1.ConditionTypeScaleOperationComplete,
				Status:             druidv1alpha1.ConditionFalse,
				Reason:             druidv1alpha1.ScaleOperationReasonScalingOut,
				LastTransitionTime: past,
				LastUpdateTime:     past,
			},
			desired:                newScaleOperationCondition(druidv1alpha1.ConditionFalse, druidv1alpha1.ScaleOperationReasonScalingIn),
			expectTransitionUpdate: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g := NewWithT(t)
			etcd := testutils.EtcdBuilderWithoutDefaults(scaleTestEtcdName, scaleTestNamespace).Build()
			etcd.Status.Conditions = []druidv1alpha1.Condition{otherCondition}
			if tc.existing != nil {
				etcd.Status.Conditions = append(etcd.Status.Conditions, *tc.existing)
			}

			upsertScaleOperationCondition(etcd, tc.desired)

			g.Expect(etcd.Status.Conditions).To(HaveLen(2))
			g.Expect(etcd.Status.Conditions[0]).To(Equal(otherCondition))
			cond := druidv1alpha1.GetScaleOperationCompleteCondition(etcd)
			g.Expect(cond).NotTo(BeNil())
			g.Expect(cond.Status).To(Equal(tc.desired.Status))
			g.Expect(cond.Reason).To(Equal(tc.desired.Reason))
			g.Expect(cond.Message).To(Equal(tc.desired.Message))
			g.Expect(cond.LastUpdateTime.After(past.Time)).To(BeTrue())
			if tc.expectTransitionUpdate {
				g.Expect(cond.LastTransitionTime.After(past.Time)).To(BeTrue())
			} else {
				g.Expect(cond.LastTransitionTime).To(Equal(past))
			}
		})
	}
}

// TestPruneBootstrapMembersStatus verifies that joined source members no longer
// present in spec.etcd.bootstrapWithExistingCluster are dropped from
// status.bootstrapWithExistingCluster.members, that the whole status field is
// cleared once nothing remains, and that it is a no-op when every joined member
// is still in spec (or nothing has joined at all).
func TestPruneBootstrapMembersStatus(t *testing.T) {
	t.Parallel()
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
			t.Parallel()
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

// TestRecordScaleOperationComplete verifies that an in-flight
// ScaleOperationComplete condition advances to True/NoScaleOperation once all
// surplus members are gone, requeues while surplus members remain, and is a
// no-op when no scale operation condition is recorded.
func TestRecordScaleOperationComplete(t *testing.T) {
	t.Parallel()
	const replicas = int32(3)

	// buildMembers returns only the Etcd's own members for [0, replicas).
	buildCleanMembers := func(name string) []etcdmember.Member {
		members := make([]etcdmember.Member, 0, replicas)
		for i := range replicas {
			members = append(members, etcdmember.Member{
				ID:     uint64(i) + 1, // #nosec G115
				Name:   fmt.Sprintf("%s-%d", name, i),
				Health: etcdmember.MemberHealthHealthy,
			})
		}
		return members
	}

	// buildSurplusMembers adds one extra member beyond the expected set.
	buildSurplusMembers := func(name string) []etcdmember.Member {
		members := buildCleanMembers(name)
		return append(members, etcdmember.Member{
			ID:   uint64(replicas) + 1,
			Name: fmt.Sprintf("%s-%d", name, replicas), // one beyond spec
		})
	}

	tests := []struct {
		name string
		// existingCondition, when non-nil, seeds a ScaleOperationComplete
		// condition on the status before the success step runs.
		existingCondition *druidv1alpha1.Condition
		// fakeMembers sets the members the fake etcd client returns.
		// A nil slice means no client call is expected (no scale operation condition).
		fakeMembers          []etcdmember.Member
		wantConditionPresent bool
		expectedStatus       druidv1alpha1.ConditionStatus
		expectedReason       string
		wantRequeue          bool
	}{
		{
			name: "in-flight scale-in with all surplus removed: condition advances to True",
			existingCondition: &druidv1alpha1.Condition{
				Type:   druidv1alpha1.ConditionTypeScaleOperationComplete,
				Status: druidv1alpha1.ConditionFalse,
				Reason: druidv1alpha1.ScaleOperationReasonScalingIn,
			},
			fakeMembers:          buildCleanMembers(scaleTestEtcdName),
			wantConditionPresent: true,
			expectedStatus:       druidv1alpha1.ConditionTrue,
			expectedReason:       druidv1alpha1.ScaleOperationReasonNoScaleOperation,
		},
		{
			name: "in-flight scale-in with surplus still present: requeues without advancing condition",
			existingCondition: &druidv1alpha1.Condition{
				Type:   druidv1alpha1.ConditionTypeScaleOperationComplete,
				Status: druidv1alpha1.ConditionFalse,
				Reason: druidv1alpha1.ScaleOperationReasonScalingIn,
			},
			fakeMembers:          buildSurplusMembers(scaleTestEtcdName),
			wantConditionPresent: true,
			expectedStatus:       druidv1alpha1.ConditionFalse,
			expectedReason:       druidv1alpha1.ScaleOperationReasonScalingIn,
			wantRequeue:          true,
		},
		{
			name:                 "no condition recorded -> success does not add one",
			existingCondition:    nil,
			fakeMembers:          nil, // no client call expected
			wantConditionPresent: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g := NewWithT(t)

			etcd := testutils.EtcdBuilderWithoutDefaults(scaleTestEtcdName, scaleTestNamespace).
				WithReplicas(replicas).
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

			var cf *etcdclientfake.Factory
			if tc.fakeMembers != nil {
				fc := &etcdclientfake.Client{Members: tc.fakeMembers}
				cf = &etcdclientfake.Factory{Client: fc}
			} else {
				cf = &etcdclientfake.Factory{}
			}

			r := &Reconciler{
				client:            cl,
				lastOpErrRecorder: ctrlutils.NewLastOperationAndLastErrorsRecorder(cl, logr.Discard()),
				etcdClientFactory: cf,
			}

			res := r.recordScaleOperationComplete(newScaleTestOperatorContext(), etcd)

			if tc.wantRequeue {
				g.Expect(res.HasErrors()).To(BeTrue())
				g.Expect(res.GetResult().Requeue).To(BeTrue())
			} else {
				g.Expect(res.HasErrors()).To(BeFalse())
			}

			persisted := &druidv1alpha1.Etcd{}
			g.Expect(cl.Get(context.Background(), types.NamespacedName{Name: scaleTestEtcdName, Namespace: scaleTestNamespace}, persisted)).To(Succeed())

			cond := druidv1alpha1.GetScaleOperationCompleteCondition(persisted)
			if !tc.wantConditionPresent {
				g.Expect(cond).To(BeNil())
				return
			}
			g.Expect(cond).NotTo(BeNil())
			g.Expect(cond.Status).To(Equal(tc.expectedStatus))
			g.Expect(cond.Reason).To(Equal(tc.expectedReason))
		})
	}
}

func newScaleTestOperatorContext() component.OperatorContext {
	return component.NewOperatorContext(context.Background(), logr.Discard(), "test-run")
}

// quorumUnsafePreSyncOperator is a component.Operator test double that models the
// scale-in member-removal PreSync on a quorum-unsafe hold: it returns a DruidError
// carrying ERR_QUORUM_UNSAFE_MEMBER_REMOVAL and does NOT touch status itself (the
// reconcile flow is responsible for recording the reason). All other methods are
// no-ops. It lets the test drive the real reconcile step sequence
// (preSyncEtcdResources -> mapping -> recordIncompleteReconcileOperation) without a
// live etcd or the full StatefulSet operator.
type quorumUnsafePreSyncOperator struct {
	returnErr error
}

func (o quorumUnsafePreSyncOperator) GetExistingResourceNames(_ component.OperatorContext, _ metav1.ObjectMeta) ([]string, error) {
	return nil, nil
}

func (o quorumUnsafePreSyncOperator) TriggerDelete(_ component.OperatorContext, _ metav1.ObjectMeta) error {
	return nil
}

func (o quorumUnsafePreSyncOperator) Sync(_ component.OperatorContext, _ *druidv1alpha1.Etcd) error {
	return nil
}

func (o quorumUnsafePreSyncOperator) PreSync(_ component.OperatorContext, _ *druidv1alpha1.Etcd) error {
	return o.returnErr
}

// TestQuorumUnsafeErrorSurvivesReconcileRecording is the regression test for the
// observability defect where a held, quorum-unsafe scale-in must surface
// ERR_QUORUM_UNSAFE_MEMBER_REMOVAL in status.lastErrors so an operator can see why
// the scale-in is stuck. It drives the real reconcile step sequence:
// preSyncEtcdResources (which maps the returned DruidError to a step result) followed
// by recordIncompleteReconcileOperation (which persists status.lastErrors and
// LastOperation.State via the real recorder).
//
// Two invariants must hold after the sequence:
//  1. the quorum-unsafe error is present in status.lastErrors and LastOperation.State
//     is Error (observability); and
//  2. the step result requeues after an interval (a transient hold) rather than as an
//     immediate error requeue, i.e. the mapping treats the quorum-unsafe code as a
//     recording requeue, not a generic terminal error.
func TestQuorumUnsafeErrorSurvivesReconcileRecording(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	etcd := testutils.EtcdBuilderWithoutDefaults(scaleTestEtcdName, scaleTestNamespace).
		WithReplicas(3).
		Build()
	etcd.UID = scaleTestEtcdUID
	etcd.Status.LastOperation = &druidapicommon.LastOperation{
		Type:  druidv1alpha1.LastOperationTypeReconcile,
		State: druidv1alpha1.LastOperationStateProcessing,
	}

	cl := fakeclient.NewClientBuilder().
		WithScheme(clientkubernetes.Scheme).
		WithObjects(etcd).
		WithStatusSubresource(etcd).
		Build()

	// The quorum-unsafe hold requeues after the retry interval; the DruidError it
	// carries names the quorum-unsafe code so the reconcile flow records the reason.
	quorumErr := druiderr.New(statefulset.ErrQuorumUnsafeMemberRemoval, component.OperationPreSync,
		"member etcd-scale-2 not removed: removal would break quorum for etcd test-ns/etcd-scale")

	registry := component.NewRegistry()
	registry.Register(component.StatefulSetKind, quorumUnsafePreSyncOperator{returnErr: quorumErr})

	r := &Reconciler{
		client:            cl,
		operatorRegistry:  registry,
		lastOpErrRecorder: ctrlutils.NewLastOperationAndLastErrorsRecorder(cl, logr.Discard()),
	}

	ctx := newScaleTestOperatorContext()

	// preSyncEtcdResources maps the quorum-unsafe DruidError to a step result that
	// short-circuits the reconcile flow.
	stepResult := r.preSyncEtcdResources(ctx, etcd)
	g.Expect(ctrlutils.ShortCircuitReconcileFlow(stepResult)).To(BeTrue())

	// A quorum-unsafe hold is transient: it must requeue after an interval, not as an
	// immediate error requeue. This distinguishes the recording-requeue mapping from
	// the generic terminal-error path.
	g.Expect(stepResult.GetResult().RequeueAfter).To(BeNumerically(">", 0),
		"quorum-unsafe hold must requeue after an interval")

	// recordIncompleteReconcileOperation persists the exit step result's errors and
	// state. The quorum-unsafe error must survive so an operator can see why the
	// scale-in is held.
	r.recordIncompleteReconcileOperation(ctx, etcd, stepResult)

	persisted := &druidv1alpha1.Etcd{}
	g.Expect(cl.Get(context.Background(), types.NamespacedName{Name: scaleTestEtcdName, Namespace: scaleTestNamespace}, persisted)).To(Succeed())

	g.Expect(hasScaleLastError(persisted, statefulset.ErrQuorumUnsafeMemberRemoval)).To(BeTrue(),
		"quorum-unsafe error must survive the reconcile recording so an operator can see why the scale-in is held")
	g.Expect(persisted.Status.LastOperation).NotTo(BeNil())
	g.Expect(persisted.Status.LastOperation.State).To(Equal(druidv1alpha1.LastOperationStateError))
}

// hasScaleLastError reports whether the Etcd status carries a LastError with code.
func hasScaleLastError(etcd *druidv1alpha1.Etcd, code druidapicommon.ErrorCode) bool {
	for _, le := range etcd.Status.LastErrors {
		if le.Code == code {
			return true
		}
	}
	return false
}
