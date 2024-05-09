// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"context"
	"slices"
	"strconv"
	"strings"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"
	"github.com/go-logr/logr"
	coordinationv1 "k8s.io/api/coordination/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const peerURLTLSEnabledKey = "member.etcd.gardener.cloud/tls-enabled"

// IsPeerURLTLSEnabledForMembers checks if TLS has been enabled for all existing members of an etcd cluster identified by etcdName and in the provided namespace.
func IsPeerURLTLSEnabledForMembers(ctx context.Context, cl client.Client, logger logr.Logger, namespace, etcdName string, numReplicas int32) (bool, error) {
	leaseList := &coordinationv1.LeaseList{}
	if err := cl.List(ctx, leaseList, client.InNamespace(namespace), client.MatchingLabels(map[string]string{
		druidv1alpha1.LabelComponentKey: common.ComponentNameMemberLease,
		druidv1alpha1.LabelPartOfKey:    etcdName,
		druidv1alpha1.LabelManagedByKey: druidv1alpha1.LabelManagedByValue,
	})); err != nil {
		return false, err
	}
	tlsEnabledForAllMembers := true
	leases := leaseList.DeepCopy().Items
	slices.SortFunc(leases, func(a, b coordinationv1.Lease) int {
		return strings.Compare(a.Name, b.Name)
	})
	for _, lease := range leases[:numReplicas] {
		tlsEnabled, err := parseAndGetTLSEnabledValue(lease, logger)
		if err != nil {
			return false, err
		}
		tlsEnabledForAllMembers = tlsEnabledForAllMembers && tlsEnabled
	}
	return tlsEnabledForAllMembers, nil
}

func parseAndGetTLSEnabledValue(lease coordinationv1.Lease, logger logr.Logger) (bool, error) {
	if lease.Annotations != nil {
		if tlsEnabledStr, ok := lease.Annotations[peerURLTLSEnabledKey]; ok {
			tlsEnabled, err := strconv.ParseBool(tlsEnabledStr)
			if err != nil {
				logger.Error(err, "tls-enabled value is not a valid boolean", "namespace", lease.Namespace, "leaseName", lease.Name)
				return false, err
			}
			return tlsEnabled, nil
		}
		logger.V(4).Info("tls-enabled annotation not present for lease.", "namespace", lease.Namespace, "leaseName", lease.Name)
	}
	return false, nil
}
