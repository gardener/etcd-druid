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

package etcdmember_test

import (
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	. "github.com/gardener/etcd-druid/pkg/health/etcdmember"
)

var _ = Describe("Builder", func() {
	var (
		builder Builder
		now     time.Time
	)

	BeforeEach(func() {
		now, _ = time.Parse(time.RFC3339, "2021-06-01")
		builder = NewBuilder()
	})

	JustBeforeEach(func() {
		builder.WithNowFunc(func() metav1.Time {
			return metav1.NewTime(now)
		})
	})

	Describe("#Build", func() {
		Context("when Builder has old members", func() {
			var (
				oldMembers map[string]druidv1alpha1.EtcdMemberStatus
			)
			BeforeEach(func() {
				oldMembers = map[string]druidv1alpha1.EtcdMemberStatus{
					"1": {
						Name:               "member1",
						ID:                 "1",
						Role:               druidv1alpha1.EtcdRoleMember,
						Status:             druidv1alpha1.EtcdMemeberStatusReady,
						Reason:             "foo reason",
						LastUpdateTime:     metav1.NewTime(now.Add(-12 * time.Hour)),
						LastTransitionTime: metav1.NewTime(now.Add(-12 * time.Hour)),
					},
					"2": {
						Name:               "member2",
						ID:                 "2",
						Role:               druidv1alpha1.EtcdRoleMember,
						Status:             druidv1alpha1.EtcdMemeberStatusReady,
						Reason:             "bar reason",
						LastUpdateTime:     metav1.NewTime(now.Add(-6 * time.Hour)),
						LastTransitionTime: metav1.NewTime(now.Add(-6 * time.Hour)),
					},
					"3": {
						Name:               "member3",
						ID:                 "3",
						Role:               druidv1alpha1.EtcdRoleMember,
						Status:             druidv1alpha1.EtcdMemeberStatusReady,
						Reason:             "foobar reason",
						LastUpdateTime:     metav1.NewTime(now.Add(-18 * time.Hour)),
						LastTransitionTime: metav1.NewTime(now.Add(-18 * time.Hour)),
					},
				}

				builder.WithOldMembers([]druidv1alpha1.EtcdMemberStatus{
					oldMembers["1"],
					oldMembers["2"],
					oldMembers["3"],
				})
			})

			It("should correctly merge them", func() {
				builder.WithResults([]Result{
					&result{
						MemberID:     "3",
						MemberStatus: druidv1alpha1.EtcdMemeberStatusUnknown,
						MemberReason: "unknown reason",
					},
				})

				conditions := builder.Build()

				Expect(conditions).To(ConsistOf(
					oldMembers["1"],
					oldMembers["2"],
					MatchFields(IgnoreExtras, Fields{
						"Name":               Equal("member3"),
						"ID":                 Equal("3"),
						"Role":               Equal(druidv1alpha1.EtcdRoleMember),
						"Status":             Equal(druidv1alpha1.EtcdMemeberStatusUnknown),
						"Reason":             Equal("unknown reason"),
						"LastUpdateTime":     Equal(metav1.NewTime(now)),
						"LastTransitionTime": Equal(metav1.NewTime(now)),
					}),
				))
			})
		})

		Context("when Builder has no old members", func() {
			It("should not add any members", func() {
				builder.WithResults([]Result{
					&result{
						MemberID:     "1",
						MemberStatus: druidv1alpha1.EtcdMemeberStatusUnknown,
						MemberReason: "unknown reason",
					},
					&result{
						MemberID:     "2",
						MemberStatus: druidv1alpha1.EtcdMemeberStatusReady,
						MemberReason: "foo reason",
					},
				})

				conditions := builder.Build()

				Expect(conditions).To(BeEmpty())
			})
		})
	})
})
