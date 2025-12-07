// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"time"

	druidapicommon "github.com/gardener/etcd-druid/api/common"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// TaskState represents the current state of an EtcdOpsTask.
// +kubebuilder:validation:Enum=Pending;InProgress;Succeeded;Failed;Rejected
//
// Transitions (irreversible):
//
//	Pending  → InProgress → Succeeded
//	   ↘                 ↘ Failed
//	    └────────────────→ Rejected
type TaskState string

const (
	// TaskStatePending indicates that the task has been accepted but not yet acted upon.
	TaskStatePending TaskState = "Pending"
	// TaskStateInProgress indicates that the task is currently being executed.
	TaskStateInProgress TaskState = "InProgress"
	// TaskStateSucceeded indicates that the task has been completed successfully.
	TaskStateSucceeded TaskState = "Succeeded"
	// TaskStateFailed indicates that the task has failed.
	TaskStateFailed TaskState = "Failed"
	// TaskStateRejected indicates that the task has been rejected as it failed to fulfill required preconditions.
	TaskStateRejected TaskState = "Rejected"
)

const (
	// LastOperationTypeAdmit indicates that the task is in the admission phase.
	LastOperationTypeAdmit druidapicommon.LastOperationType = "Admit"
	// LastOperationTypeExecution indicates that the task has successfully passed admission criteria and is currently being executed.
	LastOperationTypeExecution druidapicommon.LastOperationType = "Execution"
	// LastOperationTypeCleanup indicates that the cleanup of the task resources has begun.
	// This will happen when the task has completed and its TTL has expired.
	LastOperationTypeCleanup druidapicommon.LastOperationType = "Cleanup"
)

const (
	// LastOperationStateInProgress indicates that the operation is currently in progress.
	LastOperationStateInProgress druidapicommon.LastOperationState = "InProgress"
	// LastOperationStateCompleted indicates that the operation has completed.
	LastOperationStateCompleted druidapicommon.LastOperationState = "Completed"
	// LastOperationStateFailed indicates that the operation has failed.
	LastOperationStateFailed druidapicommon.LastOperationState = "Failed"
)

////////////////////////////////////////////////////////////////////////////////
// EtcdOpsTask
////////////////////////////////////////////////////////////////////////////////

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=eot,scope=Namespaced
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="Etcd",type=string,JSONPath=`.spec.etcdName`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.lastOperation.type`,priority=1
// +kubebuilder:printcolumn:name="TTL",type=integer,JSONPath=`.spec.ttlSecondsAfterFinished`,priority=1

// EtcdOpsTask represents a task to perform operations on an Etcd cluster.
// It defines the desired configuration in Spec and tracks the observed state in Status.
type EtcdOpsTask struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec defines the desired specification of the EtcdOpsTask.
	// +kubebuilder:validation:Required
	Spec EtcdOpsTaskSpec `json:"spec"`

	// Status defines the observed state of the EtcdOpsTask.
	// +optional
	Status EtcdOpsTaskStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true

// EtcdOpsTaskList contains a list of EtcdOpsTask.
type EtcdOpsTaskList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EtcdOpsTask `json:"items"`
}

////////////////////////////////////////////////////////////////////////////////
// Spec
////////////////////////////////////////////////////////////////////////////////

// EtcdOpsTaskSpec defines the desired state of an EtcdOpsTask.
type EtcdOpsTaskSpec struct {
	// Config specifies the configuration for the operation to be performed.
	// Exactly one of the members of EtcdOpsTaskConfig must be set.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="config is immutable"
	Config EtcdOpsTaskConfig `json:"config"`

	// TTLSecondsAfterFinished is the duration in seconds after which a finished task (status.state == Succeeded|Failed|Rejected) will be garbage-collected.
	// +optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default:=3600
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="ttlSecondsAfterFinished is immutable"
	TTLSecondsAfterFinished *int32 `json:"ttlSecondsAfterFinished,omitempty"`

	// EtcdName refers to the name of the Etcd resource that this task will operate on.
	// +optional
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="etcdName is immutable"
	EtcdName *string `json:"etcdName"`
}

// EtcdOpsTaskConfig holds the configuration for the specific operation.
// Exactly one of its members must be set according to the operation to be performed.
// +kubebuilder:validation:MinProperties=1
// +kubebuilder:validation:MaxProperties=1
type EtcdOpsTaskConfig struct {
	// OnDemandSnapshot defines the configuration for an on-demand snapshot task.
	// +optional
	OnDemandSnapshot *OnDemandSnapshotConfig `json:"onDemandSnapshot,omitempty"`
}

////////////////////////////////////////////////////////////////////////////////
// Status
////////////////////////////////////////////////////////////////////////////////

// EtcdOpsTaskStatus defines the observed state of an EtcdOpsTask.
type EtcdOpsTaskStatus struct {
	// State represents the current state of the task.
	// +optional
	State *TaskState `json:"state,omitempty"`

	// LastTransitionTime is the last time the state transitioned from one value to another.
	// +optional
	LastTransitionTime *metav1.Time `json:"lastTransitionTime,omitempty"`

	// StartedAt is the time at which the task transitioned from Pending to InProgress.
	// +optional
	StartedAt *metav1.Time `json:"startedAt,omitempty"`

	// LastErrors is a list of the most recent errors observed during the task's execution.
	// A maximum of 10 latest errors will be recorded.
	// +optional
	// +kubebuilder:validation:MaxItems=10
	LastErrors []druidapicommon.LastError `json:"lastErrors,omitempty"`

	// LastOperation tracks the fine-grained progress of the task's execution.
	// The controller initializes this field when processing the task.
	// +optional
	LastOperation *druidapicommon.LastOperation `json:"lastOperation,omitempty"`
}

// GetEtcdReference returns the NamespacedName of the etcd object referenced by the task.
func (t *EtcdOpsTask) GetEtcdReference() types.NamespacedName {
	if t.Spec.EtcdName == nil || *t.Spec.EtcdName == "" {
		return types.NamespacedName{}
	}
	return types.NamespacedName{
		Name:      *t.Spec.EtcdName,
		Namespace: t.Namespace,
	}
}

// IsCompleted returns true if the task is completed.
// The etcdopsatask is considered as completed if the task has one of these states: Succeeded, Failed, or Rejected.
func (t *EtcdOpsTask) IsCompleted() bool {
	if t.Status.State == nil {
		return false
	}
	return *t.Status.State == TaskStateSucceeded || *t.Status.State == TaskStateFailed || *t.Status.State == TaskStateRejected
}

// HasTTLExpired returns true if the TTL after completion has expired.
func (t *EtcdOpsTask) HasTTLExpired() bool {
	return t.GetTimeToExpiry() == 0
}

// GetTimeToExpiry returns the remaining duration until the task's TTL expires.
// If the task's TTL has already expired, it returns zero.
func (t *EtcdOpsTask) GetTimeToExpiry() time.Duration {
	now := time.Now().UTC()
	ttl := time.Duration(*t.Spec.TTLSecondsAfterFinished) * time.Second
	expiry := t.Status.LastTransitionTime.Add(ttl)
	if now.Before(expiry) {
		return expiry.Sub(now)
	}
	return 0
}
