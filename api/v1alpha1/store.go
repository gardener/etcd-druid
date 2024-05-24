package v1alpha1

import corev1 "k8s.io/api/core/v1"

// StorageProvider defines the type of object store provider for storing backups.
type StorageProvider string

// StoreSpec defines parameters related to ObjectStore persisting backups
type StoreSpec struct {
	// Container is the name of the container the backup is stored at.
	// +optional
	Container *string `json:"container,omitempty"`
	// Prefix is the prefix used for the store.
	// +required
	Prefix string `json:"prefix"`
	// Provider is the name of the backup provider.
	// +optional
	Provider *StorageProvider `json:"provider,omitempty"`
	// SecretRef is the reference to the secret which used to connect to the backup store.
	// +optional
	SecretRef *corev1.SecretReference `json:"secretRef,omitempty"`
}
