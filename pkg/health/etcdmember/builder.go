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
	"sort"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Builder is an interface for building status objects for etcd members.
type Builder interface {
	WithOldMembers(members []druidv1alpha1.EtcdMemberStatus) Builder
	WithResults(results []Result) Builder
	WithNowFunc(now func() metav1.Time) Builder
	Build() []druidv1alpha1.EtcdMemberStatus
}

type defaultBuilder struct {
	old     map[string]druidv1alpha1.EtcdMemberStatus
	results map[string]Result
	nowFunc func() metav1.Time
}

// NewBuilder returns a Builder for a specific etcd member status.
func NewBuilder() Builder {
	return &defaultBuilder{
		old:     make(map[string]druidv1alpha1.EtcdMemberStatus),
		results: make(map[string]Result),
		nowFunc: func() metav1.Time {
			return metav1.NewTime(time.Now().UTC())
		},
	}
}

// WithOldMember sets the old etcd member statuses. It can be used to provide default values.
func (b *defaultBuilder) WithOldMembers(members []druidv1alpha1.EtcdMemberStatus) Builder {
	for _, member := range members {
		b.old[member.Name] = member
	}

	return b
}

// WithResults adds the results.
func (b *defaultBuilder) WithResults(results []Result) Builder {
	for _, res := range results {
		if res == nil {
			continue
		}
		b.results[res.Name()] = res
	}

	return b
}

// WithNowFunc sets the function used for getting the current time.
// Should only be used for tests.
func (b *defaultBuilder) WithNowFunc(now func() metav1.Time) Builder {
	b.nowFunc = now
	return b
}

// Build creates the etcd member statuses.
// It merges the existing members with the results added to the builder.
// If OldCondition is provided:
// - Any changes to status set the `LastTransitionTime`
func (b *defaultBuilder) Build() []druidv1alpha1.EtcdMemberStatus {
	var (
		now = b.nowFunc()

		members []druidv1alpha1.EtcdMemberStatus
	)

	for name, res := range b.results {
		memberStatus := druidv1alpha1.EtcdMemberStatus{
			ID:                 res.ID(),
			Name:               res.Name(),
			Role:               res.Role(),
			Status:             res.Status(),
			Reason:             res.Reason(),
			LastTransitionTime: now,
		}

		// Don't reset LastTransitionTime if status didn't change
		if oldMemberStatus, ok := b.old[name]; ok {
			if oldMemberStatus.Status == res.Status() {
				memberStatus.LastTransitionTime = oldMemberStatus.LastTransitionTime
			}
		}

		members = append(members, memberStatus)
	}

	sort.Slice(members, func(i, j int) bool {
		return members[i].Name < members[j].Name
	})

	return members
}
