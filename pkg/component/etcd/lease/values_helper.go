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

package lease

import (
	"fmt"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
)

// GenerateValues generates `lease.Values` for the lease component with the given parameters.
func GenerateValues(etcd *druidv1alpha1.Etcd) Values {
	return Values{
		BackupEnabled:           etcd.Spec.Backup.Store != nil,
		EtcdName:                etcd.Name,
		EtcdUID:                 etcd.UID,
		DeltaSnapshotLeaseName:  GetDeltaSnapshotLeaseName(etcd),
		FullSnapshotLeaseName:   GetFullSnapshotLeaseName(etcd),
		ClusterRestoreLeaseName: GetClusterRestoreLeaseName(etcd),
		Replicas:                etcd.Spec.Replicas,
	}
}

// GetDeltaSnapshotLeaseName returns the name of the delta snapshot lease based on the given `etcd` object.
func GetDeltaSnapshotLeaseName(etcd *druidv1alpha1.Etcd) string {
	return fmt.Sprintf("%s-delta-snap", etcd.Name)
}

// GetFullSnapshotLeaseName returns the name of the full snapshot lease based on the given `etcd` object.
func GetFullSnapshotLeaseName(etcd *druidv1alpha1.Etcd) string {
	return fmt.Sprintf("%s-full-snap", etcd.Name)
}

// GetClusterRestoreLeaseName returns the name of the cluster restore indicator lease based on the given `etcd` object.
func GetClusterRestoreLeaseName(etcd *druidv1alpha1.Etcd) string {
	return fmt.Sprintf("%s-restoration-indicator", etcd.Name)
}
