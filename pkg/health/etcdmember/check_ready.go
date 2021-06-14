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
	"context"
	"time"

	kutil "github.com/gardener/gardener/pkg/utils/kubernetes"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	controllersconfig "github.com/gardener/etcd-druid/controllers/config"
)

type readyCheck struct {
	memberConfig controllersconfig.EtcdMemberConfig
	cl           client.Client
}

// TimeNow is the function used by this check to get the current time.
var TimeNow = time.Now

func (r *readyCheck) Check(ctx context.Context, etcd druidv1alpha1.Etcd) []Result {
	var (
		results   []Result
		checkTime = TimeNow().UTC()
	)

	for _, member := range etcd.Status.Members {
		// Check if status must be changed from Unknown to NotReady.
		if member.Status == druidv1alpha1.EtcdMemeberStatusUnknown &&
			member.LastTransitionTime.Time.Add(r.memberConfig.EtcdMemberNotReadyThreshold).Before(checkTime) {
			results = append(results, &result{
				id:     member.ID,
				status: druidv1alpha1.EtcdMemeberStatusNotReady,
				reason: "UnkownStateTimeout",
			})
			continue
		}

		// Skip if status is not already Unknown and LastUpdateTime is within grace period.
		if !member.LastUpdateTime.Time.Add(r.memberConfig.EtcdMemberUnknownThreshold).Before(checkTime) {
			continue
		}

		// If pod is not running or cannot be found then we deduce that the status is NotReady.
		ready, err := r.checkPodIsRunning(ctx, etcd.Namespace, member)
		if (err == nil && !ready) || apierrors.IsNotFound(err) {
			results = append(results, &result{
				id:     member.ID,
				status: druidv1alpha1.EtcdMemeberStatusNotReady,
				reason: "PodNotRunning",
			})
			continue
		}

		// For every other reason the status is Unknown.
		results = append(results, &result{
			id:     member.ID,
			status: druidv1alpha1.EtcdMemeberStatusUnknown,
			reason: "UnknownMemberStatus",
		})
	}

	return results
}

func (r *readyCheck) checkPodIsRunning(ctx context.Context, namespace string, member druidv1alpha1.EtcdMemberStatus) (bool, error) {
	pod := &corev1.Pod{}
	if err := r.cl.Get(ctx, kutil.Key(namespace, member.Name), pod); err != nil {
		return false, err
	}
	return pod.Status.Phase == corev1.PodRunning, nil
}

// ReadyCheck returns a check for the "Ready" condition.
func ReadyCheck(cl client.Client, config controllersconfig.EtcdCustodianController) Checker {
	return &readyCheck{
		cl:           cl,
		memberConfig: config.EtcdMember,
	}
}
