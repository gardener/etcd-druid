package v1alpha1

// OnDemandSnapshotType defines the type of on-demand snapshot.
// +kubebuilder:validation:Enum=full;delta
type OnDemandSnapshotType string

const (
	// OnDemandSnapshotTypeFull indicates a full snapshot, capturing the entire etcd database state.
	OnDemandSnapshotTypeFull OnDemandSnapshotType = "full"
	// OnDemandSnapshotTypeDelta indicates a delta snapshot, capturing only changes since the last snapshot.
	OnDemandSnapshotTypeDelta OnDemandSnapshotType = "delta"
)

// OnDemandSnapshotConfig defines the configuration for an on-demand snapshot task.
// +kubebuilder:validation:XValidation:rule="self.type == 'delta' ? !has(self.isFinal) || self.isFinal == false : true",message="isFinal must be false (or omitted) when type is 'delta'"
type OnDemandSnapshotConfig struct {
	// Type specifies whether the snapshot is a 'full' or 'delta' snapshot.
	// Use 'full' for a complete backup of the etcd database, or 'delta' for incremental changes since the last snapshot.
	// +kubebuilder:validation:Required
	Type OnDemandSnapshotType `json:"type"`

	// IsFinal indicates whether the snapshot is marked as final. This is subject to change.
	// +optional
	IsFinal *bool `json:"isFinal,omitempty"`

	// TimeoutSeconds is the timeout for the delta snapshot operation.
	// Defaults to 60 seconds.
	// +optional
	// +kubebuilder:default=60
	// +kubebuilder:validation:Minimum=1
	TimeoutSecondsDelta *int32 `json:"timeoutSecondsDelta,omitempty"`

	// TimeoutSecondsFull is the timeout for full snapshot operations.
	// Defaults to 480 seconds (8 minutes).
	// +optional
	// +kubebuilder:default=480
	// +kubebuilder:validation:Minimum=1
	TimeoutSecondsFull *int32 `json:"timeoutSecondsFull,omitempty"`
}
