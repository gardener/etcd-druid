// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
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
	if etcd.Spec.Etcd.BootstrapWithExistingCluster == nil || len(etcd.Spec.Etcd.BootstrapWithExistingCluster.Members) == 0 {
		return nil
	}

	if len(etcd.Status.Members) < int(etcd.Spec.Replicas) {
		return &result{
			conType: druidv1alpha1.ConditionTypeBootstrapWithExistingCluster,
			status:  druidv1alpha1.ConditionFalse,
			reason:  "BootstrapInProgress",
			message: "Not all target members have joined the cluster yet",
		}
	}

	for _, member := range etcd.Status.Members {
		if member.Status != druidv1alpha1.EtcdMemberStatusReady {
			return &result{
				conType: druidv1alpha1.ConditionTypeBootstrapWithExistingCluster,
				status:  druidv1alpha1.ConditionFalse,
				reason:  "BootstrapInProgress",
				message: "Not all target members are ready",
			}
		}
	}

	return &result{
		conType: druidv1alpha1.ConditionTypeBootstrapWithExistingCluster,
		status:  druidv1alpha1.ConditionTrue,
		reason:  "BootstrapSucceeded",
		message: "All target members have successfully joined the existing cluster",
	}
}

// BootstrapWithExistingClusterCheck returns a check for the "BootstrapWithExistingCluster" condition.
func BootstrapWithExistingClusterCheck(_ client.Client) Checker {
	return &bootstrapWithExistingCluster{}
}
