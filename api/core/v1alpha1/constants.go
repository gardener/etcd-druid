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
	// SuspendEtcdSpecReconcileAnnotation is an annotation set by an operator to temporarily suspend any etcd spec reconciliation.
	SuspendEtcdSpecReconcileAnnotation = "druid.gardener.cloud/suspend-etcd-spec-reconcile"
	// DisableEtcdComponentProtectionAnnotation is an annotation set by an operator to disable protection of components created for
	// an etcd cluster and managed by etcd-druid.
	DisableEtcdComponentProtectionAnnotation = "druid.gardener.cloud/disable-etcd-component-protection"
	// DisableEtcdMemberStatusReportingAnnotation is an annotation set by an operator to disable
	// coordination.k8s.io/v1.Lease renewals for etcd members and snapshots.
	DisableEtcdMemberStatusReportingAnnotation = "druid.gardener.cloud/disable-member-status-reporting"
	// GardenerOperationAnnotation is an annotation set by an operator to specify the operation that is desired on an Etcd resource.
	// Deprecated: Please use DruidOperationAnnotation instead.
	GardenerOperationAnnotation = "gardener.cloud/operation"
	// DruidOperationAnnotation is an annotation set by an operator to specify the operation that is desired on an Etcd resource.
	DruidOperationAnnotation = "druid.gardener.cloud/operation"
	// DruidOperationReconcile is the value for the DruidOperationAnnotation key to specify that the desired operation is to reconcile the Etcd Resource.
	// This value will only be effective if etcd-druid is not configured with auto-reconciliation of Etcd resource specification via
	// --enable-etcd-spec-auto-reconcile CLI flag.
	DruidOperationReconcile = "reconcile"
)
