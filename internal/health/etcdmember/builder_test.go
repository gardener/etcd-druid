// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package etcdmember_test

import (
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	. "github.com/gardener/etcd-druid/internal/health/etcdmember"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
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
						ID:                 ptr.To("1"),
						Status:             druidv1alpha1.EtcdMemberStatusReady,
						Reason:             "foo reason",
						LastTransitionTime: metav1.NewTime(now.Add(-12 * time.Hour)),
					},
					"2": {
						Name:               "member2",
						ID:                 ptr.To("2"),
						Status:             druidv1alpha1.EtcdMemberStatusReady,
						Reason:             "bar reason",
						LastTransitionTime: metav1.NewTime(now.Add(-6 * time.Hour)),
					},
					"3": {
						Name:               "member3",
						ID:                 ptr.To("3"),
						Status:             druidv1alpha1.EtcdMemberStatusReady,
						Reason:             "foobar reason",
						LastTransitionTime: metav1.NewTime(now.Add(-18 * time.Hour)),
					},
				}

				builder.WithOldMembers([]druidv1alpha1.EtcdMemberStatus{
					oldMembers["1"],
					oldMembers["2"],
					oldMembers["3"],
				})
			})

			It("should correctly set the LastTransitionTime", func() {
				builder.WithResults([]Result{
					&result{
						MemberID:     ptr.To("3"),
						MemberName:   "member3",
						MemberStatus: druidv1alpha1.EtcdMemberStatusUnknown,
						MemberReason: "unknown reason",
					},
				})

				conditions := builder.Build()

				Expect(conditions).To(ConsistOf(
					MatchFields(IgnoreExtras, Fields{
						"Name":               Equal("member3"),
						"ID":                 PointTo(Equal("3")),
						"Status":             Equal(druidv1alpha1.EtcdMemberStatusUnknown),
						"Reason":             Equal("unknown reason"),
						"LastTransitionTime": Equal(metav1.NewTime(now)),
					}),
				))
			})
		})

		Context("when Builder has no old members", func() {
			var (
				memberRoleLeader, memberRoleMember druidv1alpha1.EtcdRole
			)

			BeforeEach(func() {
				memberRoleLeader = druidv1alpha1.EtcdRoleLeader
				memberRoleMember = druidv1alpha1.EtcdRoleMember
			})

			It("should not add any members but sort them", func() {
				builder.WithResults([]Result{
					&result{
						MemberID:     ptr.To("2"),
						MemberName:   "member2",
						MemberRole:   &memberRoleMember,
						MemberStatus: druidv1alpha1.EtcdMemberStatusReady,
						MemberReason: "foo reason",
					},
					&result{
						MemberID:     ptr.To("1"),
						MemberName:   "member1",
						MemberRole:   &memberRoleLeader,
						MemberStatus: druidv1alpha1.EtcdMemberStatusUnknown,
						MemberReason: "unknown reason",
					},
				})

				conditions := builder.Build()

				Expect(conditions).To(HaveLen(2))
				Expect(conditions[0]).To(MatchFields(IgnoreExtras, Fields{
					"Name":               Equal("member1"),
					"ID":                 PointTo(Equal("1")),
					"Role":               PointTo(Equal(druidv1alpha1.EtcdRoleLeader)),
					"Status":             Equal(druidv1alpha1.EtcdMemberStatusUnknown),
					"Reason":             Equal("unknown reason"),
					"LastTransitionTime": Equal(metav1.NewTime(now)),
				}))
				Expect(conditions[1]).To(MatchFields(IgnoreExtras, Fields{
					"Name":               Equal("member2"),
					"ID":                 PointTo(Equal("2")),
					"Role":               PointTo(Equal(druidv1alpha1.EtcdRoleMember)),
					"Status":             Equal(druidv1alpha1.EtcdMemberStatusReady),
					"Reason":             Equal("foo reason"),
					"LastTransitionTime": Equal(metav1.NewTime(now)),
				}))
			})
		})
	})
})

type result struct {
	MemberID     *string
	MemberName   string
	MemberRole   *druidv1alpha1.EtcdRole
	MemberStatus druidv1alpha1.EtcdMemberConditionStatus
	MemberReason string
}

func (r *result) ID() *string {
	return r.MemberID
}

func (r *result) Name() string {
	return r.MemberName
}

func (r *result) Role() *druidv1alpha1.EtcdRole {
	return r.MemberRole
}

func (r *result) Reason() string {
	return r.MemberReason
}

func (r *result) Status() druidv1alpha1.EtcdMemberConditionStatus {
	return r.MemberStatus
}
