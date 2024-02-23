// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package condition

import (
	"context"
	"fmt"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/pkg/utils"

	"github.com/go-logr/logr"
	coordinationv1 "k8s.io/api/coordination/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type backupReadyCheck struct {
	cl client.Client
}

const (
	// BackupSucceeded is a constant that means that etcd backup has been successfully taken
	BackupSucceeded string = "BackupSucceeded"
	// BackupFailed is a constant that means that etcd backup has failed
	BackupFailed string = "BackupFailed"
	// Unknown is a constant that means that the etcd backup status is currently not known
	Unknown string = "Unknown"
	// NotChecked is a constant that means that the etcd backup status has not been updated or rechecked
	NotChecked string = "NotChecked"
)

func (a *backupReadyCheck) Check(ctx context.Context, logger logr.Logger, etcd druidv1alpha1.Etcd) Result {
	//Default case
	result := &result{
		conType: druidv1alpha1.ConditionTypeBackupReady,
		status:  druidv1alpha1.ConditionUnknown,
		reason:  Unknown,
		message: "Cannot determine etcd backup status",
	}

	// Special case of etcd not being configured to take snapshots
	// Do not add the BackupReady condition if backup is not configured
	if etcd.Spec.Backup.Store == nil || etcd.Spec.Backup.Store.Provider == nil || len(*etcd.Spec.Backup.Store.Provider) == 0 {
		return nil
	}

	// Fetch snapshot leases
	var (
		err, fullSnapErr, deltaSnapErr error
		fullSnapshotInterval           = 1 * time.Hour
		fullSnapLease                  = &coordinationv1.Lease{}
		deltaSnapLease                 = &coordinationv1.Lease{}
	)
	fullSnapErr = a.cl.Get(ctx, types.NamespacedName{Name: getFullSnapLeaseName(&etcd), Namespace: etcd.Namespace}, fullSnapLease)
	deltaSnapErr = a.cl.Get(ctx, types.NamespacedName{Name: getDeltaSnapLeaseName(&etcd), Namespace: etcd.Namespace}, deltaSnapLease)

	// Set status to Unknown if errors in fetching snapshot leases or lease never renewed
	if fullSnapErr != nil || deltaSnapErr != nil || (fullSnapLease.Spec.RenewTime == nil && deltaSnapLease.Spec.RenewTime == nil) {
		return result
	}

	deltaLeaseRenewTime := deltaSnapLease.Spec.RenewTime
	fullLeaseRenewTime := fullSnapLease.Spec.RenewTime
	fullLeaseCreateTime := &fullSnapLease.ObjectMeta.CreationTimestamp

	// TODO: make etcd.Spec.Backup.FullSnapshotSchedule non-optional, since it is mandatory to
	// set the full snapshot schedule, or introduce defaulting webhook to add default value for this field
	if etcd.Spec.Backup.FullSnapshotSchedule != nil {
		if fullSnapshotInterval, err = utils.ComputeScheduleInterval(*etcd.Spec.Backup.FullSnapshotSchedule); err != nil {
			logger.Error(err, "unable to compute full snapshot duration from full snapshot schedule", "fullSnapshotSchedule", *etcd.Spec.Backup.FullSnapshotSchedule)
			return result
		}
	}

	if fullLeaseRenewTime == nil && deltaLeaseRenewTime != nil {
		// Most probable during reconcile of existing clusters if fresh leases are created
		// Treat backup as succeeded if delta snap lease renewal happens in the required time window
		// and full snap lease is not older than full snapshot duration.
		if time.Since(deltaLeaseRenewTime.Time) < 2*etcd.Spec.Backup.DeltaSnapshotPeriod.Duration && time.Since(fullLeaseCreateTime.Time) < fullSnapshotInterval {
			result.reason = BackupSucceeded
			result.message = "Delta snapshot backup succeeded"
			result.status = druidv1alpha1.ConditionTrue
			return result
		}
	} else if deltaLeaseRenewTime == nil && fullLeaseRenewTime != nil {
		// Most probable during a startup scenario for new clusters
		// Special case. Return Unknown condition for some time to allow delta backups to start up
		if time.Since(fullLeaseRenewTime.Time) > 5*etcd.Spec.Backup.DeltaSnapshotPeriod.Duration {
			result.message = "Periodic delta snapshots not started yet"
			return result
		}
	} else if deltaLeaseRenewTime != nil && fullLeaseRenewTime != nil {
		// Both snap leases are maintained. Both are expected to be renewed periodically
		if time.Since(deltaLeaseRenewTime.Time) < 2*etcd.Spec.Backup.DeltaSnapshotPeriod.Duration && time.Since(fullLeaseRenewTime.Time) < fullSnapshotInterval {
			result.reason = BackupSucceeded
			result.message = "Snapshot backup succeeded"
			result.status = druidv1alpha1.ConditionTrue
			return result
		}
	}

	// Cases where snapshot leases are not updated for a long time
	// If snapshot leases are present and leases aren't updated, it is safe to assume that backup is not healthy

	if etcd.Status.Conditions != nil {
		var prevBackupReadyStatus druidv1alpha1.Condition
		for _, prevBackupReadyStatus = range etcd.Status.Conditions {
			if prevBackupReadyStatus.Type == druidv1alpha1.ConditionTypeBackupReady {
				break
			}
		}

		// Transition to "False" state only if present state is "Unknown" or "False"
		if deltaLeaseRenewTime != nil && (prevBackupReadyStatus.Status == druidv1alpha1.ConditionUnknown || prevBackupReadyStatus.Status == druidv1alpha1.ConditionFalse) {
			if time.Since(deltaLeaseRenewTime.Time) > 3*etcd.Spec.Backup.DeltaSnapshotPeriod.Duration {
				result.status = druidv1alpha1.ConditionFalse
				result.reason = BackupFailed
				result.message = "Stale snapshot leases. Not renewed in a long time"
				return result
			}
		}
	}

	// Transition to "Unknown" state is we cannot prove a "True" state
	return result
}

func getDeltaSnapLeaseName(etcd *druidv1alpha1.Etcd) string {
	return fmt.Sprintf("%s-delta-snap", etcd.Name)
}

func getFullSnapLeaseName(etcd *druidv1alpha1.Etcd) string {
	return fmt.Sprintf("%s-full-snap", etcd.Name)
}

// BackupReadyCheck returns a check for the "BackupReady" condition.
func BackupReadyCheck(cl client.Client) Checker {
	return &backupReadyCheck{
		cl: cl,
	}
}
