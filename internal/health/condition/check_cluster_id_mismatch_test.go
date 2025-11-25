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

var _ = Describe("ClusterIDMismatchCheck", func() {
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
			member1Name                = fmt.Sprintf("%s-%d", etcdName, 0)
			member2Name                = fmt.Sprintf("%s-%d", etcdName, 1)
			member3Name                = fmt.Sprintf("%s-%d", etcdName, 2)
			readyMember, unknownMember druidv1alpha1.EtcdMemberStatus
			ctx                        = context.Background()
		)

		BeforeEach(func() {
			readyMember = druidv1alpha1.EtcdMemberStatus{
				Status: druidv1alpha1.EtcdMemberStatusReady,
			}
			unknownMember = druidv1alpha1.EtcdMemberStatus{
				Status: druidv1alpha1.EtcdMemberStatusUnknown,
			}
		})

		Context("when members in status", func() {
			It("should return that the cluster has no cluster ID mismatch (all members ready)", func() {
				etcd := druidv1alpha1.Etcd{
					ObjectMeta: metav1.ObjectMeta{
						Name:      etcdName,
						Namespace: etcdNamespace,
					},
					Spec: druidv1alpha1.EtcdSpec{Replicas: 3},
					Status: druidv1alpha1.EtcdStatus{
						Members: []druidv1alpha1.EtcdMemberStatus{
							memberStatusWithName(readyMember, member1Name),
							memberStatusWithName(readyMember, member2Name),
							memberStatusWithName(readyMember, member3Name),
						},
					},
				}
				member1Lease := createMemberLease(member1Name, etcdNamespace, ptr.To(fmt.Sprintf("%s:%s:%s", member1ID, clusterID, druidv1alpha1.EtcdRoleLeader)))
				member2Lease := createMemberLease(member2Name, etcdNamespace, ptr.To(fmt.Sprintf("%s:%s:%s", member2ID, clusterID, druidv1alpha1.EtcdRoleMember)))
				member3Lease := createMemberLease(member3Name, etcdNamespace, ptr.To(fmt.Sprintf("%s:%s:%s", member3ID, clusterID, druidv1alpha1.EtcdRoleMember)))
				existingObjects := mapToClientObjects([]*coordinationv1.Lease{member1Lease, member2Lease, member3Lease})
				cl := testutils.CreateTestFakeClientForObjects(nil, nil, nil, nil, existingObjects)
				check := ClusterIDMismatchCheck(cl)

				result := check.Check(ctx, etcd)

				Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeClusterIDMismatch))
				Expect(result.Status()).To(Equal(druidv1alpha1.ConditionFalse))
			})

			It("should return that the cluster has no cluster ID mismatch (members are partly unknown)", func() {
				etcd := druidv1alpha1.Etcd{
					ObjectMeta: metav1.ObjectMeta{
						Name:      etcdName,
						Namespace: etcdNamespace,
					},
					Spec: druidv1alpha1.EtcdSpec{Replicas: 3},
					Status: druidv1alpha1.EtcdStatus{
						Members: []druidv1alpha1.EtcdMemberStatus{
							memberStatusWithName(readyMember, member1Name),
							memberStatusWithName(unknownMember, member2Name),
							memberStatusWithName(unknownMember, member3Name),
						},
					},
				}
				member1Lease := createMemberLease(member1Name, etcdNamespace, ptr.To(fmt.Sprintf("%s:%s:%s", member1ID, clusterID, druidv1alpha1.EtcdRoleLeader)))
				member2Lease := createMemberLease(member2Name, etcdNamespace, ptr.To(fmt.Sprintf("%s:%s:%s", member2ID, clusterID, druidv1alpha1.EtcdRoleMember)))
				member3Lease := createMemberLease(member3Name, etcdNamespace, ptr.To(fmt.Sprintf("%s:%s:%s", member3ID, clusterID, druidv1alpha1.EtcdRoleMember)))
				existingObjects := mapToClientObjects([]*coordinationv1.Lease{member1Lease, member2Lease, member3Lease})
				cl := testutils.CreateTestFakeClientForObjects(nil, nil, nil, nil, existingObjects)
				check := ClusterIDMismatchCheck(cl)

				result := check.Check(ctx, etcd)

				Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeClusterIDMismatch))
				Expect(result.Status()).To(Equal(druidv1alpha1.ConditionFalse))
			})

			It("should return that the cluster ID mismatch status is unknown (with old lease format from backup-restore)", func() {
				etcd := druidv1alpha1.Etcd{
					ObjectMeta: metav1.ObjectMeta{
						Name:      etcdName,
						Namespace: etcdNamespace,
					},
					Spec: druidv1alpha1.EtcdSpec{Replicas: 3},
					Status: druidv1alpha1.EtcdStatus{
						Members: []druidv1alpha1.EtcdMemberStatus{
							memberStatusWithName(readyMember, member1Name),
							memberStatusWithName(readyMember, member2Name),
							memberStatusWithName(readyMember, member3Name),
						},
					},
				}
				// Note that the cluster ID is not part of the HolderIdentity here, which is the old format used by backup-restore
				member1Lease := createMemberLease(member1Name, etcdNamespace, ptr.To(fmt.Sprintf("%s:%s", member1ID, druidv1alpha1.EtcdRoleLeader)))
				member2Lease := createMemberLease(member2Name, etcdNamespace, ptr.To(fmt.Sprintf("%s:%s", member2ID, druidv1alpha1.EtcdRoleMember)))
				member3Lease := createMemberLease(member3Name, etcdNamespace, ptr.To(fmt.Sprintf("%s:%s", member3ID, druidv1alpha1.EtcdRoleMember)))
				existingObjects := mapToClientObjects([]*coordinationv1.Lease{member1Lease, member2Lease, member3Lease})
				cl := testutils.CreateTestFakeClientForObjects(nil, nil, nil, nil, existingObjects)
				check := ClusterIDMismatchCheck(cl)

				result := check.Check(ctx, etcd)

				Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeClusterIDMismatch))
				Expect(result.Status()).To(Equal(druidv1alpha1.ConditionUnknown))
				Expect(result.Reason()).To(Equal("NoClusterIDDetected"))
			})

			It("should return that the cluster has cluster ID mismatch - all members ready, but with with more than one cluster ID", func() {
				etcd := druidv1alpha1.Etcd{
					ObjectMeta: metav1.ObjectMeta{
						Name:      etcdName,
						Namespace: etcdNamespace,
					},
					Spec: druidv1alpha1.EtcdSpec{Replicas: 3},
					Status: druidv1alpha1.EtcdStatus{
						Members: []druidv1alpha1.EtcdMemberStatus{
							memberStatusWithName(readyMember, member1Name),
							memberStatusWithName(readyMember, member2Name),
							memberStatusWithName(readyMember, member3Name),
						},
					},
				}
				member1Lease := createMemberLease(member1Name, etcdNamespace, ptr.To(fmt.Sprintf("%s:%s:%s", member1ID, clusterID, druidv1alpha1.EtcdRoleLeader)))
				member2Lease := createMemberLease(member2Name, etcdNamespace, ptr.To(fmt.Sprintf("%s:%s:%s", member2ID, clusterID, druidv1alpha1.EtcdRoleMember)))
				member3Lease := createMemberLease(member3Name, etcdNamespace, ptr.To(fmt.Sprintf("%s:%s:%s", member3ID, newClusterID, druidv1alpha1.EtcdRoleLeader)))
				existingObjects := mapToClientObjects([]*coordinationv1.Lease{member1Lease, member2Lease, member3Lease})
				cl := testutils.CreateTestFakeClientForObjects(nil, nil, nil, nil, existingObjects)
				check := ClusterIDMismatchCheck(cl)

				result := check.Check(ctx, etcd)

				Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeClusterIDMismatch))
				Expect(result.Status()).To(Equal(druidv1alpha1.ConditionTrue))
				Expect(result.Reason()).To(Equal("MultipleClusterIDsDetected"))
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

func memberStatusWithName(memberStatus druidv1alpha1.EtcdMemberStatus, name string) druidv1alpha1.EtcdMemberStatus {
	memberStatus.Name = name
	return memberStatus
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
