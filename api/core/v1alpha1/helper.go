// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"crypto/rand"
	"fmt"
	"math/big"

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

// GetClientHostname returns the hostname of the client endpoint for the Etcd cluster. This is the client service hostname when the Etcd members
// are managed by etcd-druid, else it is a random member address from the externally managed member addresses.
func GetClientHostname(etcd *Etcd) string {
	if IsPodManagementEnabled(etcd) {
		return fmt.Sprintf("%s.%s.svc", GetClientServiceName(etcd.ObjectMeta), etcd.ObjectMeta.Namespace)
	} else {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(etcd.Spec.ExternallyManagedMemberAddresses))))
		if err != nil {
			// Fallback to first member address in case of an error
			return etcd.Spec.ExternallyManagedMemberAddresses[0]
		}
		randomIndex := int(n.Int64())
		return etcd.Spec.ExternallyManagedMemberAddresses[randomIndex]
	}
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

// GetMemberNameFromAddress returns the name of the etcd member based on the address.
func GetMemberNameFromAddress(etcdObjMeta metav1.ObjectMeta, memberAddress string) string {
	return fmt.Sprintf("%s-%s", etcdObjMeta.Name, memberAddress)
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
func GetMemberLeaseNames(etcd *Etcd) []string {
	if IsPodManagementEnabled(etcd) {
		replicas := etcd.Spec.Replicas
		podNames := make([]string, replicas)
		for i := range int(replicas) {
			podNames[i] = GetOrdinalPodName(etcd.ObjectMeta, i)
		}
		return podNames
	} else {
		memberAddresses := etcd.Spec.ExternallyManagedMemberAddresses
		memberNames := make([]string, len(memberAddresses))
		for i, memberAddress := range memberAddresses {
			memberNames[i] = GetMemberNameFromAddress(etcd.ObjectMeta, memberAddress)
		}
		return memberNames
	}
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

// IsResourceMarkedForDeletion returns true if the Etcd object is marked for deletion and false otherwise.
func IsResourceMarkedForDeletion(objMeta metav1.ObjectMeta) bool {
	return objMeta.DeletionTimestamp != nil
}

// GetReconcileOperationAnnotationKey returns the reconcile operation annotation key set on an Etcd resource.
// It will return nil if no such annotation is found.
func GetReconcileOperationAnnotationKey(etcdObjMeta metav1.ObjectMeta) *string {
	if _, ok := etcdObjMeta.Annotations[DruidOperationAnnotation]; ok {
		return ptr.To(DruidOperationAnnotation)
	}
	if _, ok := etcdObjMeta.Annotations[GardenerOperationAnnotation]; ok {
		return ptr.To(GardenerOperationAnnotation)
	}
	return nil
}

// HasReconcileOperationAnnotation checks if an Etcd resource has been annotated with an operation annotation with its value set to reconcile.
func HasReconcileOperationAnnotation(etcdObjMeta metav1.ObjectMeta) bool {
	return etcdObjMeta.Annotations[DruidOperationAnnotation] == DruidOperationReconcile ||
		etcdObjMeta.Annotations[GardenerOperationAnnotation] == DruidOperationReconcile
}

// RemoveOperationAnnotation removes any operation annotation from the Etcd.ObjectMetadata.
func RemoveOperationAnnotation(etcdObjMeta metav1.ObjectMeta) {
	delete(etcdObjMeta.Annotations, DruidOperationAnnotation)
	delete(etcdObjMeta.Annotations, GardenerOperationAnnotation)
}

// IsPodManagementEnabled checks if the management of pods is handled by etcd-druid for an Etcd resource.
func IsPodManagementEnabled(etcd *Etcd) bool {
	return len(etcd.Spec.ExternallyManagedMemberAddresses) == 0
}
