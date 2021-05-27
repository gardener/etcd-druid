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

package etcdmember

import (
	"time"

	"github.com/gardener/etcd-druid/controllers/config"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
)

type readyCheck struct {
	etcdStaleMemberThreshold time.Duration
}

// TimeNow is the function used by this check to get the current time.
var TimeNow = time.Now

func (r *readyCheck) Check(status druidv1alpha1.EtcdStatus) []Result {
	var (
		results   []Result
		threshold = TimeNow().UTC().Add(-1 * r.etcdStaleMemberThreshold)
	)

	for _, etcd := range status.Members {
		if etcd.LastUpdateTime.Time.Before(threshold) {
			results = append(results, &result{
				id:     etcd.ID,
				status: druidv1alpha1.EtcdMemeberStatusUnknown,
				reason: "UnknownMemberStatus",
			})
		}
	}

	return results
}

// ReadyCheck returns a check for the "Ready" condition.
func ReadyCheck(config config.EtcdCustodianController) Checker {
	return &readyCheck{
		etcdStaleMemberThreshold: config.EtcdStaleMemberThreshold,
	}
}
