// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package condition

import (
	"context"
	"fmt"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	coordinationv1 "k8s.io/api/coordination/v1"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type fullSnapshotBackupReadyCheck struct {
	cl client.Client
}

type deltaSnapshotBackupReadyCheck struct {
	cl client.Client
}

const (
	// SnapshotUploadedOnSchedule is a constant that means that the etcd backup has been uploaded on schedule
	SnapshotUploadedOnSchedule string = "SnapshotUploadedOnSchedule"
	// SnapshotMissedSchedule is a constant that means that the etcd backup has missed the schedule
	SnapshotMissedSchedule string = "SnapshotMissedSchedule"
	// SnapshotProcessNotStarted is a constant that means the etcd snapshotting has not started yet
	SnapshotProcessNotStarted string = "SnapshottingProcessNotStarted"
	// Unknown is a constant that means that the etcd backup status is currently not known
	Unknown string = "Unknown"
	// NotChecked is a constant that means that the etcd backup status has not been updated or rechecked
	NotChecked string = "NotChecked"
)

func (f *fullSnapshotBackupReadyCheck) Check(ctx context.Context, etcd druidv1alpha1.Etcd) Result {
	//Default case
	result := &result{
		conType: druidv1alpha1.ConditionTypeFullSnapshotBackupReady,
		status:  druidv1alpha1.ConditionUnknown,
		reason:  Unknown,
		message: "Cannot determine full snapshot upload status",
	}

	// Special case of etcd not being configured to take snapshots
	// Do not add the FullSnapshotBackupReady condition if backup is not configured
	if !isBackupConfigured(&etcd) {
		return nil
	}

	//Fetch snapshot leases
	var (
		fullSnapErr   error
		fullSnapLease = &coordinationv1.Lease{}
	)
	fullSnapErr = f.cl.Get(ctx, types.NamespacedName{Name: getFullSnapLeaseName(&etcd), Namespace: etcd.ObjectMeta.Namespace}, fullSnapLease)
	fullLeaseRenewTime := fullSnapLease.Spec.RenewTime

	//Set status to Unknown if errors in fetching snapshot leases or lease never renewed
	if fullSnapErr != nil || fullLeaseRenewTime == nil {
		return result
	}

	if time.Since(fullLeaseRenewTime.Time) < 24*time.Hour {
		result.status = druidv1alpha1.ConditionTrue
		result.reason = SnapshotUploadedOnSchedule
		result.message = fmt.Sprintf("Full snapshot uploaded successfully %v ago", time.Since(fullLeaseRenewTime.Time))
		return result
	} else {
		result.status = druidv1alpha1.ConditionFalse
		result.reason = SnapshotMissedSchedule
		result.message = fmt.Sprintf("Full snapshot missed schedule, last full snapshot was taken %v ago", time.Since(fullLeaseRenewTime.Time))
		return result
	}
}

func (d *deltaSnapshotBackupReadyCheck) Check(ctx context.Context, etcd druidv1alpha1.Etcd) Result {
	//Default case
	result := &result{
		conType: druidv1alpha1.ConditionTypeDeltaSnapshotBackupReady,
		status:  druidv1alpha1.ConditionUnknown,
		reason:  Unknown,
		message: "Cannot determine delta snapshot upload status",
	}

	// Special case of etcd not being configured to take snapshots
	// Do not add the DeltaSnapshotBackupReady condition if backup is not configured
	if !isBackupConfigured(&etcd) {
		return nil
	}
	var (
		incrSnapErr    error
		deltaSnapLease = &coordinationv1.Lease{}
	)
	incrSnapErr = d.cl.Get(ctx, types.NamespacedName{Name: getDeltaSnapLeaseName(&etcd), Namespace: etcd.ObjectMeta.Namespace}, deltaSnapLease)
	deltaLeaseRenewTime := deltaSnapLease.Spec.RenewTime
	deltaSnapshotPeriod := etcd.Spec.Backup.DeltaSnapshotPeriod.Duration
	deltaLeaseCreationTime := deltaSnapLease.ObjectMeta.CreationTimestamp

	//Set status to Unknown if error in fetching delta snapshot lease or lease never renewed
	if incrSnapErr != nil {
		return result
	}

	if deltaLeaseRenewTime == nil {
		if time.Since(deltaLeaseCreationTime.Time) < deltaSnapshotPeriod {
			result.status = druidv1alpha1.ConditionTrue
			result.reason = SnapshotProcessNotStarted
			result.message = fmt.Sprintf("Delta snapshotting has not been triggered yet")
			return result
		} else if time.Since(deltaLeaseCreationTime.Time) < 3*deltaSnapshotPeriod {
			result.status = druidv1alpha1.ConditionUnknown
			result.reason = Unknown
			result.message = fmt.Sprintf("Periodic delta snapshotting has not started yet")
			return result
		} else {
			result.status = druidv1alpha1.ConditionFalse
			result.reason = SnapshotMissedSchedule
			result.message = fmt.Sprintf("The Delta snapshot lease has not been renewed even once since its inception")
		}
	} else {
		if time.Since(deltaLeaseCreationTime.Time) < deltaSnapshotPeriod {
			result.status = druidv1alpha1.ConditionTrue
			result.reason = SnapshotUploadedOnSchedule
			result.message = fmt.Sprintf("Delta snapshot uploaded successfully %v ago", time.Since(deltaLeaseRenewTime.Time))
			return result
		} else if time.Since(deltaLeaseCreationTime.Time) < 3*deltaSnapshotPeriod {
			result.status = druidv1alpha1.ConditionUnknown
			result.reason = Unknown
			result.message = fmt.Sprintf("One or more delta snapshots seems to have missed their scheduled times, likely due to a transient issue with updating the lease")
			return result
		}
	}

	//Cases where delta snapshot lease is not updated for a long time
	//If delta snapshot lease is present and not updated, it is safe to assume that backup is not healthy

	if etcd.Status.Conditions != nil {
		var prevDeltaSnapshotBackupReadyStatus druidv1alpha1.Condition
		for _, prevDeltaSnapshotBackupReadyStatus = range etcd.Status.Conditions {
			if prevDeltaSnapshotBackupReadyStatus.Type == druidv1alpha1.ConditionTypeDeltaSnapshotBackupReady {
				break
			}
		}

		// Transition to "False" state only if present state is "Unknown" or "False"
		if deltaLeaseRenewTime != nil && (prevDeltaSnapshotBackupReadyStatus.Status == druidv1alpha1.ConditionUnknown || prevDeltaSnapshotBackupReadyStatus.Status == druidv1alpha1.ConditionFalse) {
			if time.Since(deltaLeaseRenewTime.Time) > 3*deltaSnapshotPeriod {
				result.status = druidv1alpha1.ConditionFalse
				result.reason = SnapshotMissedSchedule
				result.message = fmt.Sprintf("Delta snapshot missed schedule, last delta snapshot was taken %v ago", time.Since(deltaLeaseRenewTime.Time))
				return result
			}
		}
	}
	//Transition to "Unknown" state is we cannot prove a "True" state
	return result
}

func getDeltaSnapLeaseName(etcd *druidv1alpha1.Etcd) string {
	return fmt.Sprintf("%s-delta-snap", string(etcd.ObjectMeta.Name))
}

func getFullSnapLeaseName(etcd *druidv1alpha1.Etcd) string {
	return fmt.Sprintf("%s-full-snap", string(etcd.ObjectMeta.Name))
}

// FullSnapshotBackupReadyCheck returns a check for the "FullSnapshotBackupReady" condition.
func FullSnapshotBackupReadyCheck(cl client.Client) Checker {
	return &fullSnapshotBackupReadyCheck{
		cl: cl,
	}
}

// DeltaSnapshotBackupReadyCheck returns a check for the "DeltaSnapshotBackupReady" condition.
func DeltaSnapshotBackupReadyCheck(cl client.Client) Checker {
	return &deltaSnapshotBackupReadyCheck{
		cl: cl,
	}
}

func isBackupConfigured(etcd *druidv1alpha1.Etcd) bool {
	if etcd.Spec.Backup.Store == nil || etcd.Spec.Backup.Store.Provider == nil || len(*etcd.Spec.Backup.Store.Provider) == 0 {
		return false
	}
	return true
}
