package compaction

import (
	"context"
	"fmt"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	druidmetrics "github.com/gardener/etcd-druid/internal/metrics"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
)

func (r *Reconciler) triggerFullSnapshotAndUpdateStatus(ctx context.Context, logger logr.Logger, etcd *druidv1alpha1.Etcd, accumulatedEtcdRevisions, triggerFullSnapshotThreshold int64) (ctrl.Result, error) {
	latestSnapshotCompactionCondition := druidv1alpha1.Condition{
		Type: druidv1alpha1.ConditionTypeSnapshotCompactionSucceeded,
	}
	fullSnapErr := r.triggerFullSnapshot(ctx, logger, etcd, accumulatedEtcdRevisions, triggerFullSnapshotThreshold)
	if fullSnapErr != nil {
		latestSnapshotCompactionCondition.Status = druidv1alpha1.ConditionFalse
		latestSnapshotCompactionCondition.Reason = druidv1alpha1.ConditionReasonFullSnapshotError
		latestSnapshotCompactionCondition.Message = fmt.Sprintf("Error while triggering full snapshot for etcd %s/%s: %v", etcd.Namespace, etcd.Name, fullSnapErr)
	} else {
		latestSnapshotCompactionCondition.Status = druidv1alpha1.ConditionTrue
		latestSnapshotCompactionCondition.Reason = druidv1alpha1.ConditionReasonFullSnapshotTakenSuccessfully
		latestSnapshotCompactionCondition.Message = fmt.Sprintf("Full snapshot taken successfully for etcd %s/%s", etcd.Namespace, etcd.Name)
	}
	etcdStatusUPDErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		latestEtcd := &druidv1alpha1.Etcd{}
		if err := r.Get(ctx, types.NamespacedName{Namespace: etcd.Namespace, Name: etcd.Name}, latestEtcd); err != nil {
			return err
		}
		return r.updateCompactionJobEtcdStatusCondition(ctx, latestEtcd, latestSnapshotCompactionCondition)
	})

	if fullSnapErr != nil || etcdStatusUPDErr != nil {
		var requeueErrReason error
		if fullSnapErr != nil && etcdStatusUPDErr != nil {
			requeueErrReason = fmt.Errorf("error while triggering full snapshot and updating compaction-fullSnapshot etcd status condition: %w", fullSnapErr)
		} else if fullSnapErr != nil {
			requeueErrReason = fmt.Errorf("error while triggering compaction-fullSnapshot: %w", fullSnapErr)
		} else {
			requeueErrReason = fmt.Errorf("error while updating compaction-fullSnapshot etcd status condition: %w", etcdStatusUPDErr)
		}
		logger.Error(requeueErrReason, "Error in triggerFullSnapshotAndUpdateStatus")
		return ctrl.Result{}, requeueErrReason
	}
	return ctrl.Result{}, nil
}

func (r *Reconciler) updateCompactionJobEtcdStatusCondition(ctx context.Context, latestEtcd *druidv1alpha1.Etcd, latestSnapshotCompactionCondition druidv1alpha1.Condition) error {
	oldEtcdStatus := latestEtcd.Status.DeepCopy()
	var isSnapshotCompactionConditionPresent bool
	for i, condition := range oldEtcdStatus.Conditions {
		if condition.Type == druidv1alpha1.ConditionTypeSnapshotCompactionSucceeded {
			latestSnapshotCompactionCondition.LastTransitionTime = condition.LastTransitionTime
			latestSnapshotCompactionCondition.LastUpdateTime = metav1.NewTime(time.Now().UTC())
			// Update the LastTransitionTime if the status or reason has changed
			if condition.Status != latestSnapshotCompactionCondition.Status || condition.Reason != latestSnapshotCompactionCondition.Reason {
				latestSnapshotCompactionCondition.LastTransitionTime = metav1.NewTime(time.Now().UTC())
			}
			// Update the condition in the old status
			oldEtcdStatus.Conditions[i] = latestSnapshotCompactionCondition
			isSnapshotCompactionConditionPresent = true
			break
		}
	}
	if !isSnapshotCompactionConditionPresent {
		latestSnapshotCompactionCondition.LastTransitionTime = metav1.NewTime(time.Now().UTC())
		latestSnapshotCompactionCondition.LastUpdateTime = metav1.NewTime(time.Now().UTC())
		oldEtcdStatus.Conditions = append(oldEtcdStatus.Conditions, latestSnapshotCompactionCondition)
	}
	latestEtcd.Status = *oldEtcdStatus
	return r.Status().Update(ctx, latestEtcd)
}

func computeSnapshotCompactionJobStatus(jobCompletionState int) druidv1alpha1.ConditionStatus {
	if jobCompletionState == jobSucceeded {
		return druidv1alpha1.ConditionTrue
	}
	return druidv1alpha1.ConditionFalse
}

func computeJobFailureReason(jobCompletionState int, jobFailureReason string) string {
	if jobCompletionState == jobSucceeded {
		return druidmetrics.ValueFailureReasonNone
	}
	if jobFailureReason != "" {
		return jobFailureReason
	}
	// The code should not reach here, but if it does, we return an unknown reason
	return druidmetrics.ValueFailureReasonUnknown
}
