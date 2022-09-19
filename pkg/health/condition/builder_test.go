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

package condition_test

import (
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	. "github.com/gardener/etcd-druid/pkg/health/condition"
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
		Context("when Builder has old conditions", func() {
			var (
				oldConditionTime time.Time
				oldConditions    []druidv1alpha1.Condition
			)
			BeforeEach(func() {
				oldConditionTime = now.Add(-12 * time.Hour)

				oldConditions = []druidv1alpha1.Condition{
					{
						Type:               druidv1alpha1.ConditionTypeAllMembersReady,
						LastUpdateTime:     metav1.NewTime(oldConditionTime),
						LastTransitionTime: metav1.NewTime(oldConditionTime),
						Status:             druidv1alpha1.ConditionTrue,
						Reason:             "foo reason",
						Message:            "foo message",
					},
					{
						Type:               druidv1alpha1.ConditionTypeReady,
						LastUpdateTime:     metav1.NewTime(oldConditionTime),
						LastTransitionTime: metav1.NewTime(oldConditionTime),
						Status:             druidv1alpha1.ConditionFalse,
						Reason:             "bar reason",
						Message:            "bar message",
					},
					{
						Type:               druidv1alpha1.ConditionTypeBackupReady,
						LastUpdateTime:     metav1.NewTime(oldConditionTime),
						LastTransitionTime: metav1.NewTime(oldConditionTime),
						Status:             druidv1alpha1.ConditionTrue,
						Reason:             "foobar reason",
						Message:            "foobar message",
					},
				}

				builder.WithOldConditions(oldConditions)
			})

			It("should not add old conditions", func() {
				builder.WithResults([]Result{
					&result{
						ConType:    druidv1alpha1.ConditionTypeAllMembersReady,
						ConStatus:  druidv1alpha1.ConditionTrue,
						ConReason:  "new reason",
						ConMessage: "new message",
					},
					&result{
						ConType:    druidv1alpha1.ConditionTypeReady,
						ConStatus:  druidv1alpha1.ConditionTrue,
						ConReason:  "new reason",
						ConMessage: "new message",
					},
				})

				conditions := builder.Build(1)

				Expect(conditions).To(ConsistOf(
					MatchFields(IgnoreExtras, Fields{
						"Type":               Equal(druidv1alpha1.ConditionTypeAllMembersReady),
						"LastUpdateTime":     Equal(metav1.NewTime(now)),
						"LastTransitionTime": Equal(metav1.NewTime(oldConditionTime)),
						"Status":             Equal(druidv1alpha1.ConditionTrue),
						"Reason":             Equal("new reason"),
						"Message":            Equal("new message"),
					}),
					MatchFields(IgnoreExtras, Fields{
						"Type":               Equal(druidv1alpha1.ConditionTypeReady),
						"LastUpdateTime":     Equal(metav1.NewTime(now)),
						"LastTransitionTime": Equal(metav1.NewTime(now)),
						"Status":             Equal(druidv1alpha1.ConditionTrue),
						"Reason":             Equal("new reason"),
						"Message":            Equal("new message"),
					}),
				))
			})
		})

		Context("when Builder has no old conditions", func() {
			It("should correctly set the new conditions", func() {
				builder.WithResults([]Result{
					&result{
						ConType:    druidv1alpha1.ConditionTypeAllMembersReady,
						ConStatus:  druidv1alpha1.ConditionTrue,
						ConReason:  "new reason",
						ConMessage: "new message",
					},
					&result{
						ConType:    druidv1alpha1.ConditionTypeReady,
						ConStatus:  druidv1alpha1.ConditionTrue,
						ConReason:  "new reason",
						ConMessage: "new message",
					},
				})

				conditions := builder.Build(1)

				Expect(conditions).To(ConsistOf(
					MatchFields(IgnoreExtras, Fields{
						"Type":               Equal(druidv1alpha1.ConditionTypeAllMembersReady),
						"LastUpdateTime":     Equal(metav1.NewTime(now)),
						"LastTransitionTime": Equal(metav1.NewTime(now)),
						"Status":             Equal(druidv1alpha1.ConditionTrue),
						"Reason":             Equal("new reason"),
						"Message":            Equal("new message"),
					}),
					MatchFields(IgnoreExtras, Fields{
						"Type":               Equal(druidv1alpha1.ConditionTypeReady),
						"LastUpdateTime":     Equal(metav1.NewTime(now)),
						"LastTransitionTime": Equal(metav1.NewTime(now)),
						"Status":             Equal(druidv1alpha1.ConditionTrue),
						"Reason":             Equal("new reason"),
						"Message":            Equal("new message"),
					}),
				))
			})
		})
	})
})

type result struct {
	ConType    druidv1alpha1.ConditionType
	ConStatus  druidv1alpha1.ConditionStatus
	ConReason  string
	ConMessage string
}

func (r *result) ConditionType() druidv1alpha1.ConditionType {
	return r.ConType
}

func (r *result) Status() druidv1alpha1.ConditionStatus {
	return r.ConStatus
}

func (r *result) Reason() string {
	return r.ConReason
}

func (r *result) Message() string {
	return r.ConMessage
}
