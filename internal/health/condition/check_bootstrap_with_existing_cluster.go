// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package condition

import (
	"context"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// bootstrapWithExistingCluster is a Checker for the
// BootstrappedWithExistingCluster condition.
type bootstrapWithExistingCluster struct{}

// Check returns the BootstrappedWithExistingCluster condition for the given
// Etcd. It returns nil when the feature is not configured, False during the
// join, and True once the join has completed.
//
// The condition is sticky-True: once etcd.Status.BootstrapWithExistingCluster
// is populated by mutateBootstrapWithExistingClusterStatus (which only fires
// after this Check first returns True), the condition stays True regardless
// of transient member outages. Re-evaluating member health after bootstrap
// would incorrectly regress the condition to BootstrapInProgress.
func (b *bootstrapWithExistingCluster) Check(_ context.Context, etcd druidv1alpha1.Etcd) Result {
	if etcd.Status.BootstrapWithExistingCluster != nil {
		return &result{
			conType: druidv1alpha1.ConditionTypeBootstrappedWithExistingCluster,
			status:  druidv1alpha1.ConditionTrue,
			reason:  "BootstrapSucceeded",
			message: "All members have successfully joined the existing cluster",
		}
	}

	if etcd.Spec.Etcd.BootstrapWithExistingCluster == nil || len(etcd.Spec.Etcd.BootstrapWithExistingCluster.Members) == 0 {
		return nil
	}

	if len(etcd.Status.Members) < int(etcd.Spec.Replicas) {
		return &result{
			conType: druidv1alpha1.ConditionTypeBootstrappedWithExistingCluster,
			status:  druidv1alpha1.ConditionFalse,
			reason:  "BootstrapInProgress",
			message: "Not all members have joined the cluster yet",
		}
	}

	for _, member := range etcd.Status.Members {
		if member.Status != druidv1alpha1.EtcdMemberStatusReady {
			return &result{
				conType: druidv1alpha1.ConditionTypeBootstrappedWithExistingCluster,
				status:  druidv1alpha1.ConditionFalse,
				reason:  "BootstrapInProgress",
				message: "Not all members are ready",
			}
		}
	}

	return &result{
		conType: druidv1alpha1.ConditionTypeBootstrappedWithExistingCluster,
		status:  druidv1alpha1.ConditionTrue,
		reason:  "BootstrapSucceeded",
		message: "All members have successfully joined the existing cluster",
	}
}

// BootstrapWithExistingClusterCheck returns a check for the "BootstrappedWithExistingCluster" condition.
func BootstrapWithExistingClusterCheck(_ client.Client) Checker {
	return &bootstrapWithExistingCluster{}
}
