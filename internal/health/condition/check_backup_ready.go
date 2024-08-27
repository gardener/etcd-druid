// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package condition

import (
	"context"
	"fmt"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/utils"
	coordinationv1 "k8s.io/api/coordination/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type backupReadyCheck struct {
	cl      client.Client
	results []Result
}

type fullSnapshotBackupReadyCheck struct {
	cl client.Client
}

type deltaSnapshotBackupReadyCheck struct {
	cl client.Client
}

const (
	// BackupSucceeded is a constant that means that etcd backup has been successfully taken
	BackupSucceeded string = "BackupSucceeded"
	// BackupFailed is a constant that means that etcd backup has failed
	BackupFailed string = "BackupFailed"
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

func (b *backupReadyCheck) Check(_ context.Context, _ druidv1alpha1.Etcd) Result {
	result := &result{
		conType: druidv1alpha1.ConditionTypeBackupReady,
		status:  druidv1alpha1.ConditionUnknown,
		reason:  Unknown,
		message: "Cannot determine backup upload status",
	}

	var FullSnapshotBackupReadyCheckResult, DeltaSnapshotBackupReadyCheckResult Result = nil, nil
	for _, result := range b.results {
		if result == nil {
			continue
		}
		if result.ConditionType() == druidv1alpha1.ConditionTypeFullSnapshotBackupReady {
			FullSnapshotBackupReadyCheckResult = result
			continue
		}
		if result.ConditionType() == druidv1alpha1.ConditionTypeDeltaSnapshotBackupReady {
			DeltaSnapshotBackupReadyCheckResult = result
			continue
		}
	}
	if FullSnapshotBackupReadyCheckResult == nil || DeltaSnapshotBackupReadyCheckResult == nil {
		return nil
	}
	var (
		fullSnapshotBackupMissedSchedule  = FullSnapshotBackupReadyCheckResult.Status() == druidv1alpha1.ConditionFalse
		fullSnapshotBackupSucceeded       = FullSnapshotBackupReadyCheckResult.Status() == druidv1alpha1.ConditionTrue
		deltaSnapshotBackupMissedSchedule = DeltaSnapshotBackupReadyCheckResult.Status() == druidv1alpha1.ConditionFalse
	)
	// False case
	if fullSnapshotBackupMissedSchedule || deltaSnapshotBackupMissedSchedule {
		result.status = druidv1alpha1.ConditionFalse
		result.reason = BackupFailed
		result.message = backupFailureMessage(fullSnapshotBackupMissedSchedule, deltaSnapshotBackupMissedSchedule)
		return result
	}
	// True case
	if fullSnapshotBackupSucceeded {
		result.status = druidv1alpha1.ConditionTrue
		result.reason = BackupSucceeded
		result.message = "Snapshot backup succeeded"
		return result
	}
	// Unknown case
	return result
}

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
		fullSnapErr          error
		fullSnapLease        = &coordinationv1.Lease{}
		fullSnapshotInterval = 24 * time.Hour
		err                  error
	)
	fullSnapErr = f.cl.Get(ctx, types.NamespacedName{Name: getFullSnapLeaseName(&etcd), Namespace: etcd.ObjectMeta.Namespace}, fullSnapLease)
	fullLeaseRenewTime := fullSnapLease.Spec.RenewTime
	fullLeaseCreationTime := fullSnapLease.ObjectMeta.CreationTimestamp

	if etcd.Spec.Backup.FullSnapshotSchedule != nil {
		if fullSnapshotInterval, err = utils.ComputeScheduleInterval(*etcd.Spec.Backup.FullSnapshotSchedule); err != nil {
			return result
		}
	}
	//Set status to Unknown if errors in fetching full snapshot lease
	if fullSnapErr != nil {
		return result
	}
	if fullLeaseRenewTime == nil {
		if time.Since(fullLeaseCreationTime.Time) < fullSnapshotInterval {
			return result
		} else {
			result.status = druidv1alpha1.ConditionFalse
			result.reason = SnapshotMissedSchedule
			result.message = "Full snapshot missed schedule. Backup is unhealthy"
			return result
		}
	} else {
		if time.Since(fullLeaseRenewTime.Time) < fullSnapshotInterval {
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

	//Set status to Unknown if error in fetching delta snapshot lease
	if incrSnapErr != nil {
		return result
	}

	if deltaLeaseRenewTime == nil {
		if time.Since(deltaLeaseCreationTime.Time) < deltaSnapshotPeriod {
			result.status = druidv1alpha1.ConditionTrue
			result.reason = SnapshotProcessNotStarted
			result.message = "Delta snapshotting has not been triggered yet"
			return result
		} else if time.Since(deltaLeaseCreationTime.Time) < 3*deltaSnapshotPeriod {
			result.status = druidv1alpha1.ConditionUnknown
			result.reason = Unknown
			result.message = "Periodic delta snapshotting has not started yet"
			return result
		}
	} else {
		if time.Since(deltaLeaseRenewTime.Time) < deltaSnapshotPeriod {
			result.status = druidv1alpha1.ConditionTrue
			result.reason = SnapshotUploadedOnSchedule
			result.message = fmt.Sprintf("Delta snapshot uploaded successfully %v ago", time.Since(deltaLeaseRenewTime.Time))
			return result
		} else if time.Since(deltaLeaseRenewTime.Time) < 3*deltaSnapshotPeriod {
			result.status = druidv1alpha1.ConditionUnknown
			result.reason = Unknown
			result.message = "One or more delta snapshots seems to have missed their scheduled times, likely due to a transient issue with updating the lease"
			return result
		}
	}

	//Cases where delta snapshot lease is not renewed at all or not updated for a long time
	//If delta snapshot lease is present and not updated, it is safe to assume that backup is not healthy
	if etcd.Status.Conditions != nil {
		var prevDeltaSnapshotBackupReadyStatus druidv1alpha1.Condition
		for _, prevDeltaSnapshotBackupReadyStatus = range etcd.Status.Conditions {
			if prevDeltaSnapshotBackupReadyStatus.Type == druidv1alpha1.ConditionTypeDeltaSnapshotBackupReady {
				break
			}
		}
		// Transition to "False" state only if present state is "Unknown" or "False"
		if prevDeltaSnapshotBackupReadyStatus.Status == druidv1alpha1.ConditionUnknown || prevDeltaSnapshotBackupReadyStatus.Status == druidv1alpha1.ConditionFalse {
			if deltaLeaseRenewTime == nil || time.Since(deltaLeaseRenewTime.Time) > 3*deltaSnapshotPeriod {
				result.status = druidv1alpha1.ConditionFalse
				result.reason = SnapshotMissedSchedule
				result.message = "Stale snapshot lease. Not updated for a long time. Backup is unhealthy"
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

// BackupReadyCheck returns a check for the "BackupReady" condition.
func BackupReadyCheck(cl client.Client, results []Result) Checker {
	return &backupReadyCheck{
		cl:      cl,
		results: results,
	}
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

func backupFailureMessage(fullSnapshotBackupMissedSchedule, deltaSnapshotBackupMissedSchedule bool) string {
	if fullSnapshotBackupMissedSchedule && deltaSnapshotBackupMissedSchedule {
		return "Both Full & Delta snapshot backups missed their schedule"
	}
	if fullSnapshotBackupMissedSchedule {
		return "Full snapshot backup missed schedule"
	}
	return "Delta snapshot backup missed schedule"
}

func isBackupConfigured(etcd *druidv1alpha1.Etcd) bool {
	if etcd.Spec.Backup.Store == nil || etcd.Spec.Backup.Store.Provider == nil || len(*etcd.Spec.Backup.Store.Provider) == 0 {
		return false
	}
	return true
}
