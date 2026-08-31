// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package etcd

import (
	"slices"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/component"
	ctrlutils "github.com/gardener/etcd-druid/internal/controller/utils"
	"github.com/gardener/etcd-druid/internal/utils/kubernetes"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// detectAndRecordScaleOperation determines the active scale operation for the
// Etcd resource and records it in the ScaleOperationComplete status condition.
// It runs after ensureFinalizer and before component reconciliation so the CEL
// admission rules can reject conflicting opposite-direction changes while the
// operation is in progress. See docs/proposals/08-scale-in.md for the full
// design.
//
// Detection compares etcd.spec against the existing StatefulSet's spec and the
// Etcd status only -- it does not query the etcd cluster, so a transient etcd
// outage cannot block detection. The condition uses positive polarity, so an
// in-flight operation is recorded as False and the converged state as True.
// The mapping (proposal step "Detection (Step 3)"):
//
//   - spec.replicas < StatefulSet.spec.replicas          -> False, ScalingIn
//   - spec.replicas > StatefulSet.spec.replicas          -> False, ScalingOut
//   - bootstrap members removed / bootstrap config unset -> False, BootstrapMembersRemoval
//   - no scale signal                                    -> True,  NoScaleOperation
//
// The condition is set back to True (complete) on successful completion by
// recordReconcileSuccessOperation.
func (r *Reconciler) detectAndRecordScaleOperation(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) ctrlutils.ReconcileStepResult {
	status, reason := r.determineScaleOperation(ctx, etcd)

	// The detector only ever *sets* an in-flight operation (status False). Clearing
	// the condition back to True/NoScaleOperation is done exclusively at the end of
	// a successful reconcile by recordReconcileSuccessOperation, so an in-flight
	// operation is never marked complete mid-reconcile (for example before the
	// StatefulSet has actually been shrunk during a scale-in).
	if status != druidv1alpha1.ConditionFalse {
		return ctrlutils.ContinueReconcile()
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

// determineScaleOperation returns the ScaleOperationComplete condition status
// and reason for the current desired vs observed state. The condition uses
// positive polarity: True/NoScaleOperation is the converged state, and an
// in-flight operation is False with the reason naming it. On any error observing
// the StatefulSet it conservatively reports no scale operation (True), leaving
// the existing condition untouched and letting a later reconcile converge.
func (r *Reconciler) determineScaleOperation(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) (druidv1alpha1.ConditionStatus, string) {
	// BootstrapMembersRemoval takes precedence: the operator has removed joined
	// source members (or unset bootstrapWithExistingCluster) that the target had
	// already joined. Only members recorded in status are meaningful here.
	if isBootstrapMembersRemoval(etcd) {
		return druidv1alpha1.ConditionFalse, druidv1alpha1.ScaleOperationReasonBootstrapMembersRemoval
	}

	sts, err := kubernetes.GetStatefulSet(ctx, r.client, etcd)
	if err != nil {
		ctx.Logger.Error(err, "failed to get StatefulSet while detecting scale operation; preserving existing scale operation condition")
		return existingScaleOperationConditionOrDefault(etcd)
	}
	// No StatefulSet yet (fresh cluster) means there is no observed size to
	// compare against, so there is no scale operation in progress.
	if sts == nil || sts.Spec.Replicas == nil {
		return druidv1alpha1.ConditionTrue, druidv1alpha1.ScaleOperationReasonNoScaleOperation
	}

	// Transitions to/from zero (hibernation and wake-up) are handled by the
	// existing replicas -> 0 code path and are never treated as a scale-in/out.
	stsReplicas := *sts.Spec.Replicas
	if etcd.Spec.Replicas == 0 || stsReplicas == 0 {
		return druidv1alpha1.ConditionTrue, druidv1alpha1.ScaleOperationReasonNoScaleOperation
	}

	switch {
	case etcd.Spec.Replicas < stsReplicas:
		return druidv1alpha1.ConditionFalse, druidv1alpha1.ScaleOperationReasonScalingIn
	case etcd.Spec.Replicas > stsReplicas:
		return druidv1alpha1.ConditionFalse, druidv1alpha1.ScaleOperationReasonScalingOut
	default:
		return druidv1alpha1.ConditionTrue, druidv1alpha1.ScaleOperationReasonNoScaleOperation
	}
}

// isBootstrapMembersRemoval reports whether the operator has requested removal
// of source members the target already joined via bootstrapWithExistingCluster.
// It is true when the target has recorded joined members in status and one or
// more of those members is no longer present in the spec (or the spec's
// bootstrapWithExistingCluster has been unset entirely).
func isBootstrapMembersRemoval(etcd *druidv1alpha1.Etcd) bool {
	return len(druidv1alpha1.GetBootstrapMemberNamesToDecommission(etcd)) > 0
}

// existingScaleOperationConditionOrDefault returns the current
// ScaleOperationComplete condition's status and reason, or True/NoScaleOperation
// when the condition is not yet present. It is used to preserve the recorded
// condition when the StatefulSet cannot be observed, so a transient error does
// not flip an in-flight operation back to complete.
func existingScaleOperationConditionOrDefault(etcd *druidv1alpha1.Etcd) (druidv1alpha1.ConditionStatus, string) {
	idx := slices.IndexFunc(etcd.Status.Conditions, func(c druidv1alpha1.Condition) bool {
		return c.Type == druidv1alpha1.ConditionTypeScaleOperationComplete
	})
	if idx < 0 {
		return druidv1alpha1.ConditionTrue, druidv1alpha1.ScaleOperationReasonNoScaleOperation
	}
	cond := etcd.Status.Conditions[idx]
	return cond.Status, cond.Reason
}

// scaleConditionNeedsUpdate reports whether the current ScaleOperationComplete
// condition already matches the desired status and reason, avoiding a redundant
// status patch every reconcile.
func scaleConditionNeedsUpdate(etcd *druidv1alpha1.Etcd, status druidv1alpha1.ConditionStatus, reason string) bool {
	idx := slices.IndexFunc(etcd.Status.Conditions, func(c druidv1alpha1.Condition) bool {
		return c.Type == druidv1alpha1.ConditionTypeScaleOperationComplete
	})
	if idx < 0 {
		// Only add the condition when there is an actual operation to record (an
		// in-flight operation is status False); a brand-new resource with no scale
		// operation does not need a True entry.
		return status == druidv1alpha1.ConditionFalse
	}
	existing := etcd.Status.Conditions[idx]
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

	idx := slices.IndexFunc(etcd.Status.Conditions, func(c druidv1alpha1.Condition) bool {
		return c.Type == druidv1alpha1.ConditionTypeScaleOperationComplete
	})
	if idx < 0 {
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

	cond := &etcd.Status.Conditions[idx]
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
// that is no longer present in spec.etcd.bootstrapWithExistingCluster -- these
// members have been removed from the etcd cluster by the PreSync member removal
// (which requeues until no surplus remains, so by the time this runs the
// removal has completed). When all joined members have been pruned, the whole
// status field is cleared. It is a no-op when nothing needs pruning.
func (r *Reconciler) pruneBootstrapMembersStatus(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) ctrlutils.ReconcileStepResult {
	statusBootstrap := etcd.Status.BootstrapWithExistingCluster
	if statusBootstrap == nil || len(statusBootstrap.Members) == 0 {
		return ctrlutils.ContinueReconcile()
	}

	specNames := druidv1alpha1.GetBootstrapMemberNames(etcd)
	retained := make([]druidv1alpha1.BootstrapJoinedMember, 0, len(statusBootstrap.Members))
	for _, joined := range statusBootstrap.Members {
		if specNames[joined.Name] {
			retained = append(retained, joined)
		}
	}
	if len(retained) == len(statusBootstrap.Members) {
		return ctrlutils.ContinueReconcile()
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
