// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"context"
	"slices"
	"strconv"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"

	"github.com/go-logr/logr"
	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// LeaseAnnotationKeyPeerURLTLSEnabled is the annotation key present on the member lease.
// If its value is `true` then it indicates that the member is TLS enabled.
// If the annotation is not present or its value is `false` then it indicates that the member is not TLS enabled.
const LeaseAnnotationKeyPeerURLTLSEnabled = "member.etcd.gardener.cloud/tls-enabled"

// IsPeerURLTLSEnabledForMembers checks if TLS has been enabled for all existing members of an etcd cluster identified by etcdName and in the provided namespace.
func IsPeerURLTLSEnabledForMembers(ctx context.Context, cl client.Client, logger logr.Logger, etcd *druidv1alpha1.Etcd) (bool, error) {
	tlsEnabledForAllMembers := true
	leaseObjMetaSlice, err := ListAllMemberLeaseObjectMeta(ctx, cl, etcd)
	if err != nil {
		return false, err
	}
	for _, leaseObjMeta := range leaseObjMetaSlice {
		tlsEnabled, err := parseAndGetTLSEnabledValue(leaseObjMeta, logger)
		if err != nil {
			return false, err
		}
		tlsEnabledForAllMembers = tlsEnabledForAllMembers && tlsEnabled
	}
	return tlsEnabledForAllMembers, nil
}

// ListAllMemberLeaseObjectMeta returns the list of all member leases for the given etcd cluster.
func ListAllMemberLeaseObjectMeta(ctx context.Context, cl client.Client, etcd *druidv1alpha1.Etcd) ([]metav1.PartialObjectMetadata, error) {
	objMetaList := &metav1.PartialObjectMetadataList{}
	objMetaList.SetGroupVersionKind(coordinationv1.SchemeGroupVersion.WithKind("Lease"))
	if err := cl.List(ctx,
		objMetaList,
		client.InNamespace(etcd.Namespace),
	); err != nil {
		return nil, err
	}
	// This OK to do as we do not support downscaling an etcd cluster.
	// If and when we do that by then we should have already stabilised the labels and therefore this code itself will not be there.
	allPossibleMemberNames := druidv1alpha1.GetMemberLeaseNames(etcd.ObjectMeta, etcd.Spec.Replicas)
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
		if tlsEnabledStr, ok := leaseObjMeta.Annotations[LeaseAnnotationKeyPeerURLTLSEnabled]; ok {
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
