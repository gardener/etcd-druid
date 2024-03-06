// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

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
