// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package condition

import (
	"context"
	"fmt"
	"slices"
	"strings"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"

	coordinationv1 "k8s.io/api/coordination/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type clusterIDMismatchCheck struct {
	client client.Client
}

const memberLeaseHolderIdentitySeparator = ":"

func (r *clusterIDMismatchCheck) Check(ctx context.Context, etcd druidv1alpha1.Etcd) Result {
	memberNames := make([]string, 0)
	memberConditionStatuses := make(map[string]druidv1alpha1.EtcdMemberConditionStatus)
	for _, member := range etcd.Status.Members {
		memberNames = append(memberNames, member.Name)
		memberConditionStatuses[member.Name] = member.Status
	}

	leaseNames := druidv1alpha1.GetMemberLeaseNames(etcd.ObjectMeta, etcd.Spec.Replicas)
	memberClusterIDs := make(map[string]string)
	for _, leaseName := range leaseNames {
		lease := &coordinationv1.Lease{}
		if err := r.client.Get(ctx, client.ObjectKey{Namespace: etcd.Namespace, Name: leaseName}, lease); err != nil {
			if client.IgnoreNotFound(err) == nil {
				// Lease has not been created yet.
				continue
			} else {
				return &result{
					conType: druidv1alpha1.ConditionTypeClusterIDMismatch,
					status:  druidv1alpha1.ConditionUnknown,
					reason:  "UnableToFetchLease",
					message: fmt.Sprintf("Unable to fetch Lease %s: %s", leaseName, err.Error()),
				}
			}
		}
		clusterID, err := extractClusterId(lease.Spec.HolderIdentity)
		if err != nil {
			return &result{
				conType: druidv1alpha1.ConditionTypeClusterIDMismatch,
				status:  druidv1alpha1.ConditionUnknown,
				reason:  "UnableToParseLeaseHolderIdentity",
				message: fmt.Sprintf("Unable to parse Lease %s holder identity: %s", leaseName, err.Error()),
			}
		}
		if clusterID != nil {
			memberClusterIDs[leaseName] = *clusterID
		}
	}

	uniqueClusterIDs := make([]string, 0)
	for _, memberName := range memberNames {
		if status, exists := memberConditionStatuses[memberName]; !exists || status == druidv1alpha1.EtcdMemberStatusUnknown {
			// Skip cluster ID mismatch check for members with unknown status.
			continue
		}
		clusterID, clusterIDExists := memberClusterIDs[memberName]
		if !clusterIDExists {
			// Skip cluster ID mismatch check if cluster ID is not available for the member.
			continue
		}
		if !slices.Contains(uniqueClusterIDs, clusterID) {
			uniqueClusterIDs = append(uniqueClusterIDs, clusterID)
		}
	}

	switch len(uniqueClusterIDs) {
	case 0:
		return &result{
			conType: druidv1alpha1.ConditionTypeClusterIDMismatch,
			status:  druidv1alpha1.ConditionUnknown,
			reason:  "NoClusterIDDetected",
			message: "No cluster ID could be detected from known ETCD members",
		}
	case 1:
		return &result{
			conType: druidv1alpha1.ConditionTypeClusterIDMismatch,
			status:  druidv1alpha1.ConditionFalse,
			reason:  "NoClusterIDMismatchDetected",
			message: "No cluster ID mismatch detected among ETCD members",
		}
	default:
		return &result{
			conType: druidv1alpha1.ConditionTypeClusterIDMismatch,
			status:  druidv1alpha1.ConditionTrue,
			reason:  "MultipleClusterIDsDetected",
			message: "Multiple cluster IDs detected among ETCD members",
		}
	}
}

// ClusterIDMismatchCheck returns a check for the "ClusterIDMismatch" condition.
func ClusterIDMismatchCheck(client client.Client) Checker {
	return &clusterIDMismatchCheck{
		client: client,
	}
}

// extractClusterId extracts the cluster ID from the given lease holder identity.
// The expected formats of the holder identity are:
//   - "<member-id>:<role>": from `etcd-backup-restore` versions <= v0.39.0
//   - "<member-id>:<cluster-id>:<role>": from `etcd-backup-restore` versions > v0.39.0
func extractClusterId(holderIdentity *string) (*string, error) {
	if holderIdentity == nil {
		return nil, fmt.Errorf("lease holder identity is nil")
	}
	splits := strings.Split(*holderIdentity, memberLeaseHolderIdentitySeparator)

	switch len(splits) {
	case 2:
		// "<member-id>:<role>"
		// Silently return nil as cluster ID for old format
		return nil, nil
	case 3:
		// "<member-id>:<cluster-id>:<role>"
		clusterId := splits[1]
		return &clusterId, nil
	default:
		return nil, fmt.Errorf("unexpected lease holder identity format: %s", *holderIdentity)
	}
}
