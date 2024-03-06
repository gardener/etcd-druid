// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// MergeStringMaps merges the content of the newMaps with the oldMap. If a key already exists then
// it gets overwritten by the last value with the same key.
func MergeStringMaps(oldMap map[string]string, newMaps ...map[string]string) map[string]string {
	var out map[string]string

	if oldMap != nil {
		out = make(map[string]string)
	}
	for k, v := range oldMap {
		out[k] = v
	}

	for _, newMap := range newMaps {
		if newMap != nil && out == nil {
			out = make(map[string]string)
		}

		for k, v := range newMap {
			out[k] = v
		}
	}

	return out
}

func nameAndNamespace(namespaceOrName string, nameOpt ...string) (namespace, name string) {
	if len(nameOpt) > 1 {
		panic(fmt.Sprintf("more than name/namespace for key specified: %s/%v", namespaceOrName, nameOpt))
	}
	if len(nameOpt) == 0 {
		name = namespaceOrName
		return
	}
	namespace = namespaceOrName
	name = nameOpt[0]
	return
}

// Key creates a new client.ObjectKey from the given parameters.
// There are only two ways to call this function:
//   - If only namespaceOrName is set, then a client.ObjectKey with name set to namespaceOrName is returned.
//   - If namespaceOrName and one nameOpt is given, then a client.ObjectKey with namespace set to namespaceOrName
//     and name set to nameOpt[0] is returned.
//
// For all other cases, this method panics.
func Key(namespaceOrName string, nameOpt ...string) client.ObjectKey {
	namespace, name := nameAndNamespace(namespaceOrName, nameOpt...)
	return client.ObjectKey{Namespace: namespace, Name: name}
}

// Max returns the larger of x or y.
func Max(x, y int) int {
	if y > x {
		return y
	}
	return x
}

// ComputeScheduleInterval computes the interval between two activations for the given cron schedule.
// Assumes that every cron activation is at equal intervals apart, based on cron schedules such as
// "once every X hours", "once every Y days", "at 1:00pm on every Tuesday", etc.
// TODO: write a new function to accurately compute the previous activation time from the cron schedule
// in order to compute when the previous activation of the cron schedule was supposed to have occurred,
// instead of relying on the assumption that all the cron activations are evenly spaced.
func ComputeScheduleInterval(cronSchedule string) (time.Duration, error) {
	schedule, err := cron.ParseStandard(cronSchedule)
	if err != nil {
		return 0, err
	}

	nextScheduledTime := schedule.Next(time.Now())
	nextNextScheduledTime := schedule.Next(nextScheduledTime)
	return nextNextScheduledTime.Sub(nextScheduledTime), nil
}
