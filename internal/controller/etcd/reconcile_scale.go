// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package etcd

import (
	"fmt"
	"slices"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/component"
	ctrlutils "github.com/gardener/etcd-druid/internal/controller/utils"
	"github.com/gardener/etcd-druid/internal/utils/kubernetes"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// detectAndRecordScaleOperationInProgress records an in-progress scale operation
// in the ScaleOperationComplete status condition (False = in progress). It runs
// before component reconciliation so the CEL admission rules can reject conflicting
// opposite-direction changes while an operation is active. It only sets False;
// recordScaleOperationComplete advances it to True on completion.
func (r *Reconciler) detectAndRecordScaleOperationInProgress(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) ctrlutils.ReconcileStepResult {
	desired, err := r.determineScaleOperationInProgress(ctx, etcd)
	if err != nil {
		ctx.Logger.Error(err, "failed to determine scale operation; requeuing")
		return ctrlutils.ReconcileWithError(err)
	}
	if desired == nil || !scaleConditionNeedsUpdate(etcd, desired) {
		return ctrlutils.ContinueReconcile()
	}

	ctx.Logger.Info("recording scale operation in progress", "reason", desired.Reason)
	if err := r.patchScaleOperationCondition(ctx, etcd, desired); err != nil {
		ctx.Logger.Error(err, "failed to record ScaleOperationComplete condition")
		return ctrlutils.ReconcileWithError(err)
	}
	return ctrlutils.ContinueReconcile()
}

// determineScaleOperationInProgress returns the ScaleOperationComplete condition
// (False) for the scale operation in progress, with the reason ScalingIn,
// ScalingOut, or BootstrapMembersRemoval. BootstrapMembersRemoval takes precedence
// when it applies together with a replica change. It returns nil when no scale
// operation is in progress, leaving the condition as is;
// recordScaleOperationComplete sets it to True once the operation finishes. It
// returns an error when the StatefulSet cannot be read, so the caller requeues
// and retries rather than acting on a stale condition.
func (r *Reconciler) determineScaleOperationInProgress(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) (*druidv1alpha1.Condition, error) {
	// The operator has removed joined source members (or unset
	// bootstrapWithExistingCluster) that the target had already joined. Only
	// members recorded in status are meaningful here.
	if druidv1alpha1.HasBootstrapMembersToDecommission(etcd) {
		return newScaleOperationCondition(druidv1alpha1.ConditionFalse, druidv1alpha1.ScaleOperationReasonBootstrapMembersRemoval), nil
	}

	sts, err := kubernetes.GetStatefulSet(ctx, r.client, etcd)
	if err != nil {
		return nil, fmt.Errorf("failed to get StatefulSet while detecting scale operation for %v: %w",
			client.ObjectKeyFromObject(etcd), err)
	}
	// No observed replica count to compare against.
	if sts == nil || sts.Spec.Replicas == nil {
		return nil, nil
	}

	// Transitions to or from zero replicas are handled by the existing
	// scale-to-zero code path and are never treated as a scale-in/out.
	stsReplicas := *sts.Spec.Replicas
	if etcd.Spec.Replicas == 0 || stsReplicas == 0 {
		return nil, nil
	}

	switch {
	case etcd.Spec.Replicas < stsReplicas:
		return newScaleOperationCondition(druidv1alpha1.ConditionFalse, druidv1alpha1.ScaleOperationReasonScalingIn), nil
	case etcd.Spec.Replicas > stsReplicas:
		return newScaleOperationCondition(druidv1alpha1.ConditionFalse, druidv1alpha1.ScaleOperationReasonScalingOut), nil
	default:
		return nil, nil
	}
}

// newScaleOperationCondition returns a ScaleOperationComplete condition with the
// given status and reason, and the message for that reason.
func newScaleOperationCondition(status druidv1alpha1.ConditionStatus, reason string) *druidv1alpha1.Condition {
	return &druidv1alpha1.Condition{
		Type:    druidv1alpha1.ConditionTypeScaleOperationComplete,
		Status:  status,
		Reason:  reason,
		Message: scaleOperationMessage(reason),
	}
}

// scaleConditionNeedsUpdate reports whether the recorded ScaleOperationComplete
// condition differs from desired in status or reason, avoiding a redundant status
// patch every reconcile.
func scaleConditionNeedsUpdate(etcd *druidv1alpha1.Etcd, desired *druidv1alpha1.Condition) bool {
	existing := druidv1alpha1.GetScaleOperationCompleteCondition(etcd)
	return existing == nil || existing.Status != desired.Status || existing.Reason != desired.Reason
}

// patchScaleOperationCondition upserts desired as the ScaleOperationComplete
// condition on the Etcd status sub-resource.
func (r *Reconciler) patchScaleOperationCondition(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd, desired *druidv1alpha1.Condition) error {
	originalEtcd := etcd.DeepCopy()
	upsertScaleOperationCondition(etcd, desired)
	return r.client.Status().Patch(ctx, etcd, client.MergeFrom(originalEtcd))
}

// upsertScaleOperationCondition sets (or inserts) desired as the
// ScaleOperationComplete condition on etcd.Status.Conditions. LastTransitionTime
// advances only when the status value changes; LastUpdateTime always advances.
func upsertScaleOperationCondition(etcd *druidv1alpha1.Etcd, desired *druidv1alpha1.Condition) {
	now := metav1.Now()
	cond := druidv1alpha1.GetScaleOperationCompleteCondition(etcd)
	if cond == nil {
		added := *desired
		added.LastTransitionTime = now
		added.LastUpdateTime = now
		etcd.Status.Conditions = append(etcd.Status.Conditions, added)
		return
	}

	if cond.Status != desired.Status {
		cond.LastTransitionTime = now
	}
	cond.Status = desired.Status
	cond.LastUpdateTime = now
	cond.Reason = desired.Reason
	cond.Message = desired.Message
}

// scaleOperationMessage returns a human-readable message for the given reason.
func scaleOperationMessage(reason string) string {
	switch reason {
	case druidv1alpha1.ScaleOperationReasonScalingIn:
		return "A scale-in of the etcd cluster is in progress."
	case druidv1alpha1.ScaleOperationReasonScalingOut:
		return "A scale-out of the etcd cluster is in progress."
	case druidv1alpha1.ScaleOperationReasonBootstrapMembersRemoval:
		return "Removal of source members joined via bootstrapWithExistingCluster is in progress."
	default:
		return "No scale operation is in progress."
	}
}

// pruneBootstrapMembersStatus removes, from
// etcd.Status.BootstrapWithExistingCluster.Members, any joined source member
// that is no longer present in spec.etcd.bootstrapWithExistingCluster; these
// members have been removed from the etcd cluster by the PreSync member removal
// (which requeues until no surplus remains, so by the time this runs the
// removal has completed). When all joined members have been pruned, the whole
// status field is cleared. It is a no-op when nothing needs pruning.
func (r *Reconciler) pruneBootstrapMembersStatus(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) ctrlutils.ReconcileStepResult {
	decommissioned := druidv1alpha1.GetBootstrapMemberNamesToDecommission(etcd)
	if len(decommissioned) == 0 {
		return ctrlutils.ContinueReconcile()
	}

	statusBootstrap := etcd.Status.BootstrapWithExistingCluster
	retained := make([]druidv1alpha1.BootstrapJoinedMember, 0, len(statusBootstrap.Members))
	for _, joined := range statusBootstrap.Members {
		if !slices.Contains(decommissioned, joined.Name) {
			retained = append(retained, joined)
		}
	}

	originalEtcd := etcd.DeepCopy()
	if len(retained) == 0 {
		etcd.Status.BootstrapWithExistingCluster = nil
	} else {
		etcd.Status.BootstrapWithExistingCluster.Members = retained
	}
	ctx.Logger.Info("pruning removed source members from bootstrapWithExistingCluster status",
		"retained", len(retained), "removed", len(statusBootstrap.Members)-len(retained))
	if err := r.client.Status().Patch(ctx, etcd, client.MergeFrom(originalEtcd)); err != nil {
		ctx.Logger.Error(err, "failed to prune bootstrapWithExistingCluster status")
		return ctrlutils.ReconcileWithError(err)
	}
	return ctrlutils.ContinueReconcile()
}
