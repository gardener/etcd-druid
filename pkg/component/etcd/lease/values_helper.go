// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package lease

import (
	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
)

// GenerateValues generates `lease.Values` for the lease component
func GenerateValues(etcd *druidv1alpha1.Etcd) Values {
	return Values{
		BackupEnabled:          etcd.Spec.Backup.Store != nil,
		EtcdName:               etcd.Name,
		DeltaSnapshotLeaseName: etcd.GetDeltaSnapshotLeaseName(),
		FullSnapshotLeaseName:  etcd.GetFullSnapshotLeaseName(),
		Replicas:               etcd.Spec.Replicas,
		Labels:                 etcd.GetDefaultLabels(),
		OwnerReference:         etcd.GetAsOwnerReference(),
	}
}
