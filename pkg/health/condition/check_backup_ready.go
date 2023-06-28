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
	// Special case of etcd not being configured to take snapshots
	// Do not add the BackupReady condition if backup is not configured
	if etcd.Spec.Backup.Store == nil || etcd.Spec.Backup.Store.Provider == nil || len(*etcd.Spec.Backup.Store.Provider) == 0 {
		return nil
	}

	// Fetch snapshot leases
	fullSnapshotLease, err := a.fetchLease(ctx, etcd.GetFullSnapshotLeaseName(), etcd.Namespace)
	if err != nil {
		return createBackupConditionResult(
			druidv1alpha1.ConditionUnknown, Unknown,
			fmt.Sprintf("Unable to fetch full snap lease. %s", err.Error()),
		)
	}

	deltaSnapSnapshotLease, err := a.fetchLease(ctx, etcd.GetDeltaSnapshotLeaseName(), etcd.Namespace)
	if err != nil {
		return createBackupConditionResult(
			druidv1alpha1.ConditionUnknown, Unknown,
			fmt.Sprintf("Unable to fetch delta snap lease. %s", err.Error()),
		)
	}

	deltaSnapshotLeaseRenewTime := deltaSnapSnapshotLease.Spec.RenewTime
	fullSnapshotLeaseRenewTime := fullSnapshotLease.Spec.RenewTime
	fullSnapshotLeaseCreationTime := &fullSnapshotLease.ObjectMeta.CreationTimestamp
	fullSnapshotDuration, err := utils.ComputeScheduleDuration(*etcd.Spec.Backup.FullSnapshotSchedule)
	if err != nil {
		return createBackupConditionResult(
			druidv1alpha1.ConditionUnknown, Unknown,
			fmt.Sprintf("Unable to compute full snapshot duration from schedule. %v", err.Error()),
		)
	}

	// Both snapshot leases are not yet renewed
	if fullSnapshotLeaseRenewTime == nil && deltaSnapshotLeaseRenewTime == nil {
		return createBackupConditionResult(
			druidv1alpha1.ConditionUnknown, Unknown,
			"Snapshotter has not started yet",
		)
	}

	// Most probable during reconcile of existing clusters if fresh leases are created
	if fullSnapshotLeaseRenewTime == nil && deltaSnapshotLeaseRenewTime != nil {
		return handleOnlyDeltaSnapshotLeaseRenewedCase(
			fullSnapshotLeaseCreationTime.Time, fullSnapshotDuration,
			deltaSnapshotLeaseRenewTime.Time, 2*etcd.Spec.Backup.DeltaSnapshotPeriod.Duration,
		)
	}

	// Most probable during a startup of a new cluster and only full snapshot has been taken
	if fullSnapshotLeaseRenewTime != nil && deltaSnapshotLeaseRenewTime == nil {
		return handleSnapshotterStartupCase(
			fullSnapshotLease.Spec.RenewTime.Time,
			5*etcd.Spec.Backup.DeltaSnapshotPeriod.Duration,
		)
	}

	// Both snap leases are renewed, ie, maintained. Both are expected to be renewed periodically
	return handleRenewedSnapshotLeases(
		fullSnapshotLease.Spec.RenewTime.Time, fullSnapshotDuration,
		deltaSnapSnapshotLease.Spec.RenewTime.Time, 2*etcd.Spec.Backup.DeltaSnapshotPeriod.Duration,
	)
}

func (a *backupReadyCheck) fetchLease(ctx context.Context, name string, namespace string) (*coordinationv1.Lease, error) {
	lease := &coordinationv1.Lease{}
	err := a.cl.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, lease)
	return lease, err
}

func createBackupConditionResult(status druidv1alpha1.ConditionStatus, reason string, message string) *result {
	return &result{
		conType: druidv1alpha1.ConditionTypeBackupReady,
		status:  status,
		reason:  reason,
		message: message,
	}
}

func isLeaseStale(renewTime time.Time, renewalGracePeriod time.Duration) bool {
	return time.Since(renewTime) > renewalGracePeriod
}

func wasLeaseCreatedRecently(creationTime time.Time, creationGracePeriod time.Duration) bool {
	return time.Since(creationTime) < creationGracePeriod
}

// handleOnlyDeltaSnapshotLeaseRenewedCase handles cases where only delta snapshot lease is renewed,
// while the full snapshot lease is not renewed, possibly since it might have been recreated recently.
// Treats backup as succeeded if delta snapshot lease is renewed within the required time window
// and full snapshot lease object is not older than the computed full snapshot duration.
func handleOnlyDeltaSnapshotLeaseRenewedCase(fullSnapshotLeaseCreationTime time.Time, fullSnapshotLeaseCreationGracePeriod time.Duration, deltaSnapshotLeaseRenewTime time.Time, deltaSnapshotLeaseRenewalGracePeriod time.Duration) *result {
	wasFullSnapshotLeaseCreatedRecently := wasLeaseCreatedRecently(fullSnapshotLeaseCreationTime, fullSnapshotLeaseCreationGracePeriod)
	isDeltaSnapshotLeaseStale := isLeaseStale(deltaSnapshotLeaseRenewTime, deltaSnapshotLeaseRenewalGracePeriod)

	if isDeltaSnapshotLeaseStale {
		if wasFullSnapshotLeaseCreatedRecently {
			return createBackupConditionResult(
				druidv1alpha1.ConditionFalse, BackupFailed,
				"Delta snapshot backup failed. Delta snapshot lease not renewed in a long time",
			)
		} else {
			return createBackupConditionResult(
				druidv1alpha1.ConditionFalse, BackupFailed,
				"Stale snapshot leases. Not renewed in a long time",
			)
		}
	}

	if !wasFullSnapshotLeaseCreatedRecently {
		return createBackupConditionResult(
			druidv1alpha1.ConditionFalse, BackupFailed,
			"Full snapshot backup failed. Full snapshot lease created long ago, but not renewed",
		)
	}

	return createBackupConditionResult(
		druidv1alpha1.ConditionTrue, BackupSucceeded,
		"Delta snapshot backup succeeded",
	)
}

// handleSnapshotterStartupCase handles cases where snapshotter has just started,
// so only full snapshot has been taken, while delta snapshot is still not taken.
// Returns `Unknown` condition for some time to allow delta snapshotting to begin, because
// even though the full snapshot may have succeeded within the required time, we must still wait
// for delta snapshotting to begin to consider the backups as healthy, to maintain the given RPO.
func handleSnapshotterStartupCase(fullSnapshotLeaseRenewTime time.Time, deltaSnapshotRenewalGracePeriod time.Duration) *result {
	if time.Since(fullSnapshotLeaseRenewTime) > deltaSnapshotRenewalGracePeriod {
		return createBackupConditionResult(
			druidv1alpha1.ConditionFalse, BackupFailed,
			"Delta snapshot backup failed. Delta snapshot lease not renewed in a long time",
		)
	}

	return createBackupConditionResult(
		druidv1alpha1.ConditionUnknown, Unknown,
		"Waiting for delta snapshotting to begin",
	)
}

// handleRenewedSnapshotLeases checks whether full and delta snapshot leases
// have been renewed within the required times respectively.
func handleRenewedSnapshotLeases(fullSnapshotLeaseRenewTime time.Time, fullSnapshotLeaseRenewalGracePeriod time.Duration, deltaSnapshotLeaseRenewTime time.Time, deltaSnapshotLeaseRenewalGracePeriod time.Duration) *result {
	isFullSnapshotLeaseStale := isLeaseStale(fullSnapshotLeaseRenewTime, fullSnapshotLeaseRenewalGracePeriod)
	isDeltaSnapshotLeaseStale := isLeaseStale(deltaSnapshotLeaseRenewTime, deltaSnapshotLeaseRenewalGracePeriod)

	if isFullSnapshotLeaseStale && !isDeltaSnapshotLeaseStale {
		return createBackupConditionResult(
			druidv1alpha1.ConditionFalse, BackupFailed,
			"Stale full snapshot lease. Not renewed in a long time",
		)
	}

	if !isFullSnapshotLeaseStale && isDeltaSnapshotLeaseStale {
		return createBackupConditionResult(
			druidv1alpha1.ConditionFalse, BackupFailed,
			"Stale delta snapshot lease. Not renewed in a long time",
		)
	}

	if isFullSnapshotLeaseStale && isDeltaSnapshotLeaseStale {
		return createBackupConditionResult(
			druidv1alpha1.ConditionFalse, BackupFailed,
			"Stale snapshot leases. Not renewed in a long time",
		)
	}

	return createBackupConditionResult(
		druidv1alpha1.ConditionTrue, BackupSucceeded,
		"Snapshot backup succeeded",
	)
}

// BackupReadyCheck returns a check for the "BackupReady" condition.
func BackupReadyCheck(cl client.Client) Checker {
	return &backupReadyCheck{
		cl: cl,
	}
}
