// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package serviceaccount

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// Values defines the fields used to create a ServiceAccount for Etcd.
type Values struct {
	// Name is the name of the ServiceAccount.
	Name string
	// Namespace is the namespace of the ServiceAccount.
	Namespace string
	// OwnerReference is the OwnerReference for the ServiceAccount.
	OwnerReference metav1.OwnerReference
	// Labels are the labels to apply to the ServiceAccount.
	Labels map[string]string
	// DisableAutomount defines the AutomountServiceAccountToken of the ServiceAccount.
	DisableAutomount bool
}
