// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package poddisruptionbudget

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	instanceKey = "instance"
	nameKey     = "name"
	appKey      = "app"
)

// Values contains the values necessary for creating PodDisruptionBudget.
type Values struct {
	// Name is the name of the PodDisruptionBudget.
	Name string
	// Labels are the PDB labels.
	Labels map[string]string
	// SelectorLabels are the labels to be used in the PDB spec selector
	SelectorLabels map[string]string
	// Annotations are the annotations to be used in the PDB
	Annotations map[string]string
	// MinAvailable defined the minimum number of pods to be available at any point of time
	MinAvailable int32
	// OwnerReference is the OwnerReference for the PodDisruptionBudget.
	OwnerReference metav1.OwnerReference
}
