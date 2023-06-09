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
