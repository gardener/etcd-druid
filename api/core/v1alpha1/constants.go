// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
)

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
	// GardenerOperationAnnotation is an annotation set by an operator to specify the operation that is desired on an Etcd resource.
	// Deprecated: Please use DruidOperationAnnotation instead.
	GardenerOperationAnnotation = "gardener.cloud/operation"
	// DruidOperationAnnotation is an annotation set by an operator to specify the operation that is desired on an Etcd resource.
	DruidOperationAnnotation = "druid.gardener.cloud/operation"
	// DruidOperationReconcile is the value for the DruidOperationAnnotation key to specify that the desired operation is to reconcile the Etcd Resource.
	// This value will only be effective if etcd-druid is not configured with auto-reconciliation of Etcd resource specification via
	// --enable-etcd-spec-auto-reconcile CLI flag.
	DruidOperationReconcile = "reconcile"
	// DisableEtcdRuntimeComponentCreationAnnotation is an annotation set by an operator to disable the creation and management of
	// runtime components of the etcd cluster such as pods, PVCs, leases, RBAC resources, PDBs, services, etc.
	DisableEtcdRuntimeComponentCreationAnnotation = "druid.gardener.cloud/disable-etcd-runtime-component-creation"
)

// Compaction Job/Pod reasons that are used to set the reason for a pod condition in the status of an Etcd resource.
const (
	// PodFailureReasonPreemptionByScheduler is a reason for a pod failure that indicates that the pod was preempted by the scheduler.
	PodFailureReasonPreemptionByScheduler = v1.PodReasonPreemptionByScheduler
	// PodFailureReasonDeletionByTaintManager is a reason for a pod failure that indicates that the pod was deleted by the taint manager.
	PodFailureReasonDeletionByTaintManager = "DeletionByTaintManager"
	// PodFailureReasonEvictionByEvictionAPI is a reason for a pod failure that indicates that the pod was evicted by the eviction API.
	PodFailureReasonEvictionByEvictionAPI = "EvictionByEvictionAPI"
	// PodFailureReasonTerminationByKubelet is a reason for a pod failure that indicates that the pod was terminated by the kubelet.
	PodFailureReasonTerminationByKubelet = v1.PodReasonTerminationByKubelet
	// PodFailureReasonProcessFailure is a reason for a pod failure that indicates that the pod process failed.
	PodFailureReasonProcessFailure = "ProcessFailure"
	// PodFailureReasonUnknown is a reason for a pod failure that indicates that the reason for the pod failure is unknown.
	PodFailureReasonUnknown = "Unknown"

	// PodSuccessReasonNone is a reason for a pod success that indicates that the pod has not failed.
	PodSuccessReasonNone = "None"

	// JobFailureReasonDeadlineExceeded is a reason for a job failure that indicates that the job has exceeded its deadline.
	JobFailureReasonDeadlineExceeded = batchv1.JobReasonDeadlineExceeded
	// JobFailureReasonBackoffLimitExceeded is a reason for a job failure that indicates that the job has exceeded its backoff limit.
	JobFailureReasonBackoffLimitExceeded = batchv1.JobReasonBackoffLimitExceeded

	// FullSnapshotSuccessReason is the reason for a successful full snapshot.
	FullSnapshotSuccessReason string = "FullSnapshotTakenSuccessfully"
	// FullSnapshotFailureReason is the reason for a failed full snapshot.
	FullSnapshotFailureReason string = "ErrorTriggeringFullSnapshot"
)
