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

package health

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func daemonSetMaxUnavailable(daemonSet *appsv1.DaemonSet) int32 {
	if daemonSet.Status.DesiredNumberScheduled == 0 || daemonSet.Spec.UpdateStrategy.Type != appsv1.RollingUpdateDaemonSetStrategyType {
		return 0
	}

	rollingUpdate := daemonSet.Spec.UpdateStrategy.RollingUpdate
	if rollingUpdate == nil {
		return 0
	}

	maxUnavailable, err := intstr.GetValueFromIntOrPercent(rollingUpdate.MaxUnavailable, int(daemonSet.Status.DesiredNumberScheduled), false)
	if err != nil {
		return 0
	}

	return int32(maxUnavailable)
}

// CheckDaemonSet checks whether the given DaemonSet is healthy.
// A DaemonSet is considered healthy if its controller observed its current revision and if
// its desired number of scheduled pods is equal to its updated number of scheduled pods.
func CheckDaemonSet(daemonSet *appsv1.DaemonSet) error {
	if daemonSet.Status.ObservedGeneration < daemonSet.Generation {
		return fmt.Errorf("observed generation outdated (%d/%d)", daemonSet.Status.ObservedGeneration, daemonSet.Generation)
	}

	if daemonSet.Status.CurrentNumberScheduled < daemonSet.Status.DesiredNumberScheduled {
		return fmt.Errorf("not enough scheduled pods (%d/%d)", daemonSet.Status.CurrentNumberScheduled, daemonSet.Status.DesiredNumberScheduled)
	}

	if daemonSet.Status.NumberMisscheduled > 0 {
		return fmt.Errorf("misscheduled pods found (%d)", daemonSet.Status.NumberMisscheduled)
	}

	// Check if DaemonSet rollout is ongoing.
	if daemonSet.Status.UpdatedNumberScheduled < daemonSet.Status.DesiredNumberScheduled {
		if maxUnavailable := daemonSetMaxUnavailable(daemonSet); daemonSet.Status.NumberUnavailable > maxUnavailable {
			return fmt.Errorf("too many unavailable pods found (%d/%d, only max. %d unavailable pods allowed)", daemonSet.Status.NumberUnavailable, daemonSet.Status.CurrentNumberScheduled, maxUnavailable)
		}
	} else {
		if daemonSet.Status.NumberUnavailable > 0 {
			return fmt.Errorf("too many unavailable pods found (%d/%d)", daemonSet.Status.NumberUnavailable, daemonSet.Status.CurrentNumberScheduled)
		}
	}

	return nil
}
