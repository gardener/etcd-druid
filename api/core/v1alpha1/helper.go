// SPDX-FileCopyrightText: Contributors to the Gardener project
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
// are managed by etcd-druid, else it is a randomly selected member address out of the externally managed member addresses.
func GetClientHostname(etcd *Etcd) string {
	if ArePodsManagedByEtcdDruid(etcd) {
		return fmt.Sprintf("%s.%s.svc", GetClientServiceName(etcd.ObjectMeta), etcd.Namespace)
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

// GetMemberName returns the etcd cluster member name for a given member-prefix(optional) and pod-name.
// If memberNamePrefix is non-nil and non-empty, the member name would be "<prefix>-<podName>".
// Otherwise, the member name would be the name of the backing `Pod`.
func GetMemberName(memberNamePrefix *string, podName string) string {
	if prefix := ptr.Deref(memberNamePrefix, ""); prefix != "" {
		return fmt.Sprintf("%s-%s", prefix, podName)
	}
	return podName
}

// GetMemberNameFromAddress returns the name of the etcd member based on the address.
func GetMemberNameFromAddress(etcd *Etcd, memberAddress string) string {
	return GetMemberName(etcd.Spec.MemberNamePrefix, fmt.Sprintf("%s-%s", etcd.Name, memberAddress))
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
	if ArePodsManagedByEtcdDruid(etcd) {
		leaseNames := make([]string, etcd.Spec.Replicas)
		for i := range int(etcd.Spec.Replicas) {
			podName := GetOrdinalPodName(etcd.ObjectMeta, i)
			leaseNames[i] = GetMemberName(etcd.Spec.MemberNamePrefix, podName)
		}
		return leaseNames
	} else {
		memberAddresses := etcd.Spec.ExternallyManagedMemberAddresses
		leaseNames := make([]string, len(memberAddresses))
		for i, memberAddress := range memberAddresses {
			leaseNames[i] = GetMemberNameFromAddress(etcd, memberAddress)
		}
		return leaseNames
	}
}

// GetBootstrapMemberNames returns the set of member names declared in
// spec.etcd.bootstrapWithExistingCluster.members (empty when unset).
func GetBootstrapMemberNames(etcd *Etcd) map[string]bool {
	names := map[string]bool{}
	if etcd.Spec.Etcd.BootstrapWithExistingCluster == nil {
		return names
	}
	for _, m := range etcd.Spec.Etcd.BootstrapWithExistingCluster.Members {
		names[m.Name] = true
	}
	return names
}

// GetBootstrapMemberNamesToDecommission returns the names of the members recorded as
// joined in status.bootstrapWithExistingCluster.members that are no longer
// present in spec.etcd.bootstrapWithExistingCluster.members. When the spec field
// is unset, all joined members are returned (removing them decommissions the
// source cluster). It returns nil when there is nothing to remove (no joined
// members recorded, or every joined member is still present in spec).
func GetBootstrapMemberNamesToDecommission(etcd *Etcd) []string {
	statusBootstrap := etcd.Status.BootstrapWithExistingCluster
	if statusBootstrap == nil || len(statusBootstrap.Members) == 0 {
		return nil
	}

	specNames := GetBootstrapMemberNames(etcd)
	var names []string
	for _, joined := range statusBootstrap.Members {
		if !specNames[joined.Name] {
			names = append(names, joined.Name)
		}
	}
	return names
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

// GetClientPort returns the etcd client port, defaulting to 2379 when unset.
func GetClientPort(etcd *Etcd) int32 {
	return ptr.Deref(etcd.Spec.Etcd.ClientPort, 2379)
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

// ArePodsManagedByEtcdDruid checks if the management of pods is handled by etcd-druid for an Etcd resource.
func ArePodsManagedByEtcdDruid(etcd *Etcd) bool {
	return len(etcd.Spec.ExternallyManagedMemberAddresses) == 0
}

// GetCondition returns the condition with the given type from the Etcd status,
// or nil when no such condition is present.
func GetCondition(etcd *Etcd, condType ConditionType) *Condition {
	for i := range etcd.Status.Conditions {
		if etcd.Status.Conditions[i].Type == condType {
			return &etcd.Status.Conditions[i]
		}
	}
	return nil
}

// IsScaleInInProgress reports whether a scale-in operation is currently recorded
// in the Etcd status (ScaleOperationComplete=False/ScalingIn).
func IsScaleInInProgress(etcd *Etcd) bool {
	cond := GetCondition(etcd, ConditionTypeScaleOperationComplete)
	return cond != nil && cond.Status == ConditionFalse && cond.Reason == ScaleOperationReasonScalingIn
}
