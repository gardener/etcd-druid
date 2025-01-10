// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package condition_test

import (
	"context"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"

	. "github.com/gardener/etcd-druid/internal/health/condition"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("AllMembersReadyCheck", func() {
	Describe("#Check", func() {
		var readyMember, notReadyMember druidv1alpha1.EtcdMemberStatus

		BeforeEach(func() {
			readyMember = druidv1alpha1.EtcdMemberStatus{
				Status: druidv1alpha1.EtcdMemberStatusReady,
			}
			notReadyMember = druidv1alpha1.EtcdMemberStatus{
				Status: druidv1alpha1.EtcdMemberStatusNotReady,
			}
		})

		Context("when members in status", func() {
			It("should return that all members are ready", func() {
				etcd := druidv1alpha1.Etcd{
					Spec: druidv1alpha1.EtcdSpec{
						Replicas: 3,
					},
					Status: druidv1alpha1.EtcdStatus{
						Members: []druidv1alpha1.EtcdMemberStatus{
							readyMember,
							readyMember,
							readyMember,
						},
					},
				}
				check := AllMembersReadyCheck(nil)

				result := check.Check(context.TODO(), etcd)

				Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeAllMembersReady))
				Expect(result.Status()).To(Equal(druidv1alpha1.ConditionTrue))
			})

			It("should return that members are not ready", func() {
				etcd := druidv1alpha1.Etcd{
					Spec: druidv1alpha1.EtcdSpec{
						Replicas: 3,
					},
					Status: druidv1alpha1.EtcdStatus{
						Members: []druidv1alpha1.EtcdMemberStatus{
							readyMember,
							notReadyMember,
							readyMember,
						},
					},
				}
				check := AllMembersReadyCheck(nil)

				result := check.Check(context.TODO(), etcd)

				Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeAllMembersReady))
				Expect(result.Status()).To(Equal(druidv1alpha1.ConditionFalse))
			})

			It("should return all members are not ready when number of members registered are less than spec replicas", func() {
				etcd := druidv1alpha1.Etcd{
					Spec: druidv1alpha1.EtcdSpec{
						Replicas: 3,
					},
					Status: druidv1alpha1.EtcdStatus{
						Members: []druidv1alpha1.EtcdMemberStatus{
							readyMember,
							readyMember,
						},
					},
				}
				check := AllMembersReadyCheck(nil)

				result := check.Check(context.TODO(), etcd)

				Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeAllMembersReady))
				Expect(result.Status()).To(Equal(druidv1alpha1.ConditionFalse))
				Expect(result.Reason()).To(Equal("NotAllMembersReady"))
			})
		})

		Context("when no members in status", func() {
			It("should return that readiness is unknown", func() {
				etcd := druidv1alpha1.Etcd{
					Spec: druidv1alpha1.EtcdSpec{
						Replicas: 3,
					},
					Status: druidv1alpha1.EtcdStatus{
						Members: []druidv1alpha1.EtcdMemberStatus{},
					},
				}
				check := AllMembersReadyCheck(nil)

				result := check.Check(context.TODO(), etcd)

				Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeAllMembersReady))
				Expect(result.Status()).To(Equal(druidv1alpha1.ConditionUnknown))
			})
		})
	})
})
