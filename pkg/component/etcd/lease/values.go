// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

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
