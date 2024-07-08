// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package condition

import (
	"context"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type allMembersReady struct{}

func (a *allMembersReady) Check(_ context.Context, etcd druidv1alpha1.Etcd) Result {
	if len(etcd.Status.Members) == 0 {
		return &result{
			conType: druidv1alpha1.ConditionTypeAllMembersReady,
			status:  druidv1alpha1.ConditionUnknown,
			reason:  "NoMembersInStatus",
			message: "Cannot determine readiness since status has no members",
		}
	}

	res := &result{
		conType: druidv1alpha1.ConditionTypeAllMembersReady,
		status:  druidv1alpha1.ConditionFalse,
		reason:  "NotAllMembersReady",
		message: "At least one member is not ready",
	}

	if int32(len(etcd.Status.Members)) < etcd.Spec.Replicas {
		// not all members are registered yet
		return res
	}

	// If we are here this means that all members have registered. Check if any member
	// has a not-ready status. If there is at least one then set the overall status as false.
	ready := true
	for _, member := range etcd.Status.Members {
		ready = ready && member.Status == druidv1alpha1.EtcdMemberStatusReady
		if !ready {
			break
		}
	}
	if ready {
		res.status = druidv1alpha1.ConditionTrue
		res.reason = "AllMembersReady"
		res.message = "All members are ready"
	}

	return res
}

// AllMembersReadyCheck returns a check for the "AllMembersReady" condition.
func AllMembersReadyCheck(_ client.Client) Checker {
	return &allMembersReady{}
}
