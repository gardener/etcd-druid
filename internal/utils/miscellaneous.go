// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"fmt"
	"maps"
	"strconv"
	"strings"
	"time"

	druiderrors "github.com/gardener/etcd-druid/internal/errors"

	"github.com/robfig/cron/v3"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// MergeMaps merges the contents of maps. All maps will be processed in the order
// in which they are sent. For overlapping keys across source maps, value in the merged map
// for this key will be from the last occurrence of the key-value.
func MergeMaps[K comparable, V any](sourceMaps ...map[K]V) map[K]V {
	if sourceMaps == nil {
		return nil
	}
	merged := make(map[K]V)
	for _, m := range sourceMaps {
		maps.Copy(merged, m)
	}
	return merged
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

// IsEmptyString returns true if the string is empty or contains only whitespace characters.
func IsEmptyString(s string) bool {
	return len(strings.TrimSpace(s)) == 0
}

// IfConditionOr implements a simple ternary operator, if the passed condition is true then trueVal is returned else falseVal is returned.
func IfConditionOr[T any](condition bool, trueVal, falseVal T) T {
	if condition {
		return trueVal
	}
	return falseVal
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

// GetBoolValueOrError returns the boolean value for the given key from the data map,
// and returns error if the key is not found or the value is not a valid boolean.
func GetBoolValueOrError(data map[string]string, key string) (bool, error) {
	value, ok := data[key]
	if !ok {
		return false, fmt.Errorf("key %s does not exist: %w", key, druiderrors.ErrNotFound)
	}
	result, err := strconv.ParseBool(value)
	if err != nil {
		return false, err
	}
	return result, nil
}
