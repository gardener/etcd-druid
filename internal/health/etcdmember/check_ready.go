// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package etcdmember

import (
	"context"
	"fmt"
	"strings"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	kutil "github.com/gardener/gardener/pkg/utils/kubernetes"
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

	leaseNames := druidv1alpha1.GetMemberLeaseNames(etcd.ObjectMeta, etcd.Spec.Replicas)
	leases := make([]*coordinationv1.Lease, 0, len(leaseNames))
	for _, leaseName := range leaseNames {
		lease := &coordinationv1.Lease{}
		if err := r.cl.Get(ctx, kutil.Key(etcd.Namespace, leaseName), lease); err != nil {
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
		var (
			id, role = separateIdFromRole(lease.Spec.HolderIdentity)
			res      = &result{
				id:   id,
				name: lease.Name,
				role: role,
			}
		)

		// Check if member is in bootstrapping phase
		// Members are supposed to be added to the members array only if they have joined the cluster (== RenewTime is set).
		// This behavior is expected by the `Ready` condition and it will become imprecise if members are added here too early.
		renew := lease.Spec.RenewTime
		if renew == nil {
			r.logger.Info("Member hasn't acquired lease yet, still in bootstrapping phase", "name", lease.Name)
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

const holderIdentitySeparator = ":"

func separateIdFromRole(holderIdentity *string) (*string, *druidv1alpha1.EtcdRole) {
	if holderIdentity == nil {
		return nil, nil
	}
	parts := strings.SplitN(*holderIdentity, holderIdentitySeparator, 2)
	id := &parts[0]
	if len(parts) != 2 {
		return id, nil
	}

	switch druidv1alpha1.EtcdRole(parts[1]) {
	case druidv1alpha1.EtcdRoleLeader:
		role := druidv1alpha1.EtcdRoleLeader
		return id, &role
	case druidv1alpha1.EtcdRoleMember:
		role := druidv1alpha1.EtcdRoleMember
		return id, &role
	default:
		return id, nil
	}
}

func (r *readyCheck) checkContainersAreReady(ctx context.Context, namespace string, name string) (bool, error) {
	pod := &corev1.Pod{}
	if err := r.cl.Get(ctx, kutil.Key(namespace, name), pod); err != nil {
		return false, err
	}

	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.ContainersReady {
			return cond.Status == corev1.ConditionTrue, nil
		}
	}

	return false, nil
}

// ReadyCheck returns a check for the "Ready" condition.
func ReadyCheck(cl client.Client, logger logr.Logger, etcdMemberNotReadyThreshold, etcdMemberUnknownThreshold time.Duration) Checker {
	return &readyCheck{
		logger:                      logger,
		cl:                          cl,
		etcdMemberNotReadyThreshold: etcdMemberNotReadyThreshold,
		etcdMemberUnknownThreshold:  etcdMemberUnknownThreshold,
	}
}
