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
		b.old[member.ID] = member
	}

	return b
}

// WithResults adds the results.
func (b *defaultBuilder) WithResults(results []Result) Builder {
	for _, res := range results {
		if res == nil {
			continue
		}
		b.results[res.ID()] = res
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
// - `LastUpdateTime` is always set.
func (b *defaultBuilder) Build() []druidv1alpha1.EtcdMemberStatus {
	var (
		now = b.nowFunc()

		members []druidv1alpha1.EtcdMemberStatus
	)

	for id, res := range b.results {
		member, ok := b.old[id]
		if !ok {
			// Continue if we can't find an existing member because druid is not supposed to add one.
			continue
		}

		member.Status = res.Status()
		member.LastTransitionTime = now
		member.LastUpdateTime = now
		member.Reason = res.Reason()

		members = append(members, member)
		delete(b.old, id)
	}

	for _, member := range b.old {
		// Add existing members as they were. This needs to be changed when SSA is used.
		members = append(members, member)
	}

	return members
}
