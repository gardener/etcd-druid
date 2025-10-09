// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package condition

import (
	"context"
	"fmt"
	"strings"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"

	coordinationv1 "k8s.io/api/coordination/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type readyCheck struct {
	client client.Client
}

const holderIdentitySeparator = ":"

func (r *readyCheck) Check(ctx context.Context, etcd druidv1alpha1.Etcd) Result {

	// TODO: remove this case as soon as leases are completely supported by etcd-backup-restore
	if len(etcd.Status.Members) == 0 {
		return &result{
			conType: druidv1alpha1.ConditionTypeReady,
			status:  druidv1alpha1.ConditionUnknown,
			reason:  "NoMembersInStatus",
			message: "Cannot determine readiness since status has no members",
		}
	}

	var (
		size         = len(etcd.Status.Members)
		quorum       = size/2 + 1
		readyMembers = 0
	)

	for _, member := range etcd.Status.Members {
		if member.Status == druidv1alpha1.EtcdMemberStatusNotReady {
			continue
		}
		readyMembers++
	}

	if readyMembers < quorum {
		return &result{
			conType: druidv1alpha1.ConditionTypeReady,
			status:  druidv1alpha1.ConditionFalse,
			reason:  "QuorumLost",
			message: "The majority of ETCD members is not ready",
		}
	}

	// Look for split-brain scenario.
	leaseNames := druidv1alpha1.GetMemberLeaseNames(etcd.ObjectMeta, etcd.Spec.Replicas)
	leases := make([]*coordinationv1.Lease, 0, len(leaseNames))
	for _, leaseName := range leaseNames {
		lease := &coordinationv1.Lease{}
		if err := r.client.Get(ctx, client.ObjectKey{Namespace: etcd.Namespace, Name: leaseName}, lease); err != nil {
			if client.IgnoreNotFound(err) == nil {
				// Lease has not been created yet, so we skip the split-brain check for now.
				continue
			} else {
				return &result{
					conType: druidv1alpha1.ConditionTypeReady,
					status:  druidv1alpha1.ConditionUnknown,
					reason:  "UnableToFetchLease",
					message: fmt.Sprintf("Unable to fetch Lease %s/%s: %s", etcd.Namespace, leaseName, err.Error()),
				}
			}
		}
		leases = append(leases, lease)
	}

	if len(leases) == size && readyMembers == size {
		// All leases are present, so we can check for split-brain scenario.
		// All members being ready is a prerequisite for split-brain to occur.
		clusterIDs := make([]string, 0)
		roles := make([]druidv1alpha1.EtcdRole, 0)
		for _, lease := range leases {
			clusterID, role, err := extractClusterIdAndRole(lease.Spec.HolderIdentity)
			if err != nil {
				return &result{
					conType: druidv1alpha1.ConditionTypeReady,
					status:  druidv1alpha1.ConditionUnknown,
					reason:  "UnableToParseLeaseHolderIdentity",
					message: err.Error(),
				}
			}
			if clusterID != nil {
				clusterIDs = append(clusterIDs, *clusterID)
			}
			if role != nil {
				roles = append(roles, *role)
			}
		}

		// ClusterIDs are not populated by `etcd-backup-restore` versions less than `v0.39.0`.
		// So, druid does not support checking for split-brain in such cases.
		if size > 0 && len(clusterIDs) == size && len(roles) == size {
			firstClusterID := clusterIDs[0]
			for _, clusterID := range clusterIDs[1:] {
				if clusterID != firstClusterID {
					return &result{
						conType: druidv1alpha1.ConditionTypeReady,
						status:  druidv1alpha1.ConditionFalse,
						reason:  "SplitBrainDetected",
						message: "Split-brain detected",
					}
				}
			}
		}
	}

	return &result{
		conType: druidv1alpha1.ConditionTypeReady,
		status:  druidv1alpha1.ConditionTrue,
		reason:  "Quorate",
		message: "The majority of ETCD members is ready",
	}
}

// ReadyCheck returns a check for the "Ready" condition.
func ReadyCheck(client client.Client) Checker {
	return &readyCheck{
		client: client,
	}
}

func asEtcdRole(roleStr string) *druidv1alpha1.EtcdRole {
	switch druidv1alpha1.EtcdRole(roleStr) {
	case druidv1alpha1.EtcdRoleLeader:
		role := druidv1alpha1.EtcdRoleLeader
		return &role
	case druidv1alpha1.EtcdRoleMember:
		role := druidv1alpha1.EtcdRoleMember
		return &role
	default:
		return nil
	}
}

// extractClusterIdAndRole extracts the cluster ID and role from the given lease holder identity.
// The expected formats of the holder identity are:
//   - "<member-id>:<role>": from `etcd-backup-restore` versions <= v0.39.0
//   - "<member-id>:<cluster-id>:<role>": from `etcd-backup-restore` versions > v0.39.0
func extractClusterIdAndRole(holderIdentity *string) (*string, *druidv1alpha1.EtcdRole, error) {
	splits := strings.Split(*holderIdentity, holderIdentitySeparator)

	switch len(splits) {
	case 2:
		// "<member-id>:<role>"
		role := asEtcdRole(splits[1])
		return nil, role, nil
	case 3:
		// "<member-id>:<cluster-id>:<role>"
		clusterId := splits[1]
		role := asEtcdRole(splits[2])
		return &clusterId, role, nil
	default:
		return nil, nil, fmt.Errorf("unexpected format of lease holder identity: %s", *holderIdentity)
	}
}
