// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package etcdmember

import (
	"context"
	"fmt"
	"strings"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"

	"github.com/go-logr/logr"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type readyCheck struct {
	logger                      logr.Logger
	cl                          client.Client
	etcdMemberNotReadyThreshold time.Duration
	etcdMemberUnknownThreshold  time.Duration
}

// TimeNow is the function used by this check to get the current time.
var TimeNow = time.Now

func (r *readyCheck) Check(ctx context.Context, etcd druidv1alpha1.Etcd) []Result {
	var (
		results   []Result
		checkTime = TimeNow().UTC()
	)

	leaseNames := druidv1alpha1.GetMemberLeaseNames(&etcd)
	leases := make([]*coordinationv1.Lease, 0, len(leaseNames))
	for _, leaseName := range leaseNames {
		lease := &coordinationv1.Lease{}
		if err := r.cl.Get(ctx, client.ObjectKey{Namespace: etcd.Namespace, Name: leaseName}, lease); err != nil {
			if !apierrors.IsNotFound(err) {
				r.logger.Error(err, "failed to get lease", "name", leaseName)
			}
			// If latest Etcd spec has been reconciled, then all expected leases should have been created by now.
			// An error is logged for such not-found member leases.
			if etcd.Status.ObservedGeneration != nil && *etcd.Status.ObservedGeneration == etcd.Generation {
				r.logger.Error(fmt.Errorf("lease not found"), "lease not found", "name", leaseName)
			}
			// In cases where Etcd.spec.replicas has increased, but the latest Etcd spec has not been reconciled by druid,
			// the leases for new etcd members may not have been created yet. Such not-found member leases are ignored.
			continue
		}
		leases = append(leases, lease)
	}

	for _, lease := range leases {
		var id, role, err = extractMemberIdAndRole(lease.Spec.HolderIdentity)
		if err != nil {
			r.logger.Error(err, "failed to extract member ID and role from member lease's holder identity", "holderIdentity", lease.Spec.HolderIdentity)
			continue
		}
		var res = &result{
			id:   id,
			name: lease.Name,
			role: role,
		}

		// Check if member is in bootstrapping phase
		// Members are supposed to be added to the members array only if they have joined the cluster (== RenewTime is set).
		// This behavior is expected by the `Ready` condition and it will become imprecise if members are added here too early.
		renew := lease.Spec.RenewTime
		if renew == nil {
			r.logger.V(4).Info("Member hasn't acquired lease yet, still in bootstrapping phase", "name", lease.Name)
			continue
		}

		// Check if member state must be considered as not ready
		if renew.Add(r.etcdMemberUnknownThreshold).Add(r.etcdMemberNotReadyThreshold).Before(checkTime) {
			res.status = druidv1alpha1.EtcdMemberStatusNotReady
			res.reason = "UnknownGracePeriodExceeded"
			results = append(results, res)
			continue
		}

		// Check if member state must be considered as unknown
		if renew.Add(r.etcdMemberUnknownThreshold).Before(checkTime) {
			// If pod is not running or cannot be found then we deduce that the status is NotReady.
			ready, err := r.checkContainersAreReady(ctx, lease.Namespace, lease.Name)
			if (err == nil && !ready) || apierrors.IsNotFound(err) {
				res.status = druidv1alpha1.EtcdMemberStatusNotReady
				res.reason = "ContainersNotReady"
				results = append(results, res)
				continue
			}

			res.status = druidv1alpha1.EtcdMemberStatusUnknown
			res.reason = "LeaseExpired"
			results = append(results, res)
			continue
		}

		res.status = druidv1alpha1.EtcdMemberStatusReady
		res.reason = "LeaseSucceeded"
		results = append(results, res)
	}

	return results
}

const memberLeaseHolderIdentitySeparator = ":"

// ReadyCheck returns a check for the "Ready" condition.
func ReadyCheck(cl client.Client, logger logr.Logger, etcdMemberNotReadyThreshold, etcdMemberUnknownThreshold time.Duration) Checker {
	return &readyCheck{
		logger:                      logger,
		cl:                          cl,
		etcdMemberNotReadyThreshold: etcdMemberNotReadyThreshold,
		etcdMemberUnknownThreshold:  etcdMemberUnknownThreshold,
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

// extractMemberIdAndRole extracts the member ID and role from the given member lease's holder identity.
// The expected formats of the holder identity are:
//   - "<member-id>:<role>": from `etcd-backup-restore` versions <= v0.39.0
//   - "<member-id>:<cluster-id>:<role>": from `etcd-backup-restore` versions > v0.39.0
func extractMemberIdAndRole(holderIdentity *string) (*string, *druidv1alpha1.EtcdRole, error) {
	if holderIdentity == nil {
		return nil, nil, fmt.Errorf("lease holder identity is nil")
	}
	splits := strings.Split(*holderIdentity, memberLeaseHolderIdentitySeparator)

	switch len(splits) {
	case 2:
		// "<member-id>:<role>"
		memberId := splits[0]
		role := asEtcdRole(splits[1])
		return &memberId, role, nil
	case 3:
		// "<member-id>:<cluster-id>:<role>"
		memberId := splits[0]
		role := asEtcdRole(splits[2])
		return &memberId, role, nil
	default:
		return nil, nil, fmt.Errorf("unexpected format of lease holder identity: %s", *holderIdentity)
	}
}

func (r *readyCheck) checkContainersAreReady(ctx context.Context, namespace string, name string) (bool, error) {
	pod := &corev1.Pod{}
	if err := r.cl.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, pod); err != nil {
		return false, err
	}

	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.ContainersReady {
			return cond.Status == corev1.ConditionTrue, nil
		}
	}

	return false, nil
}
