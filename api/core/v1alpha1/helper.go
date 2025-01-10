// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
)

// --------------- Helper functions for Etcd resource names ---------------

// GetPeerServiceName returns the peer service name for the Etcd cluster reachable by members within the Etcd cluster.
func GetPeerServiceName(etcdObjMeta metav1.ObjectMeta) string {
	return fmt.Sprintf("%s-peer", etcdObjMeta.Name)
}

// GetClientServiceName returns the client service name for the Etcd cluster reachable by external clients.
func GetClientServiceName(etcdObjMeta metav1.ObjectMeta) string {
	return fmt.Sprintf("%s-client", etcdObjMeta.Name)
}

// GetServiceAccountName returns the service account name for the Etcd.
func GetServiceAccountName(etcdObjMeta metav1.ObjectMeta) string {
	return etcdObjMeta.Name
}

// GetConfigMapName returns the name of the configmap for the Etcd.
func GetConfigMapName(etcdObjMeta metav1.ObjectMeta) string {
	return fmt.Sprintf("%s-config", etcdObjMeta.Name)
}

// GetCompactionJobName returns the compaction job name for the Etcd.
func GetCompactionJobName(etcdObjMeta metav1.ObjectMeta) string {
	return fmt.Sprintf("%s-compactor", etcdObjMeta.Name)
}

// GetOrdinalPodName returns the Etcd pod name based on the ordinal.
func GetOrdinalPodName(etcdObjMeta metav1.ObjectMeta, ordinal int) string {
	return fmt.Sprintf("%s-%d", etcdObjMeta.Name, ordinal)
}

// GetAllPodNames returns the names of all pods for the Etcd.
func GetAllPodNames(etcdObjMeta metav1.ObjectMeta, replicas int32) []string {
	podNames := make([]string, replicas)
	for i := range int(replicas) {
		podNames[i] = GetOrdinalPodName(etcdObjMeta, i)
	}
	return podNames
}

// GetMemberLeaseNames returns the name of member leases for the Etcd.
func GetMemberLeaseNames(etcdObjMeta metav1.ObjectMeta, replicas int32) []string {
	leaseNames := make([]string, replicas)
	for i := range int(replicas) {
		leaseNames[i] = fmt.Sprintf("%s-%d", etcdObjMeta.Name, i)
	}
	return leaseNames
}

// GetPodDisruptionBudgetName returns the name of the pod disruption budget for the Etcd.
func GetPodDisruptionBudgetName(etcdObjMeta metav1.ObjectMeta) string {
	return etcdObjMeta.Name
}

// GetRoleName returns the role name for the Etcd.
func GetRoleName(etcdObjMeta metav1.ObjectMeta) string {
	return fmt.Sprintf("%s:etcd:%s", SchemeGroupVersion.Group, etcdObjMeta.Name)
}

// GetRoleBindingName returns the role binding name for the Etcd.
func GetRoleBindingName(etcdObjMeta metav1.ObjectMeta) string {
	return fmt.Sprintf("%s:etcd:%s", SchemeGroupVersion.Group, etcdObjMeta.Name)
}

// GetDeltaSnapshotLeaseName returns the name of the delta snapshot lease for the Etcd.
func GetDeltaSnapshotLeaseName(etcdObjMeta metav1.ObjectMeta) string {
	return fmt.Sprintf("%s-delta-snap", etcdObjMeta.Name)
}

// GetFullSnapshotLeaseName returns the name of the full snapshot lease for the Etcd.
func GetFullSnapshotLeaseName(etcdObjMeta metav1.ObjectMeta) string {
	return fmt.Sprintf("%s-full-snap", etcdObjMeta.Name)
}

// GetStatefulSetName returns the name of the StatefulSet for the Etcd.
func GetStatefulSetName(etcdObjMeta metav1.ObjectMeta) string {
	return etcdObjMeta.Name
}

// --------------- Miscellaneous helper functions ---------------

// GetNamespaceName is a convenience function which creates a types.NamespacedName for an Etcd resource.
func GetNamespaceName(etcdObjMeta metav1.ObjectMeta) types.NamespacedName {
	return types.NamespacedName{
		Namespace: etcdObjMeta.Namespace,
		Name:      etcdObjMeta.Name,
	}
}

// GetSuspendEtcdSpecReconcileAnnotationKey gets the annotation key set on an Etcd resource signalling the intent
// to suspend spec reconciliation for this Etcd resource. If no annotation is set then it will return nil.
func GetSuspendEtcdSpecReconcileAnnotationKey(etcdObjMeta metav1.ObjectMeta) *string {
	if metav1.HasAnnotation(etcdObjMeta, SuspendEtcdSpecReconcileAnnotation) {
		return ptr.To(SuspendEtcdSpecReconcileAnnotation)
	}
	return nil
}

// AreManagedResourcesProtected returns false if the Etcd resource has the `druid.gardener.cloud/disable-etcd-component-protection` annotation set,
// else returns true.
func AreManagedResourcesProtected(etcdObjMeta metav1.ObjectMeta) bool {
	return !metav1.HasAnnotation(etcdObjMeta, DisableEtcdComponentProtectionAnnotation)
}

// GetDefaultLabels returns the default labels for etcd.
func GetDefaultLabels(etcdObjMeta metav1.ObjectMeta) map[string]string {
	return map[string]string{
		LabelManagedByKey: LabelManagedByValue,
		LabelPartOfKey:    etcdObjMeta.Name,
	}
}

// GetAsOwnerReference returns an OwnerReference object that represents the current Etcd instance.
func GetAsOwnerReference(etcdObjMeta metav1.ObjectMeta) metav1.OwnerReference {
	return metav1.OwnerReference{
		APIVersion:         SchemeGroupVersion.String(),
		Kind:               "Etcd",
		Name:               etcdObjMeta.Name,
		UID:                etcdObjMeta.UID,
		Controller:         ptr.To(true),
		BlockOwnerDeletion: ptr.To(true),
	}
}

// IsEtcdMarkedForDeletion returns true if the Etcd object is marked for deletion and false otherwise.
func IsEtcdMarkedForDeletion(etcdObjMeta metav1.ObjectMeta) bool {
	return etcdObjMeta.DeletionTimestamp != nil
}
