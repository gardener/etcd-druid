// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package role

import (
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Values defines the fields used to create a Role for Etcd.
type Values struct {
	// Name is the name of the Role.
	Name string
	// Namespace is the namespace of the Role.
	Namespace string
	// Rules holds all the PolicyRules for this Role
	Rules []rbacv1.PolicyRule
	// OwnerReference is the OwnerReference of the Role.
	OwnerReference metav1.OwnerReference
	// Labels are the labels of the Role.
	Labels map[string]string
}
