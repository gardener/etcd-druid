// Copyright (c) 2021 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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
	coordinationv1 "k8s.io/api/coordination/v1"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type backupReadyCheck struct {
	cl client.Client
}

const (
	FullBackupSucceeded string = "FullBackupSucceeded"
	IncrBackupSucceeded string = "IncrementalBackupSucceeded"
	BackupFailed        string = "BackupFailed"
	Unknown             string = "Unknown"
)

func (a *backupReadyCheck) Check(etcd druidv1alpha1.Etcd) Result {
	//Default case
	result := &result{
		conType: druidv1alpha1.ConditionTypeBackupReady,
		status:  druidv1alpha1.ConditionFalse,
		reason:  Unknown,
		message: "Cannot determine etcd backup status since no snapshot leases present",
	}

	//Fetch snpashot leases
	fullSnapLease := &coordinationv1.Lease{}
	deltaSnapLease := &coordinationv1.Lease{}
	var fullSnapErr, incrSnapErr error
	fullSnapErr = a.cl.Get(context.TODO(), types.NamespacedName{Name: getFullSnapLeaseName(&etcd), Namespace: "default"}, fullSnapLease)
	incrSnapErr = a.cl.Get(context.TODO(), types.NamespacedName{Name: getDeltaSnapLeaseName(&etcd), Namespace: "default"}, deltaSnapLease)

	if fullSnapErr != nil && incrSnapErr != nil {
		return result
	}

	//Cases where snpashots are taken and lease updated
	if incrSnapErr == nil && deltaSnapLease.Spec.RenewTime != nil {
		deltaLeaseRenewTime := deltaSnapLease.Spec.RenewTime
		if time.Since(deltaLeaseRenewTime.Time) < 2*etcd.Spec.Backup.DeltaSnapshotPeriod.Duration {
			result.reason = IncrBackupSucceeded
			result.message = "Delta snapshot taken"
			result.status = druidv1alpha1.ConditionTrue
			return result

		}
	}
	if fullSnapErr == nil && fullSnapLease.Spec.RenewTime != nil {
		fullLeaseRenewTime := fullSnapLease.Spec.RenewTime
		if time.Since(fullLeaseRenewTime.Time) < 2*etcd.Spec.Backup.DeltaSnapshotPeriod.Duration {
			result.reason = FullBackupSucceeded
			result.message = "Full snapshot taken"
			result.status = druidv1alpha1.ConditionTrue
			return result
		}
	}

	//Cases where snapshot leases are not updated for a long time
	//If snapshot leases are present and leases aren't updated, it is safe to assume that backup is not healthy
	result.reason = BackupFailed
	result.message = "Stale snapshot leases. Not renewed in a long time"

	return result
}

func getDeltaSnapLeaseName(etcd *druidv1alpha1.Etcd) string {
	return fmt.Sprintf("%s-delta-snap", string(etcd.ObjectMeta.Name))
}

func getFullSnapLeaseName(etcd *druidv1alpha1.Etcd) string {
	return fmt.Sprintf("%s-full-snap", string(etcd.ObjectMeta.Name))
}

// BackupReadyCheck returns a check for the "BackupReady" condition.
func BackupReadyCheck(cl client.Client) Checker {
	return &backupReadyCheck{
		cl: cl,
	}
}
