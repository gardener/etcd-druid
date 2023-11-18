// Copyright (c) 2021 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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

	result := &result{
		conType: druidv1alpha1.ConditionTypeAllMembersReady,
		status:  druidv1alpha1.ConditionFalse,
		reason:  "NotAllMembersReady",
		message: "At least one member is not ready",
	}

	if int32(len(etcd.Status.Members)) < etcd.Spec.Replicas {
		// not all members are registered yet
		return result
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
		result.status = druidv1alpha1.ConditionTrue
		result.reason = "AllMembersReady"
		result.message = "All members are ready"
	}

	return result
}

// AllMembersCheck returns a check for the "AllMembersReady" condition.
func AllMembersCheck(_ client.Client) Checker {
	return &allMembersReady{}
}
