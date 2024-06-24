// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

// Common label keys to be placed on all druid-managed resources
const (
	// LabelAppNameKey is a label which sets the name of the resource provisioned for an etcd cluster.
	LabelAppNameKey = "app.kubernetes.io/name"
	// LabelManagedByKey is a key of a label which sets druid as a manager for resources provisioned for an etcd cluster.
	LabelManagedByKey = "app.kubernetes.io/managed-by"
	// LabelManagedByValue is the value for LabelManagedByKey.
	LabelManagedByValue = "etcd-druid"
	// LabelPartOfKey is a key of a label which establishes that a provisioned resource belongs to a parent etcd cluster.
	LabelPartOfKey = "app.kubernetes.io/part-of"
	// LabelComponentKey is a key for a label that sets the component type on resources provisioned for an etcd cluster.
	LabelComponentKey = "app.kubernetes.io/component"
)

// Annotation keys that can be placed on an Etcd custom resource.
const (
	// IgnoreReconciliationAnnotation is an annotation set by an operator in order to stop reconciliation.
	// Deprecated: Please use SuspendEtcdSpecReconcileAnnotation instead
	IgnoreReconciliationAnnotation = "druid.gardener.cloud/ignore-reconciliation"
	// SuspendEtcdSpecReconcileAnnotation is an annotation set by an operator to temporarily suspend any etcd spec reconciliation.
	SuspendEtcdSpecReconcileAnnotation = "druid.gardener.cloud/suspend-etcd-spec-reconcile"
	// DisableEtcdComponentProtectionAnnotation is an annotation set by an operator to disable protection of components created for
	// an etcd cluster and managed by etcd-druid.
	DisableEtcdComponentProtectionAnnotation = "druid.gardener.cloud/disable-etcd-component-protection"
)
