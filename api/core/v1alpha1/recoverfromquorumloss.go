// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// RecoverFromQuorumLossConfig defines the configuration for the RecoverFromQuorumLoss task.
// This task automates the recovery of a multi-member etcd cluster from a permanent quorum loss.
type RecoverFromQuorumLossConfig struct {
	// ScaleDownTimeout is the timeout to wait for the StatefulSet to fully scale down to 0 replicas
	// during quorum loss recovery. Defaults to 60 seconds.
	// +optional
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Pattern="^([0-9]+(\\.[0-9]+)?(ns|us|µs|ms|s|m|h))+$"
	ScaleDownTimeout *metav1.Duration `json:"scaleDownTimeout,omitempty"`

	// PodReadyTimeout is the timeout to wait for the single-member etcd pod (ordinal 0) to become
	// ready after the StatefulSet is scaled up to 1 during quorum loss recovery.
	// Defaults to 180 seconds.
	// +optional
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Pattern="^([0-9]+(\\.[0-9]+)?(ns|us|µs|ms|s|m|h))+$"
	PodReadyTimeout *metav1.Duration `json:"podReadyTimeout,omitempty"`
}
