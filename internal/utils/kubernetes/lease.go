// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	"context"
	"slices"
	"strconv"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"

	"github.com/go-logr/logr"
	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// IsPeerURLInSyncForAllMembers checks if the peer URL is in sync for all existing members of an etcd cluster identified by etcdName and in the provided namespace.
func IsPeerURLInSyncForAllMembers(ctx context.Context, cl client.Client, logger logr.Logger, etcd *druidv1alpha1.Etcd, replicas int32) (bool, error) {
	peerURLTLSEnabled := etcd.Spec.Etcd.PeerUrlTLS != nil
	if peerURLTLSEnabled {
		return isPeerURLTLSEnabledForMembers(ctx, cl, logger, etcd, replicas)
	}
	return isPeerURLTLSDisabledForMembers(ctx, cl, logger, etcd, replicas)
}

// isPeerURLTLSEnabledForMembers checks if TLS has been enabled for all existing members of an etcd cluster identified by etcdName and in the provided namespace.
func isPeerURLTLSEnabledForMembers(ctx context.Context, cl client.Client, logger logr.Logger, etcd *druidv1alpha1.Etcd, replicas int32) (bool, error) {
	leaseObjMetaSlice, err := ListAllMemberLeaseObjectMeta(ctx, cl, etcd)
	if err != nil {
		return false, err
	}
	// During a scale-in, replicas (the current StatefulSet size) can be larger than the number
	// of leases, because the leases of removed members are already deleted. Use the smaller of
	// the two so we only check members that still exist and never slice out of range.
	n := min(int(replicas), len(leaseObjMetaSlice))
	tlsEnabledForAllMembers := true
	for _, leaseObjMeta := range leaseObjMetaSlice[:n] {
		tlsEnabled, err := parseAndGetTLSEnabledValue(leaseObjMeta, logger)
		if err != nil {
			return false, err
		}
		tlsEnabledForAllMembers = tlsEnabledForAllMembers && tlsEnabled
	}
	return tlsEnabledForAllMembers, nil
}

// isPeerURLTLSDisabledForMembers checks if TLS has been disabled for all existing members of an etcd cluster identified by etcdName and in the provided namespace.
func isPeerURLTLSDisabledForMembers(ctx context.Context, cl client.Client, logger logr.Logger, etcd *druidv1alpha1.Etcd, replicas int32) (bool, error) {
	leaseObjMetaSlice, err := ListAllMemberLeaseObjectMeta(ctx, cl, etcd)
	if err != nil {
		return false, err
	}
	// During a scale-in, replicas (the current StatefulSet size) can be larger than the number
	// of leases, because the leases of removed members are already deleted. Use the smaller of
	// the two so we only check members that still exist and never slice out of range.
	n := min(int(replicas), len(leaseObjMetaSlice))
	tlsDisabledForAllMembers := true
	for _, leaseObjMeta := range leaseObjMetaSlice[:n] {
		tlsEnabled, err := parseAndGetTLSEnabledValue(leaseObjMeta, logger)
		if err != nil {
			return false, err
		}
		tlsDisabledForAllMembers = tlsDisabledForAllMembers && !tlsEnabled
	}
	return tlsDisabledForAllMembers, nil
}

// ListAllMemberLeaseObjectMeta returns the list of all member leases for the given etcd cluster.
// The lease names are derived from etcd.Spec.Replicas, so a scale-in only ever reports leases
// for the members that are still desired; leases for departing members are intentionally excluded.
func ListAllMemberLeaseObjectMeta(ctx context.Context, cl client.Client, etcd *druidv1alpha1.Etcd) ([]metav1.PartialObjectMetadata, error) {
	objMetaList := &metav1.PartialObjectMetadataList{}
	objMetaList.SetGroupVersionKind(coordinationv1.SchemeGroupVersion.WithKind("Lease"))
	if err := cl.List(ctx,
		objMetaList,
		client.InNamespace(etcd.Namespace),
	); err != nil {
		return nil, err
	}
	allPossibleMemberNames := druidv1alpha1.GetMemberLeaseNames(etcd)
	leasesObjMeta := make([]metav1.PartialObjectMetadata, 0, len(objMetaList.Items))
	for _, lease := range objMetaList.Items {
		if metav1.IsControlledBy(&lease, &etcd.ObjectMeta) && slices.Contains(allPossibleMemberNames, lease.Name) {
			leasesObjMeta = append(leasesObjMeta, lease)
		}
	}
	return leasesObjMeta, nil
}

func parseAndGetTLSEnabledValue(leaseObjMeta metav1.PartialObjectMetadata, logger logr.Logger) (bool, error) {
	if leaseObjMeta.Annotations != nil {
		if tlsEnabledStr, ok := leaseObjMeta.Annotations[common.LeaseAnnotationKeyPeerURLTLSEnabled]; ok {
			tlsEnabled, err := strconv.ParseBool(tlsEnabledStr)
			if err != nil {
				logger.Error(err, "tls-enabled value is not a valid boolean", "namespace", leaseObjMeta.Namespace, "leaseName", leaseObjMeta.Name)
				return false, err
			}
			return tlsEnabled, nil
		}
		logger.V(4).Info("tls-enabled annotation not present for lease.", "namespace", leaseObjMeta.Namespace, "leaseName", leaseObjMeta.Name)
	}
	return false, nil
}
