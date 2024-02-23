// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package condition_test

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	. "github.com/gardener/etcd-druid/pkg/health/condition"
	"github.com/go-logr/logr"
)

var _ = Describe("AllMembersReadyCheck", func() {
	Describe("#Check", func() {
		var (
			readyMember, notReadyMember druidv1alpha1.EtcdMemberStatus

			logger = logr.Discard()
		)

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
				check := AllMembersCheck(nil)

				result := check.Check(context.TODO(), logger, etcd)

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
				check := AllMembersCheck(nil)

				result := check.Check(context.TODO(), logger, etcd)

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
				check := AllMembersCheck(nil)

				result := check.Check(context.TODO(), logger, etcd)

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
				check := AllMembersCheck(nil)

				result := check.Check(context.TODO(), logger, etcd)

				Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeAllMembersReady))
				Expect(result.Status()).To(Equal(druidv1alpha1.ConditionUnknown))
			})
		})
	})
})
