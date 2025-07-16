// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TaskState represents the current state of an EtcdOpsTask.
// +kubebuilder:validation:Enum=Pending;InProgress;Succeeded;Failed;Rejected
//
// Transitions (irreversible):
//
//	Pending  → InProgress → Succeeded
//	   ↘                 ↘ Failed | Rejected
//	    └────────────────→ Rejected
type TaskState string

const (
	// TaskStatePending indicates that the task has been accepted but not yet acted upon.
	TaskStatePending TaskState = "Pending"
	// TaskStateInProgress indicates that the task is currently being executed.
	TaskStateInProgress TaskState = "InProgress"
	// TaskStateSucceeded indicates that the task has completed successfully.
	TaskStateSucceeded TaskState = "Succeeded"
	// TaskStateFailed indicates that the task has failed.
	TaskStateFailed TaskState = "Failed"
	// TaskStateRejected indicates that the task was rejected because it was invalid or could not be processed.
	TaskStateRejected TaskState = "Rejected"
)

// OperationPhase represents the reconciliation phase of the operation.
// +kubebuilder:validation:Enum=Admit;Running;Cleanup
type OperationPhase string

const (
	// OperationPhaseAdmit indicates that the task is in the admission phase.
	OperationPhaseAdmit OperationPhase = "Admit"
	// OperationPhaseRunning indicates that the main logic of the task is being executed.
	OperationPhaseRunning OperationPhase = "Running"
	// OperationPhaseCleanup indicates that the task is in the cleanup phase.
	OperationPhaseCleanup OperationPhase = "Cleanup"
)

// OperationState is the state of each operation phase.
// +kubebuilder:validation:Enum=InProgress;Completed;Failed
type OperationState string

const (
	// OperationStateInProgress indicates that the operation is currently in progress.
	OperationStateInProgress OperationState = "InProgress"
	// OperationStateCompleted indicates that the operation has completed.
	OperationStateCompleted OperationState = "Completed"
	// OperationStateFailed indicates that the operation has failed.
	OperationStateFailed OperationState = "Failed"
)

////////////////////////////////////////////////////////////////////////////////
// EtcdOpsTask
////////////////////////////////////////////////////////////////////////////////

// +kubebuilder:storageversion
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=eot,scope=Namespaced
// +kubebuilder:printcolumn:name="State",type=string,JSONPath=`.status.state`
// +kubebuilder:printcolumn:name="Etcd",type=string,JSONPath=`.spec.etcdRef.name`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +kubebuilder:printcolumn:name="Operation State",type=string,JSONPath=`.status.lastOperation.state`,priority=1
// +kubebuilder:printcolumn:name="TTL",type=integer,JSONPath=`.spec.ttlSecondsAfterFinished`,priority=1

// EtcdOpsTask represents a task to perform operations on an Etcd cluster.
// It defines the desired configuration in Spec and tracks the observed state in Status.
// The controller is responsible for executing the task and updating the status accordingly.
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
	// +kubebuilder:default:=3600
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="ttlSecondsAfterFinished is immutable"
	TTLSecondsAfterFinished *int32 `json:"ttlSecondsAfterFinished,omitempty"`

	// EtcdRef references the Etcd resource that this task will operate on.
	// +optional
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="etcdRef is immutable"
	EtcdRef *EtcdReference `json:"etcdRef"`
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

// EtcdReference holds a reference to an Etcd resource.
type EtcdReference struct {
	// Name is the name of the Etcd custom resource.
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace is the namespace of the Etcd custom resource.
	// If unset, it defaults to the namespace of the EtcdOpsTask.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

////////////////////////////////////////////////////////////////////////////////
// Status
////////////////////////////////////////////////////////////////////////////////

// EtcdOpsTaskStatus defines the observed state of an EtcdOpsTask.
type EtcdOpsTaskStatus struct {
	// State is the overall state of the task.
	// The controller initializes this field when processing the task.
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
	LastErrors []EtcdOpsTaskLastError `json:"lastErrors,omitempty"`

	// LastOperation tracks the fine-grained progress of the task's execution.
	// The controller initializes this field when processing the task.
	// +optional
	LastOperation *EtcdOpsLastOperation `json:"lastOperation,omitempty"`
}

// EtcdOpsTaskLastError represents a single error observed during task execution.
type EtcdOpsTaskLastError struct {
	// Code is an error code that uniquely identifies an error.
	// +kubebuilder:validation:Required
	Code ErrorCode `json:"code"`

	// Description is a human-readable description of the error.
	// +kubebuilder:validation:Required
	Description string `json:"description"`

	// ObservedAt is the timestamp at which the error was observed.
	// +kubebuilder:validation:Required
	ObservedAt metav1.Time `json:"observedAt"`
}

// EtcdOpsLastOperation provides details about the last running operation of the task.
type EtcdOpsLastOperation struct {
	// State is the state of this operation.
	// +kubebuilder:validation:Required
	State OperationState `json:"state"`

	// Phase is the controller reconciliation phase in which the operation is currently running.
	// +kubebuilder:validation:Required
	Phase OperationPhase `json:"phase"`

	// LastTransitionTime is the timestamp of the most recent change to .state of each phase.
	// +optional
	LastTransitionTime *metav1.Time `json:"lastTransitionTime,omitempty"`

	// Description is a human-readable description of the current operation step.
	// +kubebuilder:validation:Required
	Description string `json:"description"`
}

////////////////////////////////////////////////////////////////////////////////
// On-Demand Snapshot Config
////////////////////////////////////////////////////////////////////////////////

// OnDemandSnapshotType defines the type of an on-demand snapshot.
// +kubebuilder:validation:Enum=full;delta
type OnDemandSnapshotType string

const (
	// OnDemandSnapshotTypeFull indicates a full snapshot, capturing the entire etcd database state.
	OnDemandSnapshotTypeFull OnDemandSnapshotType = "full"
	// OnDemandSnapshotTypeDelta indicates a delta snapshot, capturing only changes since the last snapshot.
	OnDemandSnapshotTypeDelta OnDemandSnapshotType = "delta"
)

// OnDemandSnapshotConfig defines the configuration for an on-demand snapshot task.
// +kubebuilder:validation:XValidation:rule="self.type == 'delta' ? !has(self.isFinal) : true",message="isFinal must be false (or omitted) when type is 'delta'"
type OnDemandSnapshotConfig struct {
	// Type specifies whether the snapshot is a 'full' or 'delta' snapshot.
	// Use 'full' for a complete backup of the etcd database, or 'delta' for incremental changes since the last snapshot.
	// +kubebuilder:validation:Required
	Type OnDemandSnapshotType `json:"type"`

	// IsFinal indicates whether this is the final snapshot for the etcd cluster.
	// +optional
	IsFinal *bool `json:"isFinal,omitempty"`

	// TimeoutSeconds is the timeout for the snapshot operation.
	// Defaults to 60 seconds. Must be at least 1.
	// +optional
	// +kubebuilder:default=60
	// +kubebuilder:validation:Minimum=1
	TimeoutSeconds *int32 `json:"timeoutSeconds,omitempty"`
}

// IsCompleted returns true if the task is completed.
func (t *EtcdOpsTask) IsCompleted() bool {
	if t.Status.State == nil {
		return false
	}
	return *t.Status.State == TaskStateSucceeded || *t.Status.State == TaskStateFailed || *t.Status.State == TaskStateRejected
}

// IsMarkedForDeletion returns true if the deletion timestamp is set.
func (t *EtcdOpsTask) IsMarkedForDeletion() bool {
	return t.ObjectMeta.DeletionTimestamp != nil
}

// HasTTLExpired returns true if the TTL after finished has expired.
func (t *EtcdOpsTask) HasTTLExpired() bool {
	return t.GetTimeToExpiry() <= 0
}

// GetTTL returns the TTL duration for the task as set in the spec.
func (t *EtcdOpsTask) GetTTL() time.Duration {
	return time.Duration(*t.Spec.TTLSecondsAfterFinished) * time.Second
}

// GetTimeToExpiry returns the remaining duration until the task's TTL expires.
// If the task is not completed, it returns zero.
// If LastTransitionTime is nil, uses StartedAt; if that is also nil, uses CreationTimestamp.
func (t *EtcdOpsTask) GetTimeToExpiry() time.Duration {
	var baseTime time.Time
	switch {
	case t.Status.LastTransitionTime != nil:
		baseTime = t.Status.LastTransitionTime.Time
	case t.Status.StartedAt != nil:
		baseTime = t.Status.StartedAt.Time
	default:
		// Fallback to CreationTimestamp if both LastTransitionTime and StartedAt are not set.
		// This covers cases where the task transistioned to rejected state
		// 	- has not yet transitioned to in-progress, since admit failed.
		// 	- unsupported task type.
		// Ensures a valid base time for TTL expiry calculation in all lifecycle states.
		baseTime = t.ObjectMeta.CreationTimestamp.Time
	}
	expiry := baseTime.Add(t.GetTTL())
	remaining := expiry.Sub(time.Now().UTC())
	if remaining < 0 {
		return 0
	}
	return remaining
}
