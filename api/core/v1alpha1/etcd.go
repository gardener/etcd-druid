// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// GarbageCollectionPolicyExponential defines the exponential policy for garbage collecting old backups
	GarbageCollectionPolicyExponential = "Exponential"
	// GarbageCollectionPolicyLimitBased defines the limit based policy for garbage collecting old backups
	GarbageCollectionPolicyLimitBased = "LimitBased"

	// Basic is a constant for metrics level basic.
	Basic MetricsLevel = "basic"
	// Extensive is a constant for metrics level extensive.
	Extensive MetricsLevel = "extensive"

	// GzipCompression is constant for gzip compression policy.
	GzipCompression CompressionPolicy = "gzip"
	// LzwCompression is constant for lzw compression policy.
	LzwCompression CompressionPolicy = "lzw"
	// ZlibCompression is constant for zlib compression policy.
	ZlibCompression CompressionPolicy = "zlib"

	// DefaultCompression is constant for default compression policy(only if compression is enabled).
	DefaultCompression = GzipCompression
	// DefaultCompressionEnabled is constant to define whether to compress the snapshots or not.
	DefaultCompressionEnabled = false

	// Periodic is a constant to set auto-compaction-mode 'periodic' for duration based retention.
	Periodic CompactionMode = "periodic"
	// Revision is a constant to set auto-compaction-mode 'revision' for revision number based retention.
	Revision CompactionMode = "revision"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:subresource:scale:specpath=.spec.replicas,statuspath=.status.replicas,selectorpath=.status.selector
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.ready`
// +kubebuilder:printcolumn:name="Quorate",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="All Members Ready",type=string,JSONPath=`.status.conditions[?(@.type=="AllMembersReady")].status`
// +kubebuilder:printcolumn:name="Backup Ready",type=string,JSONPath=`.status.conditions[?(@.type=="BackupReady")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:printcolumn:name="Cluster Size",type=integer,JSONPath=`.spec.replicas`,priority=1
// +kubebuilder:printcolumn:name="Current Replicas",type=integer,JSONPath=`.status.currentReplicas`,priority=1
// +kubebuilder:printcolumn:name="Ready Replicas",type=integer,JSONPath=`.status.readyReplicas`,priority=1

// Etcd is the Schema for the etcds API
type Etcd struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EtcdSpec   `json:"spec,omitempty"`
	Status EtcdStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// EtcdList contains a list of Etcd
type EtcdList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Etcd `json:"items"`
}

// MetricsLevel defines the level 'basic' or 'extensive'.
// +kubebuilder:validation:Enum=basic;extensive
type MetricsLevel string

// GarbageCollectionPolicy defines the type of policy for snapshot garbage collection.
// +kubebuilder:validation:Enum=Exponential;LimitBased
type GarbageCollectionPolicy string

// CompressionPolicy defines the type of policy for compression of snapshots.
// +kubebuilder:validation:Enum=gzip;lzw;zlib
type CompressionPolicy string

// CompactionMode defines the auto-compaction-mode: 'periodic' or 'revision'.
// 'periodic' for duration based retention and 'revision' for revision number based retention.
// +kubebuilder:validation:Enum=periodic;revision
type CompactionMode string

// TLSConfig hold the TLS configuration details.
type TLSConfig struct {
	// +required
	TLSCASecretRef SecretReference `json:"tlsCASecretRef"`
	// +required
	ServerTLSSecretRef corev1.SecretReference `json:"serverTLSSecretRef"`
	// +optional
	ClientTLSSecretRef corev1.SecretReference `json:"clientTLSSecretRef"`
}

// SecretReference defines a reference to a secret.
type SecretReference struct {
	corev1.SecretReference `json:",inline"`
	// DataKey is the name of the key in the data map containing the credentials.
	// +optional
	DataKey *string `json:"dataKey,omitempty"`
}

// CompressionSpec defines parameters related to compression of Snapshots(full as well as delta).
type CompressionSpec struct {
	// +optional
	Enabled *bool `json:"enabled,omitempty"`
	// +optional
	Policy *CompressionPolicy `json:"policy,omitempty"`
}

// LeaderElectionSpec defines parameters related to the LeaderElection configuration.
type LeaderElectionSpec struct {
	// ReelectionPeriod defines the Period after which leadership status of corresponding etcd is checked.
	// +optional
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Pattern="^([0-9]+(\\.[0-9]+)?(ns|us|µs|ms|s|m|h))+$"
	ReelectionPeriod *metav1.Duration `json:"reelectionPeriod,omitempty"`
	// EtcdConnectionTimeout defines the timeout duration for etcd client connection during leader election.
	// +optional
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Pattern="^([0-9]+(\\.[0-9]+)?(ns|us|µs|ms|s|m|h))+$"
	EtcdConnectionTimeout *metav1.Duration `json:"etcdConnectionTimeout,omitempty"`
}

// BackupSpec defines parameters associated with the full and delta snapshots of etcd.
// +kubebuilder:validation:XValidation:message="etcd.spec.backup.garbageCollectionPeriod must be greater than etcd.spec.backup.deltaSnapshotPeriod",rule="!(has(self.deltaSnapshotPeriod) && has(self.garbageCollectionPeriod)) || duration(self.deltaSnapshotPeriod).getSeconds() < duration(self.garbageCollectionPeriod).getSeconds()"
type BackupSpec struct {
	// Port define the port on which etcd-backup-restore server will be exposed.
	// +optional
	Port *int32 `json:"port,omitempty"`
	// +optional
	TLS *TLSConfig `json:"tls,omitempty"`
	// Image defines the etcd container image and tag
	// +optional
	Image *string `json:"image,omitempty"`
	// Store defines the specification of object store provider for storing backups.
	// +optional
	Store *StoreSpec `json:"store,omitempty"`
	// Resources defines compute Resources required by backup-restore container.
	// More info: https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
	// SnapshotCompaction defines the specification for compaction of backups.
	// +optional
	SnapshotCompaction *SnapshotCompactionSpec `json:"snapshotCompaction,omitempty"`
	// FullSnapshotSchedule defines the cron standard schedule for full snapshots.
	// +optional
	// +kubebuilder:validation:Pattern="^(\\*|[1-5]?[0-9]|[1-5]?[0-9]-[1-5]?[0-9]|(?:[1-9]|[1-4][0-9]|5[0-9])\\/(?:[1-9]|[1-4][0-9]|5[0-9]|60)|\\*\\/(?:[1-9]|[1-4][0-9]|5[0-9]|60))\\s+(\\*|[0-9]|1[0-9]|2[0-3]|[0-9]-(?:[0-9]|1[0-9]|2[0-3])|1[0-9]-(?:1[0-9]|2[0-3])|2[0-3]-2[0-3]|(?:[1-9]|1[0-9]|2[0-3])\\/(?:[1-9]|1[0-9]|2[0-4])|\\*\\/(?:[1-9]|1[0-9]|2[0-4]))\\s+(\\*|[1-9]|[12][0-9]|3[01]|[1-9]-(?:[1-9]|[12][0-9]|3[01])|[12][0-9]-(?:[12][0-9]|3[01])|3[01]-3[01]|(?:[1-9]|[12][0-9]|30)\\/(?:[1-9]|[12][0-9]|3[01])|\\*\\/(?:[1-9]|[12][0-9]|3[01]))\\s+(\\*|[1-9]|1[0-2]|[1-9]-(?:[1-9]|1[0-2])|1[0-2]-1[0-2]|(?:[1-9]|1[0-2])\\/(?:[1-9]|1[0-2])|\\*\\/(?:[1-9]|1[0-2]))\\s+(\\*|[1-7]|[1-6]-[1-7]|[1-6]\\/[1-7]|\\*\\/[1-7])$"
	FullSnapshotSchedule *string `json:"fullSnapshotSchedule,omitempty"`
	// GarbageCollectionPolicy defines the policy for garbage collecting old backups
	// +optional
	GarbageCollectionPolicy *GarbageCollectionPolicy `json:"garbageCollectionPolicy,omitempty"`
	// MaxBackupsLimitBasedGC defines the maximum number of Full snapshots to retain in Limit Based GarbageCollectionPolicy
	// All full snapshots beyond this limit will be garbage collected.
	// +optional
	MaxBackupsLimitBasedGC *int32 `json:"maxBackupsLimitBasedGC,omitempty"`
	// GarbageCollectionPeriod defines the period for garbage collecting old backups
	// +optional
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Pattern="^([0-9]+(\\.[0-9]+)?(ns|us|µs|ms|s|m|h))+$"
	GarbageCollectionPeriod *metav1.Duration `json:"garbageCollectionPeriod,omitempty"`
	// DeltaSnapshotPeriod defines the period after which delta snapshots will be taken
	// +optional
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Pattern="^([0-9]+(\\.[0-9]+)?(ns|us|µs|ms|s|m|h))+$"
	DeltaSnapshotPeriod *metav1.Duration `json:"deltaSnapshotPeriod,omitempty"`
	// DeltaSnapshotMemoryLimit defines the memory limit after which delta snapshots will be taken
	// +optional
	DeltaSnapshotMemoryLimit *resource.Quantity `json:"deltaSnapshotMemoryLimit,omitempty"`
	// DeltaSnapshotRetentionPeriod defines the duration for which delta snapshots will be retained, excluding the latest snapshot set.
	// The value should be a string formatted as a duration (e.g., '1s', '2m', '3h', '4d')
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Pattern="^([0-9]+(\\.[0-9]+)?(ns|us|µs|ms|s|m|h))+$"
	// +optional
	DeltaSnapshotRetentionPeriod *metav1.Duration `json:"deltaSnapshotRetentionPeriod,omitempty"`

	// SnapshotCompression defines the specification for compression of Snapshots.
	// +optional
	SnapshotCompression *CompressionSpec `json:"compression,omitempty"`
	// EnableProfiling defines if profiling should be enabled for the etcd-backup-restore-sidecar
	// +optional
	EnableProfiling *bool `json:"enableProfiling,omitempty"`
	// EtcdSnapshotTimeout defines the timeout duration for etcd FullSnapshot operation
	// +optional
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Pattern="^([0-9]+(\\.[0-9]+)?(ns|us|µs|ms|s|m|h))+$"
	EtcdSnapshotTimeout *metav1.Duration `json:"etcdSnapshotTimeout,omitempty"`
	// LeaderElection defines parameters related to the LeaderElection configuration.
	// +optional
	LeaderElection *LeaderElectionSpec `json:"leaderElection,omitempty"`
}

// SnapshotCompactionSpec defines parameters related to the compaction job configuration.
type SnapshotCompactionSpec struct {
	// Resources defines compute Resources required by compaction job.
	// More info: https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
	// EventsThreshold defines the threshold for the number of etcd events before triggering a compaction job
	// +optional
	EventsThreshold *int64 `json:"eventsThreshold,omitempty"`
	// TriggerFullSnapshotThreshold defines the upper threshold for the number of etcd events before giving up on compaction job and triggering a full snapshot.
	// +optional
	TriggerFullSnapshotThreshold *int64 `json:"triggerFullSnapshotThreshold,omitempty"`
}

// EtcdConfig defines the configuration for the etcd cluster to be deployed.
type EtcdConfig struct {
	// Quota defines the etcd DB quota.
	// +optional
	Quota *resource.Quantity `json:"quota,omitempty"`
	// SnapshotCount defines the number of applied Raft entries to hold in-memory before compaction.
	// More info: https://etcd.io/docs/v3.4/op-guide/maintenance/#raft-log-retention
	// +optional
	SnapshotCount *int64 `json:"snapshotCount,omitempty"`
	// DefragmentationSchedule defines the cron standard schedule for defragmentation of etcd.
	// +optional
	// +kubebuilder:validation:Pattern="^(\\*|[1-5]?[0-9]|[1-5]?[0-9]-[1-5]?[0-9]|(?:[1-9]|[1-4][0-9]|5[0-9])\\/(?:[1-9]|[1-4][0-9]|5[0-9]|60)|\\*\\/(?:[1-9]|[1-4][0-9]|5[0-9]|60))\\s+(\\*|[0-9]|1[0-9]|2[0-3]|[0-9]-(?:[0-9]|1[0-9]|2[0-3])|1[0-9]-(?:1[0-9]|2[0-3])|2[0-3]-2[0-3]|(?:[1-9]|1[0-9]|2[0-3])\\/(?:[1-9]|1[0-9]|2[0-4])|\\*\\/(?:[1-9]|1[0-9]|2[0-4]))\\s+(\\*|[1-9]|[12][0-9]|3[01]|[1-9]-(?:[1-9]|[12][0-9]|3[01])|[12][0-9]-(?:[12][0-9]|3[01])|3[01]-3[01]|(?:[1-9]|[12][0-9]|30)\\/(?:[1-9]|[12][0-9]|3[01])|\\*\\/(?:[1-9]|[12][0-9]|3[01]))\\s+(\\*|[1-9]|1[0-2]|[1-9]-(?:[1-9]|1[0-2])|1[0-2]-1[0-2]|(?:[1-9]|1[0-2])\\/(?:[1-9]|1[0-2])|\\*\\/(?:[1-9]|1[0-2]))\\s+(\\*|[1-7]|[1-6]-[1-7]|[1-6]\\/[1-7]|\\*\\/[1-7])$"
	DefragmentationSchedule *string `json:"defragmentationSchedule,omitempty"`
	// +optional
	ServerPort *int32 `json:"serverPort,omitempty"`
	// +optional
	ClientPort *int32 `json:"clientPort,omitempty"`
	// +optional
	WrapperPort *int32 `json:"wrapperPort,omitempty"`
	// Image defines the etcd container image and tag
	// +optional
	Image *string `json:"image,omitempty"`
	// +optional
	AuthSecretRef *corev1.SecretReference `json:"authSecretRef,omitempty"`
	// Metrics defines the level of detail for exported metrics of etcd, specify 'extensive' to include histogram metrics.
	// +optional
	Metrics *MetricsLevel `json:"metrics,omitempty"`
	// Resources defines the compute Resources required by etcd container.
	// More info: https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
	// ClientUrlTLS contains the ca, server TLS and client TLS secrets for client communication to ETCD cluster
	// +optional
	ClientUrlTLS *TLSConfig `json:"clientUrlTls,omitempty"`
	// PeerUrlTLS contains the ca and server TLS secrets for peer communication within ETCD cluster
	// Currently, PeerUrlTLS does not require client TLS secrets for gardener implementation of ETCD cluster.
	// +optional
	PeerUrlTLS *TLSConfig `json:"peerUrlTls,omitempty"`
	// EtcdDefragTimeout defines the timeout duration for etcd defrag call
	// +optional
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Pattern="^([0-9]+(\\.[0-9]+)?(ns|us|µs|ms|s|m|h))+$"
	EtcdDefragTimeout *metav1.Duration `json:"etcdDefragTimeout,omitempty"`
	// HeartbeatDuration defines the duration for members to send heartbeats. The default value is 10s.
	// +optional
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Pattern="^([0-9]+(\\.[0-9]+)?(ns|us|µs|ms|s|m|h))+$"
	HeartbeatDuration *metav1.Duration `json:"heartbeatDuration,omitempty"`
	// ClientService defines the parameters of the client service that a user can specify
	// +optional
	ClientService *ClientService `json:"clientService,omitempty"`
}

// ClientService defines the parameters of the client service that a user can specify
type ClientService struct {
	// Annotations specify the annotations that should be added to the client service
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
	// Labels specify the labels that should be added to the client service
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
	// TrafficDistribution defines the traffic distribution preference that should be added to the client service.
	// More info: https://kubernetes.io/docs/reference/networking/virtual-ips/#traffic-distribution
	// +optional
	// +kubebuilder:validation:Enum=PreferClose
	TrafficDistribution *string `json:"trafficDistribution,omitempty"`
}

// SharedConfig defines parameters shared and used by Etcd as well as backup-restore sidecar.
type SharedConfig struct {
	// AutoCompactionMode defines the auto-compaction-mode:'periodic' mode or 'revision' mode for etcd and embedded-etcd of backup-restore sidecar.
	// +optional
	AutoCompactionMode *CompactionMode `json:"autoCompactionMode,omitempty"`
	// AutoCompactionRetention defines the auto-compaction-retention length for etcd as well as for embedded-etcd of backup-restore sidecar.
	// +optional
	AutoCompactionRetention *string `json:"autoCompactionRetention,omitempty"`
}

// SchedulingConstraints defines the different scheduling constraints that must be applied to the
// pod spec in the etcd statefulset.
// Currently supported constraints are Affinity and TopologySpreadConstraints.
type SchedulingConstraints struct {
	// Affinity defines the various affinity and anti-affinity rules for a pod
	// that are honoured by the kube-scheduler.
	// +optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`
	// TopologySpreadConstraints describes how a group of pods ought to spread across topology domains,
	// that are honoured by the kube-scheduler.
	// +optional
	TopologySpreadConstraints []corev1.TopologySpreadConstraint `json:"topologySpreadConstraints,omitempty"`
}

// EtcdSpec defines the desired state of Etcd
// +kubebuilder:validation:XValidation:message="etcd.spec.storageClass is an immutable field.",rule="has(oldSelf.storageClass) ==  has(self.storageClass)"
// +kubebuilder:validation:XValidation:message="etcd.spec.emptyDirVolumeSource can not be removed once set.",rule="has(oldSelf.emptyDirVolumeSource) ? has(self.emptyDirVolumeSource) : true"
// +kubebuilder:validation:XValidation:message="etcd.spec.volumeClaimTemplate and etcd.spec.emptyDirVolumeSource can not be both set.",rule="!(has(self.volumeClaimTemplate) && has(self.emptyDirVolumeSource))"
type EtcdSpec struct {
	// selector is a label query over pods that should match the replica count.
	// It must match the pod template's labels.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#label-selectors
	// +optional
	// Deprecated: this field will be removed in the future.
	Selector *metav1.LabelSelector `json:"selector"`
	// +required
	Labels map[string]string `json:"labels"`
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
	// +required
	Etcd EtcdConfig `json:"etcd"`
	// +required
	Backup BackupSpec `json:"backup"`
	// +optional
	Common SharedConfig `json:"sharedConfig,omitempty"`
	// +optional
	SchedulingConstraints SchedulingConstraints `json:"schedulingConstraints,omitempty"`
	// +required
	// +kubebuilder:validation:XValidation:message="Replicas can either be increased or be downscaled to 0.",rule="self==0 ? true : self < oldSelf ? false : true"
	Replicas int32 `json:"replicas"`
	// PriorityClassName is the name of a priority class that shall be used for the etcd pods.
	// +optional
	PriorityClassName *string `json:"priorityClassName,omitempty"`
	// StorageClass defines the name of the StorageClass required by the claim.
	// More info: https://kubernetes.io/docs/concepts/storage/persistent-volumes#class-1
	// +optional
	// +kubebuilder:validation:XValidation:message="etcd.spec.storageClass is an immutable field",rule="self == oldSelf"
	StorageClass *string `json:"storageClass,omitempty"`
	// StorageCapacity defines the size of persistent volume.
	// +optional
	StorageCapacity *resource.Quantity `json:"storageCapacity,omitempty"`
	// VolumeClaimTemplate defines the volume claim template to be created
	// +optional
	// +kubebuilder:validation:XValidation:message="etcd.spec.volumeClaimTemplate can only be set and unset",rule="self == oldSelf"
	VolumeClaimTemplate *string `json:"volumeClaimTemplate,omitempty"`
	// RunAsRoot defines whether the securityContext of the pod specification should indicate that the containers shall
	// run as root. By default, they run as non-root with user 'nobody'.
	// +optional
	RunAsRoot *bool `json:"runAsRoot,omitempty"`
	// EmptyDirVolumeSource defines the emptyDirVolumeSource that is specified to make use of emtpyDir storage for the etcd data directories.
	// The user does not get the option to configure the name of these volumes.
	// This feature is currently in ALPHA. etcd-druid only supports migrating etcd clusters from using PVCs to emptyDir volumes.
	// The inverse is explicitly NOT supported currently. Once migrated, you CANNOT migrate back to CSI volumes.
	// +optional
	// +kubebuilder:validation:XValidation:message="etcd.spec.emptyDirVolumeSource is an immutable field once set.",rule="self == oldSelf"
	EmptyDirVolumeSource *corev1.EmptyDirVolumeSource `json:"emptyDirVolumeSource,omitempty"`
}

// CrossVersionObjectReference contains enough information to let you identify the referred resource.
type CrossVersionObjectReference struct {
	// Kind of the referent
	// +required
	Kind string `json:"kind,omitempty"`
	// Name of the referent
	// +required
	Name string `json:"name,omitempty"`
	// API version of the referent
	// +optional
	APIVersion string `json:"apiVersion,omitempty"`
}

const (
	// ConditionTypeReady is a constant for a condition type indicating that the etcd cluster is ready.
	ConditionTypeReady ConditionType = "Ready"
	// ConditionTypeLastSnapshotCompactionSucceeded is a constant for a condition type indicating the status of last snapshot compaction.
	// If `ConditionTypeLastSnapshotCompactionSucceeded` condition status is `False`, it means the compaction controller is currently retrying the compaction operation.
	// Compaction operation can either be a compaction job or a full snapshot.
	ConditionTypeLastSnapshotCompactionSucceeded ConditionType = "LastSnapshotCompactionSucceeded"
	// ConditionTypeAllMembersReady is a constant for a condition type indicating that all members of the etcd cluster are ready.
	ConditionTypeAllMembersReady ConditionType = "AllMembersReady"
	// ConditionTypeAllMembersUpdated is a constant for a condition type indicating that all members
	// of the etcd cluster have been updated with the desired spec changes.
	ConditionTypeAllMembersUpdated ConditionType = "AllMembersUpdated"
	// ConditionTypeBackupReady is a constant for a condition type indicating that the etcd backup is ready.
	ConditionTypeBackupReady ConditionType = "BackupReady"
	// ConditionTypeDataVolumesReady is a constant for a condition type indicating that the etcd data volumes are ready.
	ConditionTypeDataVolumesReady ConditionType = "DataVolumesReady"
)

// EtcdMemberConditionStatus is the status of an etcd cluster member.
type EtcdMemberConditionStatus string

const (
	// EtcdMemberStatusReady indicates that the etcd member is ready.
	EtcdMemberStatusReady EtcdMemberConditionStatus = "Ready"
	// EtcdMemberStatusNotReady indicates that the etcd member is not ready.
	EtcdMemberStatusNotReady EtcdMemberConditionStatus = "NotReady"
	// EtcdMemberStatusUnknown indicates that the status of the etcd member is unknown.
	EtcdMemberStatusUnknown EtcdMemberConditionStatus = "Unknown"
)

// EtcdRole is the role of an etcd cluster member.
type EtcdRole string

const (
	// EtcdRoleLeader describes the etcd role `Leader`.
	EtcdRoleLeader EtcdRole = "Leader"
	// EtcdRoleMember describes the etcd role `Member`.
	EtcdRoleMember EtcdRole = "Member"
)

// EtcdMemberStatus holds information about etcd cluster membership.
type EtcdMemberStatus struct {
	// Name is the name of the etcd member. It is the name of the backing `Pod`.
	Name string `json:"name"`
	// ID is the ID of the etcd member.
	// +optional
	ID *string `json:"id,omitempty"`
	// Role is the role in the etcd cluster, either `Leader` or `Member`.
	// +optional
	Role *EtcdRole `json:"role,omitempty"`
	// Status of the condition, one of True, False, Unknown.
	Status EtcdMemberConditionStatus `json:"status"`
	// The reason for the condition's last transition.
	Reason string `json:"reason"`
	// LastTransitionTime is the last time the condition's status changed.
	LastTransitionTime metav1.Time `json:"lastTransitionTime"`
}

// EtcdStatus defines the observed state of Etcd.
type EtcdStatus struct {
	// ObservedGeneration is the most recent generation observed for this resource.
	// +optional
	ObservedGeneration *int64 `json:"observedGeneration,omitempty"`
	// +optional
	Etcd *CrossVersionObjectReference `json:"etcd,omitempty"`
	// Conditions represents the latest available observations of an etcd's current state.
	// +optional
	Conditions []Condition `json:"conditions,omitempty"`
	// LastErrors captures errors that occurred during the last operation.
	// +optional
	LastErrors []LastError `json:"lastErrors,omitempty"`
	// LastOperation indicates the last operation performed on this resource.
	// +optional
	LastOperation *LastOperation `json:"lastOperation,omitempty"`
	// CurrentReplicas is the current replica count for the etcd cluster.
	// +optional
	CurrentReplicas int32 `json:"currentReplicas,omitempty"`
	// Replicas is the replica count of the etcd cluster.
	// +optional
	Replicas int32 `json:"replicas,omitempty"`
	// ReadyReplicas is the count of replicas being ready in the etcd cluster.
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`
	// Ready is `true` if all etcd replicas are ready.
	// +optional
	Ready *bool `json:"ready,omitempty"`
	// LabelSelector is a label query over pods that should match the replica count.
	// It must match the pod template's labels.
	// Deprecated: this field will be removed in the future.
	// +optional
	LabelSelector *metav1.LabelSelector `json:"labelSelector,omitempty"`
	// Members represents the members of the etcd cluster
	// +optional
	Members []EtcdMemberStatus `json:"members,omitempty"`
	// PeerUrlTLSEnabled captures the state of peer url TLS being enabled for the etcd member(s)
	// +optional
	PeerUrlTLSEnabled *bool `json:"peerUrlTLSEnabled,omitempty"`
	// Selector is a label query over pods that should match the replica count.
	// It must match the pod template's labels.
	// +optional
	Selector *string `json:"selector,omitempty"`
}

// LastOperationType is a string alias representing type of the last operation.
type LastOperationType string

const (
	// LastOperationTypeCreate indicates that the last operation was a creation of a new Etcd resource.
	LastOperationTypeCreate LastOperationType = "Create"
	// LastOperationTypeReconcile indicates that the last operation was a reconciliation of the spec of an Etcd resource.
	LastOperationTypeReconcile LastOperationType = "Reconcile"
	// LastOperationTypeDelete indicates that the last operation was a deletion of an existing Etcd resource.
	LastOperationTypeDelete LastOperationType = "Delete"
)

// LastOperationState is a string alias representing the state of the last operation.
type LastOperationState string

const (
	// LastOperationStateProcessing indicates that an operation is in progress.
	LastOperationStateProcessing LastOperationState = "Processing"
	// LastOperationStateSucceeded indicates that an operation has completed successfully.
	LastOperationStateSucceeded LastOperationState = "Succeeded"
	// LastOperationStateError indicates that an operation is completed with errors and will be retried.
	LastOperationStateError LastOperationState = "Error"
	// LastOperationStateRequeue indicates that an operation is not completed and either due to an error or unfulfilled conditions will be retried.
	LastOperationStateRequeue LastOperationState = "Requeue"
)

// LastOperation holds the information on the last operation done on the Etcd resource.
type LastOperation struct {
	// Type is the type of last operation.
	Type LastOperationType `json:"type"`
	// State is the state of the last operation.
	State LastOperationState `json:"state"`
	// Description describes the last operation.
	Description string `json:"description"`
	// RunID correlates an operation with a reconciliation run.
	// Every time an Etcd resource is reconciled (barring status reconciliation which is periodic), a unique ID is
	// generated which can be used to correlate all actions done as part of a single reconcile run. Capturing this
	// as part of LastOperation aids in establishing this correlation. This further helps in also easily filtering
	// reconcile logs as all structured logs in a reconciliation run should have the `runID` referenced.
	RunID string `json:"runID"`
	// LastUpdateTime is the time at which the operation was last updated.
	LastUpdateTime metav1.Time `json:"lastUpdateTime"`
}

// ErrorCode is a string alias representing an error code that identifies an error.
type ErrorCode string

// LastError stores details of the most recent error encountered for a resource.
type LastError struct {
	// Code is an error code that uniquely identifies an error.
	Code ErrorCode `json:"code"`
	// Description is a human-readable message indicating details of the error.
	Description string `json:"description"`
	// ObservedAt is the time the error was observed.
	ObservedAt metav1.Time `json:"observedAt"`
}

// IsBackupStoreEnabled returns true if backup store has been enabled for the Etcd resource, else returns false.
func (e *Etcd) IsBackupStoreEnabled() bool {
	return e.Spec.Backup.Store != nil
}

// IsReconciliationInProgress returns true if the Etcd resource is currently being reconciled, else returns false.
func (e *Etcd) IsReconciliationInProgress() bool {
	return e.Status.LastOperation != nil &&
		e.Status.LastOperation.Type == LastOperationTypeReconcile &&
		(e.Status.LastOperation.State == LastOperationStateProcessing ||
			e.Status.LastOperation.State == LastOperationStateError ||
			e.Status.LastOperation.State == LastOperationStateRequeue)
}

// IsDeletionInProgress returns true if the Etcd resource is currently being reconciled, else returns false.
func (e *Etcd) IsDeletionInProgress() bool {
	return e.Status.LastOperation != nil &&
		e.Status.LastOperation.Type == LastOperationTypeDelete &&
		(e.Status.LastOperation.State == LastOperationStateProcessing ||
			e.Status.LastOperation.State == LastOperationStateError ||
			e.Status.LastOperation.State == LastOperationStateRequeue)
}
