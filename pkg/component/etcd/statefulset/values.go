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

package statefulset

import (
	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
)

// Values contains the values necessary for creating ETCD statefulset.
type Values struct {
	// Name is the name of the StatefulSet.
	Name string
	// Namespace is the namespace of StatefulSet.
	Namespace string

	// Name is the UID of the etcd resource.
	EtcdUID types.UID

	// Replicas is the number of ETCD instance that the ETCD cluster will have.
	Replicas int32
	// StatusReplicas is the number of replicas maintained in ETCD status.
	StatusReplicas int32

	// Annotations is the annotation provided in ETCD spec.
	Annotations map[string]string
	// Labels is the labels provided in ETCD spec.
	Labels map[string]string
	// BackupImage is the backup restore image.
	BackupImage string
	// EtcdImage is the etcd custom image.
	EtcdImage string
	// PriorityClassName is the Priority Class name.
	PriorityClassName *string
	// ServiceAccountName is the service account name.
	ServiceAccountName        string
	Affinity                  *corev1.Affinity
	TopologySpreadConstraints []corev1.TopologySpreadConstraint

	EtcdResourceRequirements   *corev1.ResourceRequirements
	BackupResourceRequirements *corev1.ResourceRequirements

	EtcdCommand           []string
	ReadinessProbeCommand []string
	EtcdBackupCommand     []string

	EnableClientTLS string
	EnablePeerTLS   string

	FailBelowRevision       string
	VolumeClaimTemplateName string

	FullSnapLeaseName  string
	DeltaSnapLeaseName string

	StorageCapacity *resource.Quantity
	StorageClass    *string

	DefragmentationSchedule *string
	FullSnapshotSchedule    *string

	EtcdSnapshotTimeout *metav1.Duration
	EtcdDefragTimeout   *metav1.Duration

	DeltaSnapshotMemoryLimit *resource.Quantity

	GarbageCollectionPolicy *druidv1alpha1.GarbageCollectionPolicy
	GarbageCollectionPeriod *metav1.Duration

	LeaderElection *druidv1alpha1.LeaderElectionSpec
	BackupStore    *druidv1alpha1.StoreSpec

	EnableProfiling *bool

	DeltaSnapshotPeriod *metav1.Duration

	SnapshotCompression *druidv1alpha1.CompressionSpec
	HeartbeatDuration   *metav1.Duration

	// MetricsLevel defines the level of detail for exported metrics of etcd, specify 'extensive' to include histogram metrics.
	MetricsLevel *druidv1alpha1.MetricsLevel
	// Quota defines the etcd DB quota.
	Quota *resource.Quantity

	// ClientUrlTLS holds the TLS configuration details for client communication.
	ClientUrlTLS *druidv1alpha1.TLSConfig
	// PeerUrlTLS hold the TLS configuration details for peer communication.
	PeerUrlTLS *druidv1alpha1.TLSConfig
	// BackupTLS hold the TLS configuration for communication with Backup server.
	BackupTLS *druidv1alpha1.TLSConfig

	//ClientServiceName is name of the etcd client service.
	ClientServiceName string
	// ClientPort holds the client port.
	ClientPort *int32
	//PeerServiceName is name of the etcd peer service.
	PeerServiceName string
	// ServerPort is the peer port.
	ServerPort *int32
	// ServerPort is the backup-restore side-car port.
	BackupPort *int32

	OwnerCheck *druidv1alpha1.OwnerCheckSpec
	// AutoCompactionMode defines the auto-compaction-mode: 'periodic' or 'revision'.
	AutoCompactionMode *druidv1alpha1.CompactionMode
	//AutoCompactionRetention defines the auto-compaction-retention length for etcd as well as for embedded-Etcd of backup-restore sidecar.
	AutoCompactionRetention *string
	// ConfigMapName is the name of the configmap that holds the ETCD config.
	ConfigMapName           string
	PeerTLSChangedToEnabled bool
}
