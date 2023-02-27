// Copyright (c) 2023 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package rolebinding

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

type Values struct {
	// Name is the name of the RoleBinding.
	Name string
	// Namespace is the namespace of the RoleBinding.
	Namespace string
	// RoleName is the role name of the RoleBinding.
	RoleName string
	// ServiceAccountName is the service account name of the RoleBinding.
	ServiceAccountName string
	// OwnerReferences are the OwnerReferences of the RoleBinding.
	OwnerReferences []metav1.OwnerReference
	// Labels are the labels of the RoleBinding.
	Labels map[string]string
}
