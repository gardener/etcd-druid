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

	"k8s.io/utils/pointer"

	"github.com/go-logr/logr"

	"github.com/gardener/etcd-druid/pkg/health/condition"
	"github.com/gardener/etcd-druid/pkg/health/etcdmember"
	"github.com/gardener/gardener/pkg/utils/test"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	controllersconfig "github.com/gardener/etcd-druid/controllers/config"
	. "github.com/gardener/etcd-druid/pkg/health/status"
)

var _ = Describe("Check", func() {
	Describe("#Check", func() {
		It("should correctly execute checks and fill status", func() {
			memberRoleLeader := druidv1alpha1.EtcdRoleLeader
			memberRoleMember := druidv1alpha1.EtcdRoleMember

			config := controllersconfig.EtcdCustodianController{
				EtcdMember: controllersconfig.EtcdMemberConfig{},
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
						ID:                 pointer.StringPtr("1"),
						Name:               "member1",
						Status:             druidv1alpha1.EtcdMemberStatusReady,
						LastTransitionTime: metav1.NewTime(timeBefore),
						Reason:             "foo reason",
					},
					{
						ID:                 pointer.StringPtr("2"),
						Name:               "member2",
						Status:             druidv1alpha1.EtcdMemberStatusNotReady,
						LastTransitionTime: metav1.NewTime(timeBefore),
						Reason:             "bar reason",
					},
					{
						ID:                 pointer.StringPtr("3"),
						Name:               "member3",
						Status:             druidv1alpha1.EtcdMemberStatusReady,
						LastTransitionTime: metav1.NewTime(timeBefore),
						Reason:             "foobar reason",
					},
				},
			}

			etcd := &druidv1alpha1.Etcd{
				Spec: druidv1alpha1.EtcdSpec{
					Replicas: 1,
				},
				Status: status,
			}

			defer test.WithVar(&ConditionChecks, []ConditionCheckFn{
				func(client.Client) condition.Checker {
					return createConditionCheck(druidv1alpha1.ConditionTypeReady, druidv1alpha1.ConditionFalse, "FailedConditionCheck", "check failed")
				},
				func(client.Client) condition.Checker {
					return createConditionCheck(druidv1alpha1.ConditionTypeAllMembersReady, druidv1alpha1.ConditionTrue, "bar reason", "bar message")
				},
				func(client.Client) condition.Checker {
					return createConditionCheck(druidv1alpha1.ConditionTypeBackupReady, druidv1alpha1.ConditionUnknown, "foobar reason", "foobar message")
				},
			})()

			defer test.WithVar(&EtcdMemberChecks, []EtcdMemberCheckFn{
				func(_ client.Client, _ logr.Logger, _ controllersconfig.EtcdCustodianController) etcdmember.Checker {
					return createEtcdMemberCheck(
						etcdMemberResult{pointer.StringPtr("1"), "member1", &memberRoleLeader, druidv1alpha1.EtcdMemberStatusUnknown, "Unknown"},
						etcdMemberResult{pointer.StringPtr("2"), "member2", &memberRoleMember, druidv1alpha1.EtcdMemberStatusNotReady, "bar reason"},
						etcdMemberResult{pointer.StringPtr("3"), "member3", &memberRoleMember, druidv1alpha1.EtcdMemberStatusReady, "foobar reason"},
					)
				},
			})()

			defer test.WithVar(&TimeNow, func() time.Time { return timeNow })()

			checker := NewChecker(nil, config)

			Expect(checker.Check(context.Background(), log.NullLogger{}, etcd)).To(Succeed())

			Expect(etcd.Status.Conditions).To(ConsistOf(
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
					"LastUpdateTime":     Equal(metav1.NewTime(timeNow)),
					"Reason":             Equal("foobar reason"),
					"Message":            Equal("foobar message"),
				}),
			))

			Expect(etcd.Status.Members).To(ConsistOf(
				MatchFields(IgnoreExtras, Fields{
					"ID":                 PointTo(Equal("1")),
					"Name":               Equal("member1"),
					"Role":               PointTo(Equal(druidv1alpha1.EtcdRoleLeader)),
					"Status":             Equal(druidv1alpha1.EtcdMemberStatusUnknown),
					"LastTransitionTime": Equal(metav1.NewTime(timeNow)),
					"Reason":             Equal("Unknown"),
				}),
				MatchFields(IgnoreExtras, Fields{
					"ID":                 PointTo(Equal("2")),
					"Name":               Equal("member2"),
					"Role":               PointTo(Equal(druidv1alpha1.EtcdRoleMember)),
					"Status":             Equal(druidv1alpha1.EtcdMemberStatusNotReady),
					"LastTransitionTime": Equal(metav1.NewTime(timeBefore)),
					"Reason":             Equal("bar reason"),
				}),
				MatchFields(IgnoreExtras, Fields{
					"ID":                 PointTo(Equal("3")),
					"Name":               Equal("member3"),
					"Role":               PointTo(Equal(druidv1alpha1.EtcdRoleMember)),
					"Status":             Equal(druidv1alpha1.EtcdMemberStatusReady),
					"LastTransitionTime": Equal(metav1.NewTime(timeBefore)),
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

func (t *testChecker) Check(_ context.Context, _ druidv1alpha1.Etcd) condition.Result {
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
	id     *string
	name   string
	role   *druidv1alpha1.EtcdRole
	status druidv1alpha1.EtcdMemberConditionStatus
	reason string
}

func (r *etcdMemberResult) ID() *string {
	return r.id
}

func (r *etcdMemberResult) Name() string {
	return r.name
}

func (r *etcdMemberResult) Role() *druidv1alpha1.EtcdRole {
	return r.role
}

func (r *etcdMemberResult) Status() druidv1alpha1.EtcdMemberConditionStatus {
	return r.status
}

func (r *etcdMemberResult) Reason() string {
	return r.reason
}

type etcdMemberTestChecker struct {
	results []etcdMemberResult
}

func (t *etcdMemberTestChecker) Check(_ context.Context, _ druidv1alpha1.Etcd) []etcdmember.Result {
	var results []etcdmember.Result
	for _, r := range t.results {
		result := r
		results = append(results, &result)
	}

	return results
}

func createEtcdMemberCheck(results ...etcdMemberResult) etcdmember.Checker {
	return &etcdMemberTestChecker{
		results: results,
	}
}
