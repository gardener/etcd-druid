// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package compaction

import (
	"context"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// updateCompactionJobEtcdStatusCondition updates the Etcd status condition LastSnapshotCompactionSucceeded with the latest job/fullSnapshot status.
func (r *Reconciler) updateCompactionJobEtcdStatusCondition(ctx context.Context, latestEtcd *druidv1alpha1.Etcd, latestCondition druidv1alpha1.Condition) error {
	oldEtcdStatus := latestEtcd.Status.DeepCopy()
	var isSnapshotCompactionConditionPresent bool
	for i, condition := range oldEtcdStatus.Conditions {
		if condition.Type == druidv1alpha1.ConditionTypeLastSnapshotCompactionSucceeded {
			latestCondition.LastTransitionTime = condition.LastTransitionTime
			latestCondition.LastUpdateTime = metav1.NewTime(time.Now().UTC())
			// Update the LastTransitionTime if the status or reason has changed
			if condition.Status != latestCondition.Status || condition.Reason != latestCondition.Reason {
				latestCondition.LastTransitionTime = metav1.NewTime(time.Now().UTC())
			}
			// Update the condition in the old status
			oldEtcdStatus.Conditions[i] = latestCondition
			isSnapshotCompactionConditionPresent = true
			break
		}
	}
	if !isSnapshotCompactionConditionPresent {
		latestCondition.LastTransitionTime = metav1.NewTime(time.Now().UTC())
		latestCondition.LastUpdateTime = metav1.NewTime(time.Now().UTC())
		oldEtcdStatus.Conditions = append(oldEtcdStatus.Conditions, latestCondition)
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

func computeSnapshotCompactionJobReason(jobCompletionState int, jobFailureReason string) string {
	if jobCompletionState == jobSucceeded {
		return druidv1alpha1.PodSuccessReasonNone
	}
	if jobFailureReason != "" {
		return jobFailureReason
	}
	// The code should not reach here, but if it does, we return an unknown reason
	return druidv1alpha1.PodFailureReasonUnknown
}

func isLastCompactionConditionDeadlineExceeded(etcd *druidv1alpha1.Etcd) bool {
	etcdConditions := etcd.Status.Conditions
	for _, condition := range etcdConditions {
		if condition.Type == druidv1alpha1.ConditionTypeLastSnapshotCompactionSucceeded &&
			condition.Status == druidv1alpha1.ConditionFalse &&
			condition.Reason == druidv1alpha1.JobFailureReasonDeadlineExceeded {
			return true
		}
	}
	return false
}
