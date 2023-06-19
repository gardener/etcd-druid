// Copyright (c) 2022 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package condition

import (
	"context"
	"fmt"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/pkg/utils"
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
	NotChecked string = "ConditionNotChecked"
)

func (a *backupReadyCheck) Check(ctx context.Context, etcd druidv1alpha1.Etcd) Result {
	// Default case
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
		err            error
		fullSnapLease  = &coordinationv1.Lease{}
		deltaSnapLease = &coordinationv1.Lease{}
	)

	err = a.cl.Get(ctx, types.NamespacedName{Name: getFullSnapLeaseName(&etcd), Namespace: etcd.ObjectMeta.Namespace}, fullSnapLease)
	if err != nil {
		result.message = fmt.Sprintf("Unable to fetch full snap lease. %s", err.Error())
		return result
	}

	err = a.cl.Get(ctx, types.NamespacedName{Name: getDeltaSnapLeaseName(&etcd), Namespace: etcd.ObjectMeta.Namespace}, deltaSnapLease)
	if err != nil {
		result.message = fmt.Sprintf("Unable to fetch delta snap lease. %s", err.Error())
		return result
	}

	deltaLeaseRenewTime := deltaSnapLease.Spec.RenewTime
	fullLeaseRenewTime := fullSnapLease.Spec.RenewTime
	fullLeaseCreateTime := &fullSnapLease.ObjectMeta.CreationTimestamp

	if fullLeaseRenewTime == nil && deltaLeaseRenewTime == nil {
		// Both snapshot leases are not yet renewed
		result.message = "Snapshotter has not started yet"
		return result

	} else if fullLeaseRenewTime == nil && deltaLeaseRenewTime != nil {
		// Most probable during reconcile of existing clusters if fresh leases are created

		fullSnapshotDuration, err := utils.ComputeScheduleDuration(*etcd.Spec.Backup.FullSnapshotSchedule)
		if err != nil {
			return result
		}

		// Treat backup as succeeded if delta snap lease renewal happens in the required time window
		// and full snap lease is not older than the computed full snapshot duration.
		if time.Since(deltaLeaseRenewTime.Time) < 2*etcd.Spec.Backup.DeltaSnapshotPeriod.Duration {
			if time.Since(fullLeaseCreateTime.Time) < fullSnapshotDuration {
				result.reason = BackupSucceeded
				result.message = "Delta snapshot backup succeeded"
				result.status = druidv1alpha1.ConditionTrue
				return result
			} else {
				result.status = druidv1alpha1.ConditionFalse
				result.reason = BackupFailed
				result.message = "Full snapshot backup failed. Full snapshot lease created long ago, but not renewed"
				return result
			}
		}

	} else if fullLeaseRenewTime != nil && deltaLeaseRenewTime == nil {
		// Most probable during a startup scenario for new clusters
		// Special case: return Unknown condition for some time to allow delta backups to start up
		// Even though full snapshot may have succeeded by the required time, we must still wait
		// for delta snapshotting to begin to consider the backups as healthy, to maintain the given RPO
		if time.Since(fullLeaseRenewTime.Time) < 5*etcd.Spec.Backup.DeltaSnapshotPeriod.Duration {
			result.message = "Waiting for periodic delta snapshotting to begin"
			return result
		} else {
			result.status = druidv1alpha1.ConditionFalse
			result.reason = BackupFailed
			result.message = "Delta snapshot backup failed. Delta snapshot lease created long ago, but not renewed"
			return result
		}

	} else {
		// Both snap leases are renewed, ie, maintained. Both are expected to be renewed periodically
		fullSnapshotDuration, err := utils.ComputeScheduleDuration(*etcd.Spec.Backup.FullSnapshotSchedule)
		if err != nil {
			return result
		}

		// Mark backup as succeeded only if at least one of the last two delta snapshots has been taken successfully
		// and if the last full snapshot has been taken according to the given full snapshot schedule
		if time.Since(deltaLeaseRenewTime.Time) < 2*etcd.Spec.Backup.DeltaSnapshotPeriod.Duration && time.Since(fullLeaseRenewTime.Time) < fullSnapshotDuration {
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
	return fmt.Sprintf("%s-delta-snap", etcd.ObjectMeta.Name)
}

func getFullSnapLeaseName(etcd *druidv1alpha1.Etcd) string {
	return fmt.Sprintf("%s-full-snap", etcd.ObjectMeta.Name)
}

// BackupReadyCheck returns a check for the "BackupReady" condition.
func BackupReadyCheck(cl client.Client) Checker {
	return &backupReadyCheck{
		cl: cl,
	}
}
