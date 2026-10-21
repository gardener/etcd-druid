// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package ondelete

import (
	"context"
	"fmt"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/health/etcdmember"

	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// memberRole returns the pod's etcd role from the member lease's HolderIdentity,
// or (nil, nil) when the lease is missing (a bootstrapping member). Callers
// treat a nil role as "unknown → order as a follower". The stale-role race
// (lease lag behind actual leadership) is an accepted limitation of DEP-07;
// do not add a lease-freshness check here.
func (r *Reconciler) memberRole(ctx context.Context, etcd *druidv1alpha1.Etcd, pod *corev1.Pod) (*druidv1alpha1.EtcdRole, error) {
	leaseName := druidv1alpha1.GetMemberName(etcd.Spec.MemberNamePrefix, pod.Name)
	lease := &coordinationv1.Lease{}
	if err := r.client.Get(ctx, client.ObjectKey{Namespace: pod.Namespace, Name: leaseName}, lease); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get member lease %s/%s: %w", pod.Namespace, leaseName, err)
	}
	_, role, err := etcdmember.ExtractMemberIdAndRole(lease.Spec.HolderIdentity)
	if err != nil {
		return nil, fmt.Errorf("failed to parse holder identity of member lease %s/%s: %w", pod.Namespace, leaseName, err)
	}
	return role, nil
}

// isLeader is a nil-safe check: a nil role is treated as non-leader.
func isLeader(role *druidv1alpha1.EtcdRole) bool {
	return role != nil && *role == druidv1alpha1.EtcdRoleLeader
}
