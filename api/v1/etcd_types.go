/*
Copyright (c) 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PVCRetentionPolicy defines policies for deleting/retaining Etcd PVCs
type PVCRetentionPolicy string

const (
	// PolicyDeleteAll is a constant for a policy type indicating that all the PVCs of etcd instances has to be deleted.
	PolicyDeleteAll PVCRetentionPolicy = "DeleteAll"
	// PolicyRetainMaster is a constant for a policy type indicating that all the PVCs except that of master has to be deleted.
	PolicyRetainMaster PVCRetentionPolicy = "RetainMaster"
	// PolicyRetainAll is a constant for a policy type indicating that all the PVCs of etcd has to be retained.
	PolicyRetainAll PVCRetentionPolicy = "RetRetainAllainMaster"
	// GarbageCollectionPolicyExponential defines the exponential policy for garbage collecting old backups
	GarbageCollectionPolicyExponential = "Exponential"
	// GarbageCollectionPolicyLimitBased defines the limit based policy for garbage collecting old backups
	GarbageCollectionPolicyLimitBased = "LimitBased"
)

// MetricLevel defines the level 'basic' or 'extensive'.
type MetricLevel string

// GarbageCollectionPolicy defines the type of policy for snapshot garbage collection.
type GarbageCollectionPolicy string

const (
	// Basic is a constant for metric level basic.
	Basic MetricLevel = "basic"
	// Extensive is a constant for metric level extensive.
	Extensive MetricLevel = "extensive"
)

// Spec defines the desired state of Etcd
type Spec struct {
	// +required
	Etcd EtcdSpec `json:"etcd"`
	// +required
	Backup BackupSpec `json:"backup"`
	// +required
	Store StoreSpec `json:"store"`
	// +optional
	PVCRetentionPolicy PVCRetentionPolicy `json:"pvcRetentionPolicy,omitempty"`
	// +required
	Replicas int `json:"replicas"`
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
	// +required
	StorageClass string `json:"storageClass"`
	// +optional
	TLSServerSecretName string `json:"tlsServerSecret,omitempty"`
	// +optional
	TLSClientSecretName string `json:"tlsClientSecret,omitempty"`
	// +optional
	StorageCapacity string `json:"storageCapacity,omitempty"`
}

// StoreSpec defines parameters related to ObjectStore persisting backups
type StoreSpec struct {
	// +required
	StorageContainer string `json:"storageContainer"`
	// +optional
	StorePrefix string `json:"storePrefix,omitempty"`
	// +required
	StorageProvider string `json:"storageProvider"`
	// +required
	StoreSecret string `json:"storeSecret"`
}

// BackupSpec defines parametes associated with the full and delta snapshots of etcd
type BackupSpec struct {
	// +optional
	Port int `json:"port,omitempty"`
	// +required
	ImageRepository string `json:"imageRepository"`
	// +required
	ImageVersion string `json:"imageVersion"`
	// +optional
	FullSnapshotSchedule string `json:"fullSnapshotSchedule,omitempty"`
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
	// +optional
	PullPolicy corev1.PullPolicy `json:"pullPolicy,omitempty"`
	// +optional
	GarbageCollectionPolicy GarbageCollectionPolicy `json:"garbageCollectionPolicy,omitempty"`
	// +optional
	GarbageCollectionPeriod string `json:"garbageCollectionPeriod,omitempty"`
	// +optional
	EtcdQuotaBytes int `json:"etcdQuotaBytes,omitempty"`
	// +optional
	EtcdConnectionTimeout string `json:"etcdConnectionTimeout,omitempty"`
	// +optional
	SnapstoreTempDir string `json:"snapstoreTempDir,omitempty"`
	// +optional
	DeltaSnapshotPeriod string `json:"deltaSnapshotPeriod,omitempty"`
	// +optional
	DeltaSnapshotMemoryLimit int `json:"deltaSnapshotMemoryLimit,omitempty"`
}

// EtcdSpec defines parametes associated etcd deployed
type EtcdSpec struct {
	// +optional
	DefragmentationSchedule string `json:"defragmentationSchedule,omitempty"`
	// +optional
	ServerPort int `json:"serverPort,omitempty"`
	// +optional
	ClientPort int `json:"clientPort,omitempty"`
	// +required
	ImageRepository string `json:"imageRepository"`
	// +required
	ImageVersion string `json:"imageVersion"`
	// +optional
	MetricLevel MetricLevel `json:"metrics,omitempty"`
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
	// +required
	EnableTLS bool `json:"enableTLS"`
	// +optional
	PullPolicy corev1.PullPolicy `json:"pullPolicy,omitempty"`
	// +optional
	Username string `json:"username,omitempty"`
	// +optional
	Password string `json:"password,omitempty"`
	// +required
	InitialClusterToken string `json:"initialClusterToken"`
	// +required
	InitialClusterState string `json:"initialClusterState"`
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

// ConditionStatus is the status of a condition.
type ConditionStatus string

// ConditionType is a string alias.
type ConditionType string

const (
	// ConditionAvailable is a condition type for indicating availability.
	ConditionAvailable ConditionType = "Available"

	// ConditionTrue means a resource is in the condition.
	ConditionTrue ConditionStatus = "True"
	// ConditionFalse means a resource is not in the condition.
	ConditionFalse ConditionStatus = "False"
	// ConditionUnknown means Gardener can't decide if a resource is in the condition or not.
	ConditionUnknown ConditionStatus = "Unknown"
	// ConditionProgressing means the condition was seen true, failed but stayed within a predefined failure threshold.
	// In the future, we could add other intermediate conditions, e.g. ConditionDegraded.
	ConditionProgressing ConditionStatus = "Progressing"

	// ConditionCheckError is a constant for a reason in condition.
	ConditionCheckError = "ConditionCheckError"
)

// Condition holds the information about the state of a resource.
type Condition struct {
	// Type of the Shoot condition.
	Type ConditionType `json:"type,omitempty"`
	// Status of the condition, one of True, False, Unknown.
	Status ConditionStatus `json:"status,omitempty"`
	// Last time the condition transitioned from one status to another.
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
	// Last time the condition was updated.
	LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty"`
	// The reason for the condition's last transition.
	Reason string `json:"reason,omitempty"`
	// A human readable message indicating details about the transition.
	Message string `json:"message,omitempty"`
}

// EndpointStatus is the status of a condition.
type EndpointStatus string

// LastOperationType is a string alias.
type LastOperationType string

const (
	// LastOperationTypeCreate indicates a 'create' operation.
	LastOperationTypeCreate LastOperationType = "Create"
	// LastOperationTypeReconcile indicates a 'reconcile' operation.
	LastOperationTypeReconcile LastOperationType = "Reconcile"
	// LastOperationTypeDelete indicates a 'delete' operation.
	LastOperationTypeDelete LastOperationType = "Delete"
)

// LastOperationState is a string alias.
type LastOperationState string

const (
	// LastOperationStateProcessing indicates that an operation is ongoing.
	LastOperationStateProcessing LastOperationState = "Processing"
	// LastOperationStateSucceeded indicates that an operation has completed successfully.
	LastOperationStateSucceeded LastOperationState = "Succeeded"
	// LastOperationStateError indicates that an operation is completed with errors and will be retried.
	LastOperationStateError LastOperationState = "Error"
	// LastOperationStateFailed indicates that an operation is completed with errors and won't be retried.
	LastOperationStateFailed LastOperationState = "Failed"
	// LastOperationStatePending indicates that an operation cannot be done now, but will be tried in future.
	LastOperationStatePending LastOperationState = "Pending"
	// LastOperationStateAborted indicates that an operation has been aborted.
	LastOperationStateAborted LastOperationState = "Aborted"
)

// LastOperation indicates the type and the state of the last operation, along with a description
// message and a progress indicator.
type LastOperation struct {
	// A human readable message indicating details about the last operation.
	Description string `json:"description,omitempty"`
	// Last time the operation state transitioned from one to another.
	LastUpdateTime metav1.Time `json:"lastUpdateTime,omitempty"`
	// The progress in percentage (0-100) of the last operation.
	Progress int `json:"progress,omitempty"`
	// Status of the last operation, one of Aborted, Processing, Succeeded, Error, Failed.
	State LastOperationState `json:"state,omitempty"`
	// Type of the last operation, one of Create, Reconcile, Delete.
	Type LastOperationType `json:"type,omitempty"`
}

// Status defines the observed state of Etcd
type Status struct {
	Etcd            CrossVersionObjectReference `json:"etcd,omitempty"`
	Conditions      []Condition                 `json:"conditions,omitempty"`
	CurrentReplicas int32                       `json:"currentReplicas,omitempty"`
	Endpoints       []corev1.Endpoints          `json:"endpoints,omitempty"`
	LastError       string                      `json:"lastError,omitempty"`
	Replicas        int32                       `json:"replicas,omitempty"`
	ReadyReplicas   int32                       `json:"readyReplicas,omitempty"`
	Ready           bool                        `json:"ready,omitempty"`
	UpdatedReplicas int32                       `json:"updatedReplicas,omitempty"`
	//LastOperation   LastOperation               `json:"lastOperation,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// Etcd is the Schema for the etcds API
type Etcd struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   Spec   `json:"spec,omitempty"`
	Status Status `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// EtcdList contains a list of Etcd
type EtcdList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Etcd `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Etcd{}, &EtcdList{})
}
