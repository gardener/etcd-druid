// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package condition

import (
	"sort"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// skipMergeConditions contain the list of conditions we don't want to add to the list if not recalculated
var skipMergeConditions = map[druidv1alpha1.ConditionType]struct{}{
	druidv1alpha1.ConditionTypeReady:                    {},
	druidv1alpha1.ConditionTypeAllMembersReady:          {},
	druidv1alpha1.ConditionTypeBackupReady:              {},
	druidv1alpha1.ConditionTypeFullSnapshotBackupReady:  {},
	druidv1alpha1.ConditionTypeDeltaSnapshotBackupReady: {},
}

// Builder is an interface for building conditions.
type Builder interface {
	WithOldConditions(conditions []druidv1alpha1.Condition) Builder
	WithResults(result []Result) Builder
	WithNowFunc(now func() metav1.Time) Builder
	Build(replicas int32) []druidv1alpha1.Condition
}

type defaultBuilder struct {
	old     map[druidv1alpha1.ConditionType]druidv1alpha1.Condition
	results map[druidv1alpha1.ConditionType]Result
	nowFunc func() metav1.Time
}

// NewBuilder returns a Builder for a specific condition.
func NewBuilder() Builder {
	return &defaultBuilder{
		old:     make(map[druidv1alpha1.ConditionType]druidv1alpha1.Condition),
		results: make(map[druidv1alpha1.ConditionType]Result),
		nowFunc: func() metav1.Time {
			return metav1.NewTime(time.Now().UTC())
		},
	}
}

// WithOldConditions sets the old conditions. It can be used to provide default values.
func (b *defaultBuilder) WithOldConditions(conditions []druidv1alpha1.Condition) Builder {
	for _, cond := range conditions {
		b.old[cond.Type] = cond
	}

	return b
}

// WithResults adds the results.
func (b *defaultBuilder) WithResults(results []Result) Builder {
	for _, r := range results {
		if r == nil {
			continue
		}
		b.results[r.ConditionType()] = r
	}

	return b
}

// WithNowFunc sets the function used for getting the current time.
// Should only be used for tests.
func (b *defaultBuilder) WithNowFunc(now func() metav1.Time) Builder {
	b.nowFunc = now
	return b
}

// Build creates the conditions.
// It merges the existing conditions with the results added to the builder.
// If OldCondition is provided:
// - Any changes to status set the `LastTransitionTime`
// - `LastUpdateTime` is always set.
func (b *defaultBuilder) Build(replicas int32) []druidv1alpha1.Condition {
	var (
		now        = b.nowFunc()
		conditions []druidv1alpha1.Condition
	)

	for condType, res := range b.results {
		condition, ok := b.old[condType]
		if !ok {
			condition = druidv1alpha1.Condition{
				Type:               condType,
				LastTransitionTime: now,
			}
		}

		if condition.Status != res.Status() {
			condition.LastTransitionTime = now
		}
		condition.LastUpdateTime = now
		if replicas == 0 {
			if condition.Status == "" {
				condition.Status = druidv1alpha1.ConditionUnknown
			}
			condition.Reason = NotChecked
			condition.Message = "etcd cluster has been scaled down"
		} else {
			condition.Status = res.Status()
			condition.Message = res.Message()
			condition.Reason = res.Reason()
		}

		conditions = append(conditions, condition)
		delete(b.old, condType)
	}

	for _, condition := range b.old {
		// Do not add conditions that are part of the skipMergeConditions list
		_, ok := skipMergeConditions[condition.Type]
		if ok {
			continue
		}
		// Add existing conditions as they were. This needs to be changed when SSA is used.
		conditions = append(conditions, condition)
	}

	sort.Slice(conditions, func(i, j int) bool {
		return conditions[i].Type < conditions[j].Type
	})

	return conditions
}
