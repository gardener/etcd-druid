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

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// Values contains the values necessary for creating ETCD leases.
type Values struct {
	// BackupEnabled specifies if the backup functionality for the etcd cluster is enabled.
	BackupEnabled bool
	// EtcdName is the name of the etcd resource.
	EtcdName string
	// DeltaSnapshotLeaseName is the name of the delta snapshot lease object.
	DeltaSnapshotLeaseName string
	// FullSnapshotLeaseName is the name of the full snapshot lease object.
	FullSnapshotLeaseName string
	// Replicas is the replica count of the etcd cluster.
	Replicas int32
	// Labels is the labels of deployed configmap
	Labels map[string]string
	// OwnerReference is the OwnerReference for the Configmap.
	OwnerReference metav1.OwnerReference
}
