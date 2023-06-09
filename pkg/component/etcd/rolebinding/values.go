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

// Values defines the fields used to create a RoleBinding for Etcd.
type Values struct {
	// Name is the name of the RoleBinding.
	Name string
	// Namespace is the namespace of the RoleBinding.
	Namespace string
	// RoleName is the role name of the RoleBinding. It is assumed that the role exists in the namespace where the etcd custom resource is created.
	RoleName string
	// ServiceAccountName is the service account subject name for the RoleBinding.
	// It is assumed that the ServiceAccount exists in the namespace where the etcd custom resource is created.
	ServiceAccountName string
	// OwnerReference is the OwnerReference for the RoleBinding.
	OwnerReference metav1.OwnerReference
	// Labels are the labels of the RoleBinding.
	Labels map[string]string
}
