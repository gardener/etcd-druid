// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package condition

import (
	"context"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type bootstrapWithExistingCluster struct{}

func (b *bootstrapWithExistingCluster) Check(_ context.Context, etcd druidv1alpha1.Etcd) Result {
	// Sticky success: bootstrap is a one-shot event. Once status records
	// the joined source members, this condition stays True for the
	// lifetime of the resource. Re-evaluating member health here would
	// regress the condition to BootstrapInProgress on a transient member
	// outage post-bootstrap, which is incorrect.
	//
	// status.bootstrapWithExistingClusterMembers is populated by
	// reconcile_status.mutateBootstrapWithExistingClusterStatus only after
	// this condition has reached True at least once, so a non-empty status
	// slice is a durable witness of a successful bootstrap. It also stays
	// populated when spec.etcd.bootstrapWithExistingCluster is later
	// cleared (the member-removal trigger), so the condition correctly
	// stays True through that flow until removal completes and a future
	// controller clears status.
	if len(etcd.Status.BootstrapWithExistingClusterMembers) > 0 {
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
