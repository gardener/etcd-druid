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
	"github.com/gardener/etcd-druid/pkg/utils"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type readyCheck struct{}

func (r *readyCheck) Check(ctx context.Context, etcd druidv1alpha1.Etcd) Result {
	if etcd.Status.ClusterSize == nil {
		return &result{
			conType: druidv1alpha1.ConditionTypeReady,
			status:  druidv1alpha1.ConditionUnknown,
			reason:  "ClusterSizeUnknown",
			message: "Cannot determine readiness of cluster since no cluster size has been calculated",
		}
	}

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
		size         = utils.Max(int(*etcd.Status.ClusterSize), len(etcd.Status.Members))
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
func ReadyCheck(cl client.Client) Checker {
	return &readyCheck{}
}
