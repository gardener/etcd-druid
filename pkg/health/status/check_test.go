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

package status_test

import (
	"context"
	"time"

	"github.com/gardener/gardener/pkg/utils/test"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	controllerconfig "github.com/gardener/etcd-druid/controllers/config"
	"github.com/gardener/etcd-druid/pkg/health/condition"
	"github.com/gardener/etcd-druid/pkg/health/etcdmember"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"

	. "github.com/gardener/etcd-druid/pkg/health/status"
)

var _ = Describe("Check", func() {
	Describe("#Check", func() {
		It("should correctly execute checks and fill status", func() {
			config := controllerconfig.EtcdCustodianController{
				EtcdStaleMemberThreshold: 1 * time.Minute,
			}
			timeBefore, _ := time.Parse(time.RFC3339, "2021-06-01T00:00:00Z")
			timeNow := timeBefore.Add(1 * time.Hour)

			status := druidv1alpha1.EtcdStatus{
				Conditions: []druidv1alpha1.Condition{
					{
						Type:               druidv1alpha1.ConditionTypeReady,
						Status:             druidv1alpha1.ConditionTrue,
						LastTransitionTime: metav1.NewTime(timeBefore),
						LastUpdateTime:     metav1.NewTime(timeBefore),
						Reason:             "foo reason",
						Message:            "foo message",
					},
					{
						Type:               druidv1alpha1.ConditionTypeAllMembersReady,
						Status:             druidv1alpha1.ConditionTrue,
						LastTransitionTime: metav1.NewTime(timeBefore),
						LastUpdateTime:     metav1.NewTime(timeBefore),
						Reason:             "bar reason",
						Message:            "bar message",
					},
					{
						Type:               druidv1alpha1.ConditionTypeBackupReady,
						Status:             druidv1alpha1.ConditionUnknown,
						LastTransitionTime: metav1.NewTime(timeBefore),
						LastUpdateTime:     metav1.NewTime(timeBefore),
						Reason:             "foobar reason",
						Message:            "foobar message",
					},
				},
				Members: []druidv1alpha1.EtcdMemberStatus{
					{
						ID:                 "1",
						Name:               "Member1",
						Role:               druidv1alpha1.EtcdRoleMember,
						Status:             druidv1alpha1.EtcdMemeberStatusReady,
						LastTransitionTime: metav1.NewTime(timeBefore),
						LastUpdateTime:     metav1.NewTime(timeBefore),
						Reason:             "foo reason",
					},
					{
						ID:                 "2",
						Name:               "Member2",
						Role:               druidv1alpha1.EtcdRoleLearner,
						Status:             druidv1alpha1.EtcdMemeberStatusNotReady,
						LastTransitionTime: metav1.NewTime(timeBefore),
						LastUpdateTime:     metav1.NewTime(timeBefore),
						Reason:             "bar reason",
					},
					{
						ID:                 "3",
						Name:               "Member3",
						Role:               druidv1alpha1.EtcdRoleMember,
						Status:             druidv1alpha1.EtcdMemeberStatusReady,
						LastTransitionTime: metav1.NewTime(timeBefore),
						LastUpdateTime:     metav1.NewTime(timeBefore),
						Reason:             "foobar reason",
					},
				},
			}

			defer test.WithVar(&ConditionChecks, []ConditionCheckFn{
				func() condition.Checker {
					return createConditionCheck(druidv1alpha1.ConditionTypeReady, druidv1alpha1.ConditionFalse, "FailedConditionCheck", "check failed")
				},
				func() condition.Checker {
					return createConditionCheck(druidv1alpha1.ConditionTypeAllMembersReady, druidv1alpha1.ConditionTrue, "bar reason", "bar message")
				},
			})()

			defer test.WithVar(&EtcdMemberChecks, []EtcdMemberCheckFn{
				func(_ controllerconfig.EtcdCustodianController) etcdmember.Checker {
					return createEtcdMemberCheck("1", druidv1alpha1.EtcdMemeberStatusUnknown, "Unknown")
				},
			})()

			defer test.WithVar(&TimeNow, func() time.Time { return timeNow })()

			checker := NewChecker(config)

			Expect(checker.Check(context.Background(), &status)).To(Succeed())

			Expect(status.Conditions).To(ConsistOf(
				MatchFields(IgnoreExtras, Fields{
					"Type":               Equal(druidv1alpha1.ConditionTypeReady),
					"Status":             Equal(druidv1alpha1.ConditionFalse),
					"LastTransitionTime": Equal(metav1.NewTime(timeNow)),
					"LastUpdateTime":     Equal(metav1.NewTime(timeNow)),
					"Reason":             Equal("FailedConditionCheck"),
					"Message":            Equal("check failed"),
				}),
				MatchFields(IgnoreExtras, Fields{
					"Type":               Equal(druidv1alpha1.ConditionTypeAllMembersReady),
					"Status":             Equal(druidv1alpha1.ConditionTrue),
					"LastTransitionTime": Equal(metav1.NewTime(timeBefore)),
					"LastUpdateTime":     Equal(metav1.NewTime(timeNow)),
					"Reason":             Equal("bar reason"),
					"Message":            Equal("bar message"),
				}),
				MatchFields(IgnoreExtras, Fields{
					"Type":               Equal(druidv1alpha1.ConditionTypeBackupReady),
					"Status":             Equal(druidv1alpha1.ConditionUnknown),
					"LastTransitionTime": Equal(metav1.NewTime(timeBefore)),
					"LastUpdateTime":     Equal(metav1.NewTime(timeBefore)),
					"Reason":             Equal("foobar reason"),
					"Message":            Equal("foobar message"),
				}),
			))

			Expect(status.Members).To(ConsistOf(
				MatchFields(IgnoreExtras, Fields{
					"ID":                 Equal("1"),
					"Name":               Equal("Member1"),
					"Role":               Equal(druidv1alpha1.EtcdRoleMember),
					"Status":             Equal(druidv1alpha1.EtcdMemeberStatusUnknown),
					"LastTransitionTime": Equal(metav1.NewTime(timeNow)),
					"LastUpdateTime":     Equal(metav1.NewTime(timeNow)),
					"Reason":             Equal("Unknown"),
				}),
				MatchFields(IgnoreExtras, Fields{
					"ID":                 Equal("2"),
					"Name":               Equal("Member2"),
					"Role":               Equal(druidv1alpha1.EtcdRoleLearner),
					"Status":             Equal(druidv1alpha1.EtcdMemeberStatusNotReady),
					"LastTransitionTime": Equal(metav1.NewTime(timeBefore)),
					"LastUpdateTime":     Equal(metav1.NewTime(timeBefore)),
					"Reason":             Equal("bar reason"),
				}),
				MatchFields(IgnoreExtras, Fields{
					"ID":                 Equal("3"),
					"Name":               Equal("Member3"),
					"Role":               Equal(druidv1alpha1.EtcdRoleMember),
					"Status":             Equal(druidv1alpha1.EtcdMemeberStatusReady),
					"LastTransitionTime": Equal(metav1.NewTime(timeBefore)),
					"LastUpdateTime":     Equal(metav1.NewTime(timeBefore)),
					"Reason":             Equal("foobar reason"),
				}),
			))

		})
	})
})

type conditionResult struct {
	ConType    druidv1alpha1.ConditionType
	ConStatus  druidv1alpha1.ConditionStatus
	ConReason  string
	ConMessage string
}

func (r *conditionResult) ConditionType() druidv1alpha1.ConditionType {
	return r.ConType
}

func (r *conditionResult) Status() druidv1alpha1.ConditionStatus {
	return r.ConStatus
}

func (r *conditionResult) Reason() string {
	return r.ConReason
}

func (r *conditionResult) Message() string {
	return r.ConMessage
}

type testChecker struct {
	result *conditionResult
}

func (t *testChecker) Check(_ druidv1alpha1.EtcdStatus) condition.Result {
	return t.result
}

func createConditionCheck(conType druidv1alpha1.ConditionType, status druidv1alpha1.ConditionStatus, reason, message string) condition.Checker {
	return &testChecker{
		result: &conditionResult{
			ConType:    conType,
			ConStatus:  status,
			ConReason:  reason,
			ConMessage: message,
		},
	}
}

type etcdMemberResult struct {
	id     string
	status druidv1alpha1.EtcdMemberConditionStatus
	reason string
}

func (r *etcdMemberResult) ID() string {
	return r.id
}

func (r *etcdMemberResult) Status() druidv1alpha1.EtcdMemberConditionStatus {
	return r.status
}

func (r *etcdMemberResult) Reason() string {
	return r.reason
}

type etcdMemberTestChecker struct {
	result *etcdMemberResult
}

func (t *etcdMemberTestChecker) Check(_ druidv1alpha1.EtcdStatus) []etcdmember.Result {
	return []etcdmember.Result{
		t.result,
	}
}

func createEtcdMemberCheck(id string, status druidv1alpha1.EtcdMemberConditionStatus, reason string) etcdmember.Checker {
	return &etcdMemberTestChecker{
		result: &etcdMemberResult{
			id:     id,
			status: status,
			reason: reason,
		},
	}
}
