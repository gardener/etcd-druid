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

var _ = Describe("ReadyCheck", func() {
	Describe("#Check", func() {
		var readyMember, notReadyMember, unknownMember druidv1alpha1.EtcdMemberStatus

		BeforeEach(func() {
			readyMember = druidv1alpha1.EtcdMemberStatus{
				Status: druidv1alpha1.EtcdMemberStatusReady,
			}
			notReadyMember = druidv1alpha1.EtcdMemberStatus{
				Status: druidv1alpha1.EtcdMemberStatusNotReady,
			}
			unknownMember = druidv1alpha1.EtcdMemberStatus{
				Status: druidv1alpha1.EtcdMemberStatusUnknown,
			}
		})

		Context("when members in status", func() {
			It("should return that the cluster has a quorum (all members ready)", func() {
				etcd := druidv1alpha1.Etcd{
					Spec: druidv1alpha1.EtcdSpec{Replicas: 3},
					Status: druidv1alpha1.EtcdStatus{
						Members: []druidv1alpha1.EtcdMemberStatus{
							readyMember,
							readyMember,
							readyMember,
						},
					},
				}
				check := ReadyCheck(nil)

				result := check.Check(context.TODO(), etcd)

				Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeReady))
				Expect(result.Status()).To(Equal(druidv1alpha1.ConditionTrue))
			})

			It("should return that the cluster has a quorum (members are partly unknown)", func() {
				etcd := druidv1alpha1.Etcd{
					Spec: druidv1alpha1.EtcdSpec{Replicas: 3},
					Status: druidv1alpha1.EtcdStatus{
						Members: []druidv1alpha1.EtcdMemberStatus{
							readyMember,
							unknownMember,
							unknownMember,
						},
					},
				}
				check := ReadyCheck(nil)

				result := check.Check(context.TODO(), etcd)

				Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeReady))
				Expect(result.Status()).To(Equal(druidv1alpha1.ConditionTrue))
			})

			It("should return that the cluster has a quorum (one member not ready)", func() {
				etcd := druidv1alpha1.Etcd{
					Spec: druidv1alpha1.EtcdSpec{Replicas: 3},
					Status: druidv1alpha1.EtcdStatus{
						Members: []druidv1alpha1.EtcdMemberStatus{
							readyMember,
							notReadyMember,
							readyMember,
						},
					},
				}
				check := ReadyCheck(nil)

				result := check.Check(context.TODO(), etcd)

				Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeReady))
				Expect(result.Status()).To(Equal(druidv1alpha1.ConditionTrue))
			})

			It("should return that the cluster has lost its quorum", func() {
				etcd := druidv1alpha1.Etcd{
					Spec: druidv1alpha1.EtcdSpec{Replicas: 3},
					Status: druidv1alpha1.EtcdStatus{
						Members: []druidv1alpha1.EtcdMemberStatus{
							readyMember,
							notReadyMember,
							notReadyMember,
						},
					},
				}
				check := ReadyCheck(nil)

				result := check.Check(context.TODO(), etcd)

				Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeReady))
				Expect(result.Status()).To(Equal(druidv1alpha1.ConditionFalse))
				Expect(result.Reason()).To(Equal("QuorumLost"))
			})
		})

		Context("when no members in status", func() {
			It("should return that quorum is unknown", func() {
				etcd := druidv1alpha1.Etcd{
					Spec: druidv1alpha1.EtcdSpec{Replicas: 3},
					Status: druidv1alpha1.EtcdStatus{
						Members: []druidv1alpha1.EtcdMemberStatus{},
					},
				}
				check := ReadyCheck(nil)

				result := check.Check(context.TODO(), etcd)

				Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeReady))
				Expect(result.Status()).To(Equal(druidv1alpha1.ConditionUnknown))
				Expect(result.Reason()).To(Equal("NoMembersInStatus"))
			})
		})
	})

})
