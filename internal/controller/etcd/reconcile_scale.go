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
	status, reason, err := r.determineScaleOperationInProgress(ctx, etcd)
	if err != nil {
		ctx.Logger.Error(err, "failed to determine scale operation; requeuing")
		return ctrlutils.ReconcileWithError(err)
	}

	if !scaleConditionNeedsUpdate(etcd, status, reason) {
		return ctrlutils.ContinueReconcile()
	}

	ctx.Logger.Info("recording scale operation condition", "status", status, "reason", reason)
	if err := r.patchScaleOperationCondition(ctx, etcd, status, reason); err != nil {
		ctx.Logger.Error(err, "failed to record ScaleOperationComplete condition")
		return ctrlutils.ReconcileWithError(err)
	}
	return ctrlutils.ContinueReconcile()
}

// determineScaleOperationInProgress reports the ScaleOperationComplete status and
// reason for the etcd. When a scale operation is in progress it returns False with
// the reason ScalingIn, ScalingOut, or BootstrapMembersRemoval; BootstrapMembersRemoval
// takes precedence when it applies together with a replica change. Otherwise it returns
// the existing condition unchanged; recordScaleOperationComplete sets the condition to
// True once the operation finishes. It returns an error when the StatefulSet cannot be
// read, so the caller requeues and retries rather than acting on a stale condition.
func (r *Reconciler) determineScaleOperationInProgress(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) (druidv1alpha1.ConditionStatus, string, error) {
	// BootstrapMembersRemoval takes precedence: the operator has removed joined
	// source members (or unset bootstrapWithExistingCluster) that the target had
	// already joined. Only members recorded in status are meaningful here.
	if druidv1alpha1.HasBootstrapMembersToDecommission(etcd) {
		return druidv1alpha1.ConditionFalse, druidv1alpha1.ScaleOperationReasonBootstrapMembersRemoval, nil
	}

	sts, err := kubernetes.GetStatefulSet(ctx, r.client, etcd)
	if err != nil {
		return "", "", fmt.Errorf("failed to get StatefulSet while detecting scale operation for %v: %w",
			client.ObjectKeyFromObject(etcd), err)
	}
	// No observed replica count to compare against; preserve the existing
	// condition rather than asserting no scale operation is in progress.
	if sts == nil || sts.Spec.Replicas == nil {
		status, reason := druidv1alpha1.GetScaleOperationCompleteCondition(etcd)
		return status, reason, nil
	}

	// Transitions to or from zero replicas are handled by the existing
	// scale-to-zero code path and are never treated as a scale-in/out.
	stsReplicas := *sts.Spec.Replicas
	if etcd.Spec.Replicas == 0 || stsReplicas == 0 {
		status, reason := druidv1alpha1.GetScaleOperationCompleteCondition(etcd)
		return status, reason, nil
	}

	switch {
	case etcd.Spec.Replicas < stsReplicas:
		return druidv1alpha1.ConditionFalse, druidv1alpha1.ScaleOperationReasonScalingIn, nil
	case etcd.Spec.Replicas > stsReplicas:
		return druidv1alpha1.ConditionFalse, druidv1alpha1.ScaleOperationReasonScalingOut, nil
	default:
		// Replica counts match: no active scale signal. Preserve the existing
		// condition and let recordScaleOperationComplete advance it on completion.
		status, reason := druidv1alpha1.GetScaleOperationCompleteCondition(etcd)
		return status, reason, nil
	}
}

// scaleConditionNeedsUpdate reports whether the current ScaleOperationComplete
// condition already matches the desired status and reason, avoiding a redundant
// status patch every reconcile.
func scaleConditionNeedsUpdate(etcd *druidv1alpha1.Etcd, status druidv1alpha1.ConditionStatus, reason string) bool {
	existing := druidv1alpha1.GetCondition(etcd, druidv1alpha1.ConditionTypeScaleOperationComplete)
	if existing == nil {
		// Only add the condition when there is an actual operation to record (an
		// in-flight operation is status False); a brand-new resource with no scale
		// operation does not need a True entry.
		return status == druidv1alpha1.ConditionFalse
	}
	return existing.Status != status || existing.Reason != reason
}

// patchScaleOperationCondition upserts the ScaleOperationComplete condition on
// the Etcd status sub-resource with the given status and reason.
func (r *Reconciler) patchScaleOperationCondition(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd, status druidv1alpha1.ConditionStatus, reason string) error {
	originalEtcd := etcd.DeepCopy()
	upsertScaleOperationCondition(etcd, status, reason)
	return r.client.Status().Patch(ctx, etcd, client.MergeFrom(originalEtcd))
}

// upsertScaleOperationCondition sets (or inserts) the ScaleOperationComplete
// condition on etcd.Status.Conditions. LastTransitionTime advances only when the
// status value changes; LastUpdateTime always advances.
func upsertScaleOperationCondition(etcd *druidv1alpha1.Etcd, status druidv1alpha1.ConditionStatus, reason string) {
	now := metav1.Now()
	message := scaleOperationMessage(reason)

	cond := druidv1alpha1.GetCondition(etcd, druidv1alpha1.ConditionTypeScaleOperationComplete)
	if cond == nil {
		etcd.Status.Conditions = append(etcd.Status.Conditions, druidv1alpha1.Condition{
			Type:               druidv1alpha1.ConditionTypeScaleOperationComplete,
			Status:             status,
			LastTransitionTime: now,
			LastUpdateTime:     now,
			Reason:             reason,
			Message:            message,
		})
		return
	}

	if cond.Status != status {
		cond.LastTransitionTime = now
	}
	cond.Status = status
	cond.LastUpdateTime = now
	cond.Reason = reason
	cond.Message = message
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
