// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package condition_test

import (
	"context"
	"fmt"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	testutils "github.com/gardener/etcd-druid/test/utils"

	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/gardener/etcd-druid/internal/health/condition"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ReadyCheck", func() {
	Describe("#Check", func() {
		const (
			etcdName      = "etcd"
			etcdNamespace = "etcd-test"
			clusterID     = "0"
			newClusterID  = "1"
			member1ID     = "1"
			member2ID     = "2"
			member3ID     = "3"
		)
		var (
			member1Name                                = fmt.Sprintf("%s-%d", etcdName, 0)
			member2Name                                = fmt.Sprintf("%s-%d", etcdName, 1)
			member3Name                                = fmt.Sprintf("%s-%d", etcdName, 2)
			readyMember, notReadyMember, unknownMember druidv1alpha1.EtcdMemberStatus
		)

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
					ObjectMeta: metav1.ObjectMeta{
						Name:      etcdName,
						Namespace: etcdNamespace,
					},
					Spec: druidv1alpha1.EtcdSpec{Replicas: 3},
					Status: druidv1alpha1.EtcdStatus{
						Members: []druidv1alpha1.EtcdMemberStatus{
							readyMember,
							readyMember,
							readyMember,
						},
					},
				}
				member1Lease := createMemberLease(member1Name, etcdNamespace, ptr.To(fmt.Sprintf("%s:%s:%s", member1ID, clusterID, druidv1alpha1.EtcdRoleLeader)))
				member2Lease := createMemberLease(member2Name, etcdNamespace, ptr.To(fmt.Sprintf("%s:%s:%s", member2ID, clusterID, druidv1alpha1.EtcdRoleMember)))
				member3Lease := createMemberLease(member3Name, etcdNamespace, ptr.To(fmt.Sprintf("%s:%s:%s", member3ID, clusterID, druidv1alpha1.EtcdRoleMember)))
				existingObjects := mapToClientObjects([]*coordinationv1.Lease{member1Lease, member2Lease, member3Lease})
				cl := testutils.CreateTestFakeClientForObjects(nil, nil, nil, nil, existingObjects)
				check := ReadyCheck(cl)

				result := check.Check(context.TODO(), etcd)

				Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeReady))
				Expect(result.Status()).To(Equal(druidv1alpha1.ConditionTrue))
			})

			It("should return that the cluster has a quorum (members are partly unknown)", func() {
				etcd := druidv1alpha1.Etcd{
					ObjectMeta: metav1.ObjectMeta{
						Name:      etcdName,
						Namespace: etcdNamespace,
					},
					Spec: druidv1alpha1.EtcdSpec{Replicas: 3},
					Status: druidv1alpha1.EtcdStatus{
						Members: []druidv1alpha1.EtcdMemberStatus{
							readyMember,
							unknownMember,
							unknownMember,
						},
					},
				}
				member1Lease := createMemberLease(member1Name, etcdNamespace, ptr.To(fmt.Sprintf("%s:%s:%s", member1ID, clusterID, druidv1alpha1.EtcdRoleLeader)))
				member2Lease := createMemberLease(member2Name, etcdNamespace, ptr.To(fmt.Sprintf("%s:%s:%s", member2ID, clusterID, druidv1alpha1.EtcdRoleMember)))
				member3Lease := createMemberLease(member3Name, etcdNamespace, ptr.To(fmt.Sprintf("%s:%s:%s", member3ID, clusterID, druidv1alpha1.EtcdRoleMember)))
				existingObjects := mapToClientObjects([]*coordinationv1.Lease{member1Lease, member2Lease, member3Lease})
				cl := testutils.CreateTestFakeClientForObjects(nil, nil, nil, nil, existingObjects)
				check := ReadyCheck(cl)

				result := check.Check(context.TODO(), etcd)

				Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeReady))
				Expect(result.Status()).To(Equal(druidv1alpha1.ConditionTrue))
			})

			It("should return that the cluster has a quorum (all members ready, with old lease format from backup-restore)", func() {
				etcd := druidv1alpha1.Etcd{
					ObjectMeta: metav1.ObjectMeta{
						Name:      etcdName,
						Namespace: etcdNamespace,
					},
					Spec: druidv1alpha1.EtcdSpec{Replicas: 3},
					Status: druidv1alpha1.EtcdStatus{
						Members: []druidv1alpha1.EtcdMemberStatus{
							readyMember,
							readyMember,
							readyMember,
						},
					},
				}
				// Note that the cluster ID is not part of the HolderIdentity here, which is the old format used by backup-restore
				member1Lease := createMemberLease(member1Name, etcdNamespace, ptr.To(fmt.Sprintf("%s:%s", member1ID, druidv1alpha1.EtcdRoleLeader)))
				member2Lease := createMemberLease(member2Name, etcdNamespace, ptr.To(fmt.Sprintf("%s:%s", member2ID, druidv1alpha1.EtcdRoleMember)))
				member3Lease := createMemberLease(member3Name, etcdNamespace, ptr.To(fmt.Sprintf("%s:%s", member3ID, druidv1alpha1.EtcdRoleMember)))
				existingObjects := mapToClientObjects([]*coordinationv1.Lease{member1Lease, member2Lease, member3Lease})
				cl := testutils.CreateTestFakeClientForObjects(nil, nil, nil, nil, existingObjects)
				check := ReadyCheck(cl)

				result := check.Check(context.TODO(), etcd)

				Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeReady))
				Expect(result.Status()).To(Equal(druidv1alpha1.ConditionTrue))
			})

			It("should return that the cluster has a split-brain (there are 2 leaders simultaneously)", func() {
				etcd := druidv1alpha1.Etcd{
					ObjectMeta: metav1.ObjectMeta{
						Name:      etcdName,
						Namespace: etcdNamespace,
					},
					Spec: druidv1alpha1.EtcdSpec{Replicas: 3},
					Status: druidv1alpha1.EtcdStatus{
						Members: []druidv1alpha1.EtcdMemberStatus{
							readyMember,
							readyMember,
							readyMember,
						},
					},
				}
				member1Lease := createMemberLease(member1Name, etcdNamespace, ptr.To(fmt.Sprintf("%s:%s:%s", member1ID, clusterID, druidv1alpha1.EtcdRoleLeader)))
				member2Lease := createMemberLease(member2Name, etcdNamespace, ptr.To(fmt.Sprintf("%s:%s:%s", member2ID, clusterID, druidv1alpha1.EtcdRoleMember)))
				member3Lease := createMemberLease(member3Name, etcdNamespace, ptr.To(fmt.Sprintf("%s:%s:%s", member3ID, clusterID, druidv1alpha1.EtcdRoleLeader)))
				existingObjects := mapToClientObjects([]*coordinationv1.Lease{member1Lease, member2Lease, member3Lease})
				cl := testutils.CreateTestFakeClientForObjects(nil, nil, nil, nil, existingObjects)
				check := ReadyCheck(cl)

				result := check.Check(context.TODO(), etcd)

				Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeReady))
				Expect(result.Status()).To(Equal(druidv1alpha1.ConditionFalse))
				Expect(result.Reason()).To(Equal("SplitBrainDetected"))
			})

			It("should return that the cluster has a split-quorum (members are part of different clusters)", func() {
				etcd := druidv1alpha1.Etcd{
					ObjectMeta: metav1.ObjectMeta{
						Name:      etcdName,
						Namespace: etcdNamespace,
					},
					Spec: druidv1alpha1.EtcdSpec{Replicas: 3},
					Status: druidv1alpha1.EtcdStatus{
						Members: []druidv1alpha1.EtcdMemberStatus{
							readyMember,
							unknownMember,
							unknownMember,
						},
					},
				}
				member1Lease := createMemberLease(member1Name, etcdNamespace, ptr.To(fmt.Sprintf("%s:%s:%s", member1ID, clusterID, druidv1alpha1.EtcdRoleLeader)))
				member2Lease := createMemberLease(member2Name, etcdNamespace, ptr.To(fmt.Sprintf("%s:%s:%s", member2ID, clusterID, druidv1alpha1.EtcdRoleMember)))
				member3Lease := createMemberLease(member3Name, etcdNamespace, ptr.To(fmt.Sprintf("%s:%s:%s", member3ID, newClusterID, druidv1alpha1.EtcdRoleMember)))
				existingObjects := mapToClientObjects([]*coordinationv1.Lease{member1Lease, member2Lease, member3Lease})
				cl := testutils.CreateTestFakeClientForObjects(nil, nil, nil, nil, existingObjects)
				check := ReadyCheck(cl)

				result := check.Check(context.TODO(), etcd)

				Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeReady))
				Expect(result.Status()).To(Equal(druidv1alpha1.ConditionFalse))
				Expect(result.Reason()).To(Equal("SplitQuorumDetected"))
			})

			It("should return that the cluster has a quorum (one member not ready)", func() {
				etcd := druidv1alpha1.Etcd{
					ObjectMeta: metav1.ObjectMeta{
						Name:      etcdName,
						Namespace: etcdNamespace,
					},
					Spec: druidv1alpha1.EtcdSpec{Replicas: 3},
					Status: druidv1alpha1.EtcdStatus{
						Members: []druidv1alpha1.EtcdMemberStatus{
							readyMember,
							notReadyMember,
							readyMember,
						},
					},
				}
				member1Lease := createMemberLease(member1Name, etcdNamespace, ptr.To(fmt.Sprintf("%s:%s:%s", member1ID, clusterID, druidv1alpha1.EtcdRoleLeader)))
				member3Lease := createMemberLease(member3Name, etcdNamespace, ptr.To(fmt.Sprintf("%s:%s:%s", member3ID, clusterID, druidv1alpha1.EtcdRoleMember)))
				existingObjects := mapToClientObjects([]*coordinationv1.Lease{member1Lease, member3Lease})
				cl := testutils.CreateTestFakeClientForObjects(nil, nil, nil, nil, existingObjects)
				check := ReadyCheck(cl)

				result := check.Check(context.TODO(), etcd)

				Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeReady))
				Expect(result.Status()).To(Equal(druidv1alpha1.ConditionTrue))
			})

			It("should return that the cluster has lost its quorum", func() {
				etcd := druidv1alpha1.Etcd{
					ObjectMeta: metav1.ObjectMeta{
						Name:      etcdName,
						Namespace: etcdNamespace,
					},
					Spec: druidv1alpha1.EtcdSpec{Replicas: 3},
					Status: druidv1alpha1.EtcdStatus{
						Members: []druidv1alpha1.EtcdMemberStatus{
							readyMember,
							notReadyMember,
							notReadyMember,
						},
					},
				}
				member1Lease := createMemberLease(member1Name, etcdNamespace, ptr.To(fmt.Sprintf("%s:%s:%s", member1ID, clusterID, druidv1alpha1.EtcdRoleLeader)))
				existingObjects := mapToClientObjects([]*coordinationv1.Lease{member1Lease})
				cl := testutils.CreateTestFakeClientForObjects(nil, nil, nil, nil, existingObjects)
				check := ReadyCheck(cl)

				result := check.Check(context.TODO(), etcd)

				Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeReady))
				Expect(result.Status()).To(Equal(druidv1alpha1.ConditionFalse))
				Expect(result.Reason()).To(Equal("QuorumLost"))
			})
		})

		Context("when no members in status", func() {
			It("should return that quorum is unknown", func() {
				etcd := druidv1alpha1.Etcd{
					ObjectMeta: metav1.ObjectMeta{
						Name:      etcdName,
						Namespace: etcdNamespace,
					},
					Spec: druidv1alpha1.EtcdSpec{Replicas: 3},
					Status: druidv1alpha1.EtcdStatus{
						Members: []druidv1alpha1.EtcdMemberStatus{},
					},
				}
				cl := testutils.CreateTestFakeClientForObjects(nil, nil, nil, nil, nil)
				check := ReadyCheck(cl)

				result := check.Check(context.TODO(), etcd)

				Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeReady))
				Expect(result.Status()).To(Equal(druidv1alpha1.ConditionUnknown))
				Expect(result.Reason()).To(Equal("NoMembersInStatus"))
			})
		})
	})

})

func mapToClientObjects(leases []*coordinationv1.Lease) []client.Object {
	objects := make([]client.Object, 0, len(leases))
	for _, lease := range leases {
		objects = append(objects, lease)
	}
	return objects
}

func createMemberLease(name, namespace string, holderIdentity *string) *coordinationv1.Lease {
	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: coordinationv1.LeaseSpec{
			HolderIdentity: holderIdentity,
		},
	}
	return lease
}
