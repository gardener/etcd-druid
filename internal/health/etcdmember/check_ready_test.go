// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package etcdmember_test

import (
	"context"
	"fmt"
	"time"

	"github.com/gardener/etcd-druid/internal/utils"

	testutils "github.com/gardener/etcd-druid/test/utils"
	"github.com/go-logr/logr"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	. "github.com/gardener/etcd-druid/internal/health/etcdmember"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"

	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	leaseDurationSeconds = 40
	unknownThreshold     = 300 * time.Second
	notReadyThreshold    = 60 * time.Second
)

var _ = Describe("ReadyCheck", func() {
	Describe("#Check", func() {
		const (
			etcdName      = "etcd"
			etcdNamespace = "etcd-test"
			member1ID     = "1"
			member2ID     = "2"
			member3ID     = "3"
		)
		var (
			ctx         context.Context
			check       Checker
			member1Name = fmt.Sprintf("%s-%d", etcdName, 0) // used for both lease and pod
			member2Name = fmt.Sprintf("%s-%d", etcdName, 1) // used for both lease and pod
			member3Name = fmt.Sprintf("%s-%d", etcdName, 2) // used for both lease and pod
		)

		BeforeEach(func() {
			ctx = context.Background()
		})

		Context("single node etcd: when just expired", func() {
			It("should set the affected condition to UNKNOWN because lease is lost", func() {
				now := time.Now()
				lease := createMemberLease(member1Name, etcdNamespace, pointer.String(fmt.Sprintf("%s:%s", member1ID, druidv1alpha1.EtcdRoleLeader)), utils.PointerOf(now.Add(-1*unknownThreshold).Add(-1*time.Second)))
				pod := createMemberPod(member1Name, etcdNamespace, true)
				existingObjects := mapToClientObjects([]*coordinationv1.Lease{lease}, []*corev1.Pod{pod})
				cl := testutils.CreateTestFakeClientForObjects(nil, nil, nil, nil, existingObjects)
				check = ReadyCheck(cl, logr.Discard(), notReadyThreshold, unknownThreshold)
				etcd := testutils.EtcdBuilderWithDefaults(etcdName, etcdNamespace).WithReplicas(1).Build()
				results := check.Check(ctx, *etcd)

				Expect(results).To(HaveLen(1))
				Expect(results[0].Status()).To(Equal(druidv1alpha1.EtcdMemberStatusUnknown))
				Expect(results[0].ID()).To(PointTo(Equal(member1ID)))
				Expect(results[0].Role()).To(PointTo(Equal(druidv1alpha1.EtcdRoleLeader)))
			})

			It("should set the affected condition to FAILED because containers are not ready", func() {
				now := time.Now()
				pod := createMemberPod(member1Name, etcdNamespace, false)
				lease := createMemberLease(member1Name, etcdNamespace, pointer.String(fmt.Sprintf("%s:%s", member1ID, druidv1alpha1.EtcdRoleLeader)), utils.PointerOf(now.Add(-1*unknownThreshold).Add(-1*time.Second)))
				existingObjects := mapToClientObjects([]*coordinationv1.Lease{lease}, []*corev1.Pod{pod})
				cl := testutils.CreateTestFakeClientForObjects(nil, nil, nil, nil, existingObjects)
				check = ReadyCheck(cl, logr.Discard(), notReadyThreshold, unknownThreshold)
				etcd := testutils.EtcdBuilderWithDefaults(etcdName, etcdNamespace).WithReplicas(1).Build()
				results := check.Check(ctx, *etcd)

				Expect(results).To(HaveLen(1))
				Expect(results[0].Status()).To(Equal(druidv1alpha1.EtcdMemberStatusNotReady))
				Expect(results[0].ID()).To(PointTo(Equal(member1ID)))
				Expect(results[0].Role()).To(PointTo(Equal(druidv1alpha1.EtcdRoleLeader)))
			})

			It("should set the affected condition to FAILED because Pod is not found", func() {
				now := time.Now()
				lease := createMemberLease(member1Name, etcdNamespace, pointer.String(fmt.Sprintf("%s:%s", member1ID, druidv1alpha1.EtcdRoleLeader)), utils.PointerOf(now.Add(-1*unknownThreshold).Add(-1*time.Second)))
				existingObjects := mapToClientObjects([]*coordinationv1.Lease{lease}, nil)
				cl := testutils.CreateTestFakeClientForObjects(nil, nil, nil, nil, existingObjects)
				check = ReadyCheck(cl, logr.Discard(), notReadyThreshold, unknownThreshold)
				etcd := testutils.EtcdBuilderWithDefaults(etcdName, etcdNamespace).WithReplicas(1).Build()
				results := check.Check(ctx, *etcd)

				Expect(results).To(HaveLen(1))
				Expect(results[0].Status()).To(Equal(druidv1alpha1.EtcdMemberStatusNotReady))
				Expect(results[0].ID()).To(PointTo(Equal(member1ID)))
				Expect(results[0].Role()).To(PointTo(Equal(druidv1alpha1.EtcdRoleLeader)))
			})

			It("should set the affected condition to FAILED because Pod retrieval errors out", func() {
				now := time.Now()
				lease := createMemberLease(member1Name, etcdNamespace, pointer.String(fmt.Sprintf("%s:%s", member1ID, druidv1alpha1.EtcdRoleLeader)), utils.PointerOf(now.Add(-1*unknownThreshold).Add(-1*time.Second)))
				existingObjects := mapToClientObjects([]*coordinationv1.Lease{lease}, nil)
				cl := testutils.CreateTestFakeClientForObjects(testutils.TestAPIInternalErr, nil, nil, nil, existingObjects)
				check = ReadyCheck(cl, logr.Discard(), notReadyThreshold, unknownThreshold)
				etcd := testutils.EtcdBuilderWithDefaults(etcdName, etcdNamespace).WithReplicas(1).Build()
				results := check.Check(ctx, *etcd)

				Expect(results).To(HaveLen(1))
				Expect(results[0].Status()).To(Equal(druidv1alpha1.EtcdMemberStatusNotReady))
				Expect(results[0].ID()).To(PointTo(Equal(member1ID)))
				Expect(results[0].Role()).To(PointTo(Equal(druidv1alpha1.EtcdRoleLeader)))
			})
		})

		Context("multi node etcd: when expired a while ago", func() {

			It("should set the affected condition to FAILED because status was Unknown for a while", func() {
				now := time.Now()
				shortExpirationTime := now.Add(-1 * unknownThreshold).Add(-1 * time.Second)
				longExpirationTime := now.Add(-1 * unknownThreshold).Add(-1 * time.Second).Add(-1 * notReadyThreshold)
				member1Lease := createMemberLease(member1Name, etcdNamespace, pointer.String(fmt.Sprintf("%s:%s", member1ID, druidv1alpha1.EtcdRoleLeader)), utils.PointerOf(shortExpirationTime))
				member2Lease := createMemberLease(member2Name, etcdNamespace, pointer.String(fmt.Sprintf("%s:%s", member2ID, druidv1alpha1.EtcdRoleMember)), utils.PointerOf(longExpirationTime))
				member3Lease := createMemberLease(member3Name, etcdNamespace, pointer.String(fmt.Sprintf("%s:%s", member3ID, druidv1alpha1.EtcdRoleMember)), utils.PointerOf(shortExpirationTime))
				member1Pod := createMemberPod(member1Name, etcdNamespace, true)
				member2Pod := createMemberPod(member2Name, etcdNamespace, false)
				existingObjects := mapToClientObjects([]*coordinationv1.Lease{member1Lease, member2Lease, member3Lease}, []*corev1.Pod{member1Pod, member2Pod})
				cl := testutils.CreateTestFakeClientForObjects(nil, nil, nil, nil, existingObjects)
				check = ReadyCheck(cl, logr.Discard(), notReadyThreshold, unknownThreshold)
				etcd := testutils.EtcdBuilderWithDefaults(etcdName, etcdNamespace).WithReplicas(3).Build()
				results := check.Check(ctx, *etcd)

				Expect(results).To(HaveLen(3))

				Expect(results[0].ID()).To(PointTo(Equal(member1ID)))
				Expect(results[0].Status()).To(Equal(druidv1alpha1.EtcdMemberStatusUnknown))
				Expect(results[0].Role()).To(PointTo(Equal(druidv1alpha1.EtcdRoleLeader)))
				Expect(results[0].Reason()).To(Equal("LeaseExpired"))

				Expect(results[1].ID()).To(PointTo(Equal(member2ID)))
				Expect(results[1].Status()).To(Equal(druidv1alpha1.EtcdMemberStatusNotReady))
				Expect(results[1].Role()).To(PointTo(Equal(druidv1alpha1.EtcdRoleMember)))
				Expect(results[1].Reason()).To(Equal("UnknownGracePeriodExceeded"))

				Expect(results[2].ID()).To(PointTo(Equal(member3ID)))
				Expect(results[2].Status()).To(Equal(druidv1alpha1.EtcdMemberStatusNotReady))
				Expect(results[2].Role()).To(PointTo(Equal(druidv1alpha1.EtcdRoleMember)))
				Expect(results[2].Reason()).To(Equal("ContainersNotReady"))
			})

		})

		Context("multi node etcd: when leases are up-to-date", func() {

			It("should set member ready", func() {
				now := time.Now()
				renewTime := now.Add(-1 * 20 * time.Second)
				member1Lease := createMemberLease(member1Name, etcdNamespace, pointer.String(fmt.Sprintf("%s:%s", member1ID, druidv1alpha1.EtcdRoleLeader)), utils.PointerOf(renewTime))
				member2Lease := createMemberLease(member2Name, etcdNamespace, pointer.String(fmt.Sprintf("%s:%s", member2ID, druidv1alpha1.EtcdRoleMember)), utils.PointerOf(renewTime))
				member3Lease := createMemberLease(member3Name, etcdNamespace, pointer.String(fmt.Sprintf("%s:%s", member3ID, druidv1alpha1.EtcdRoleMember)), utils.PointerOf(renewTime))

				existingObjects := mapToClientObjects([]*coordinationv1.Lease{member1Lease, member2Lease, member3Lease}, nil)
				cl := testutils.CreateTestFakeClientForObjects(nil, nil, nil, nil, existingObjects)
				check = ReadyCheck(cl, logr.Discard(), notReadyThreshold, unknownThreshold)
				etcd := testutils.EtcdBuilderWithDefaults(etcdName, etcdNamespace).WithReplicas(3).Build()
				results := check.Check(ctx, *etcd)

				Expect(results).To(HaveLen(3))

				Expect(results[0].ID()).To(PointTo(Equal(member1ID)))
				Expect(results[0].Status()).To(Equal(druidv1alpha1.EtcdMemberStatusReady))
				Expect(results[0].Role()).To(PointTo(Equal(druidv1alpha1.EtcdRoleLeader)))
				Expect(results[0].Reason()).To(Equal("LeaseSucceeded"))

				Expect(results[1].ID()).To(PointTo(Equal(member2ID)))
				Expect(results[1].Status()).To(Equal(druidv1alpha1.EtcdMemberStatusReady))
				Expect(results[1].Role()).To(PointTo(Equal(druidv1alpha1.EtcdRoleMember)))
				Expect(results[1].Reason()).To(Equal("LeaseSucceeded"))

				Expect(results[2].ID()).To(PointTo(Equal(member3ID)))
				Expect(results[2].Status()).To(Equal(druidv1alpha1.EtcdMemberStatusReady))
				Expect(results[2].Role()).To(PointTo(Equal(druidv1alpha1.EtcdRoleMember)))
				Expect(results[2].Reason()).To(Equal("LeaseSucceeded"))
			})
		})

		Context("multi node etcd: when lease has not been acquired", func() {

			It("should only contain members which acquired lease once", func() {
				now := time.Now()
				renewTime := now.Add(-1 * 20 * time.Second)
				shortExpirationTime := now.Add(-1 * unknownThreshold).Add(-1 * time.Second)

				member1Lease := createMemberLease(member1Name, etcdNamespace, pointer.String(fmt.Sprintf("%s:%s", member1ID, druidv1alpha1.EtcdRoleLeader)), utils.PointerOf(renewTime))
				member2Lease := createMemberLease(member2Name, etcdNamespace, pointer.String(fmt.Sprintf("%s:%s", member2ID, druidv1alpha1.EtcdRoleMember)), utils.PointerOf(shortExpirationTime))
				member3Lease := createMemberLease(member3Name, etcdNamespace, pointer.String(fmt.Sprintf("%s:%s", member3ID, druidv1alpha1.EtcdRoleMember)), nil)
				existingObjects := mapToClientObjects([]*coordinationv1.Lease{member1Lease, member2Lease, member3Lease}, nil)
				cl := testutils.CreateTestFakeClientForObjects(nil, nil, nil, nil, existingObjects)
				check = ReadyCheck(cl, logr.Discard(), notReadyThreshold, unknownThreshold)
				etcd := testutils.EtcdBuilderWithDefaults(etcdName, etcdNamespace).WithReplicas(3).Build()
				results := check.Check(ctx, *etcd)

				Expect(results).To(HaveLen(2))

				Expect(results[0].ID()).To(PointTo(Equal(member1ID)))
				Expect(results[0].Status()).To(Equal(druidv1alpha1.EtcdMemberStatusReady))
				Expect(results[0].Role()).To(PointTo(Equal(druidv1alpha1.EtcdRoleLeader)))
				Expect(results[0].Reason()).To(Equal("LeaseSucceeded"))

				Expect(results[1].ID()).To(PointTo(Equal(member2ID)))
				Expect(results[1].Status()).To(Equal(druidv1alpha1.EtcdMemberStatusNotReady))
				Expect(results[1].Role()).To(PointTo(Equal(druidv1alpha1.EtcdRoleMember)))
				Expect(results[1].Reason()).To(Equal("ContainersNotReady"))

			})
		})
	})
})

func mapToClientObjects(leases []*coordinationv1.Lease, pods []*corev1.Pod) []client.Object {
	objects := make([]client.Object, 0, len(leases)+len(pods))
	for _, lease := range leases {
		objects = append(objects, lease)
	}
	for _, pod := range pods {
		objects = append(objects, pod)
	}
	return objects
}

func createMemberLease(name, namespace string, holderIdentity *string, renewTime *time.Time) *coordinationv1.Lease {
	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: coordinationv1.LeaseSpec{
			HolderIdentity:       holderIdentity,
			LeaseDurationSeconds: pointer.Int32(leaseDurationSeconds),
		},
	}
	if renewTime != nil {
		lease.Spec.RenewTime = &metav1.MicroTime{Time: *renewTime}
	}
	return lease
}

func createMemberPod(name, namespace string, ready bool) *corev1.Pod {
	var readyCondition corev1.ConditionStatus
	if ready {
		readyCondition = corev1.ConditionTrue
	} else {
		readyCondition = corev1.ConditionFalse
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "etcd",
					Image: "etcd:3.4.26",
				},
			},
		},
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{
				{
					Type:   corev1.ContainersReady,
					Status: readyCondition,
				},
			},
		},
	}
}
