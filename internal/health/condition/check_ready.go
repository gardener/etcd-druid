// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package condition

import (
	"context"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type readyCheck struct{}

func (r *readyCheck) Check(_ context.Context, etcd druidv1alpha1.Etcd) Result {

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

	return &result{
		conType: druidv1alpha1.ConditionTypeReady,
		status:  druidv1alpha1.ConditionTrue,
		reason:  "Quorate",
		message: "The majority of ETCD members is ready",
	}
}

// ReadyCheck returns a check for the "Ready" condition.
func ReadyCheck(_ client.Client) Checker {
	return &readyCheck{}
}
