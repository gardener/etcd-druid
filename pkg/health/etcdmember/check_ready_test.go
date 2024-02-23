// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package etcdmember_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	"github.com/go-logr/logr"

	kutil "github.com/gardener/gardener/pkg/utils/kubernetes"
	"github.com/gardener/gardener/pkg/utils/test"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gstruct"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/pkg/common"
	. "github.com/gardener/etcd-druid/pkg/health/etcdmember"
	mockclient "github.com/gardener/etcd-druid/pkg/mock/controller-runtime/client"
	"github.com/gardener/etcd-druid/pkg/utils"
)

var _ = Describe("ReadyCheck", func() {
	Describe("#Check", func() {
		var (
			ctx                                 context.Context
			mockCtrl                            *gomock.Controller
			cl                                  *mockclient.MockClient
			leaseDurationSeconds                *int32
			unknownThreshold, notReadyThreshold time.Duration
			now                                 time.Time
			check                               Checker
			logger                              logr.Logger

			member1Name string
			member1ID   *string
			etcd        druidv1alpha1.Etcd
			leasesList  *coordinationv1.LeaseList
		)

		BeforeEach(func() {
			ctx = context.Background()
			mockCtrl = gomock.NewController(GinkgoT())
			cl = mockclient.NewMockClient(mockCtrl)
			unknownThreshold = 300 * time.Second
			notReadyThreshold = 60 * time.Second
			now, _ = time.Parse(time.RFC3339, "2021-06-01T00:00:00Z")
			logger = log.Log.WithName("Test")
			check = ReadyCheck(cl, logger, notReadyThreshold, unknownThreshold)

			member1ID = pointer.String("1")
			member1Name = "member1"

			etcd = druidv1alpha1.Etcd{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "etcd",
					Namespace: "etcd-test",
				},
			}
		})

		AfterEach(func() {
			mockCtrl.Finish()
		})

		JustBeforeEach(func() {
			cl.EXPECT().List(ctx, gomock.AssignableToTypeOf(&coordinationv1.LeaseList{}), client.InNamespace(etcd.Namespace),
				client.MatchingLabels{common.GardenerOwnedBy: etcd.Name, v1beta1constants.GardenerPurpose: utils.PurposeMemberLease}).
				DoAndReturn(
					func(_ context.Context, leases *coordinationv1.LeaseList, _ ...client.ListOption) error {
						*leases = *leasesList
						return nil
					})
		})

		Context("when just expired", func() {
			BeforeEach(func() {
				renewTime := metav1.NewMicroTime(now.Add(-1 * unknownThreshold).Add(-1 * time.Second))
				leasesList = &coordinationv1.LeaseList{
					Items: []coordinationv1.Lease{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      member1Name,
								Namespace: etcd.Namespace,
							},
							Spec: coordinationv1.LeaseSpec{
								HolderIdentity:       pointer.String(fmt.Sprintf("%s:%s", *member1ID, druidv1alpha1.EtcdRoleLeader)),
								LeaseDurationSeconds: leaseDurationSeconds,
								RenewTime:            &renewTime,
							},
						},
					},
				}
			})

			It("should set the affected condition to UNKNOWN because lease is lost", func() {
				defer test.WithVar(&TimeNow, func() time.Time {
					return now
				})()

				cl.EXPECT().Get(ctx, kutil.Key(etcd.Namespace, member1Name), gomock.AssignableToTypeOf(&corev1.Pod{})).DoAndReturn(
					func(_ context.Context, _ client.ObjectKey, pod *corev1.Pod, _ ...client.ListOption) error {
						*pod = corev1.Pod{
							Status: corev1.PodStatus{
								Conditions: []corev1.PodCondition{
									{
										Type:   corev1.ContainersReady,
										Status: corev1.ConditionTrue,
									},
								},
							},
						}
						return nil
					},
				)

				results := check.Check(ctx, etcd)

				Expect(results).To(HaveLen(1))
				Expect(results[0].Status()).To(Equal(druidv1alpha1.EtcdMemberStatusUnknown))
				Expect(results[0].ID()).To(Equal(member1ID))
				Expect(results[0].Role()).To(gstruct.PointTo(Equal(druidv1alpha1.EtcdRoleLeader)))
			})

			It("should set the affected condition to UNKNOWN because Pod cannot be received", func() {
				defer test.WithVar(&TimeNow, func() time.Time {
					return now
				})()

				cl.EXPECT().Get(ctx, kutil.Key(etcd.Namespace, member1Name), gomock.AssignableToTypeOf(&corev1.Pod{})).DoAndReturn(
					func(_ context.Context, _ client.ObjectKey, pod *corev1.Pod, _ ...client.ListOption) error {
						return errors.New("foo")
					},
				)

				results := check.Check(ctx, etcd)

				Expect(results).To(HaveLen(1))
				Expect(results[0].Status()).To(Equal(druidv1alpha1.EtcdMemberStatusUnknown))
				Expect(results[0].ID()).To(Equal(member1ID))
				Expect(results[0].Role()).To(gstruct.PointTo(Equal(druidv1alpha1.EtcdRoleLeader)))
			})

			It("should set the affected condition to FAILED because containers are not ready", func() {
				defer test.WithVar(&TimeNow, func() time.Time {
					return now
				})()

				cl.EXPECT().Get(ctx, kutil.Key(etcd.Namespace, member1Name), gomock.AssignableToTypeOf(&corev1.Pod{})).DoAndReturn(
					func(_ context.Context, _ client.ObjectKey, pod *corev1.Pod, _ ...client.ListOption) error {
						*pod = corev1.Pod{
							Status: corev1.PodStatus{
								Conditions: []corev1.PodCondition{
									{
										Type:   corev1.ContainersReady,
										Status: corev1.ConditionFalse,
									},
								},
							},
						}
						return nil
					},
				)

				results := check.Check(ctx, etcd)

				Expect(results).To(HaveLen(1))
				Expect(results[0].Status()).To(Equal(druidv1alpha1.EtcdMemberStatusNotReady))
				Expect(results[0].ID()).To(Equal(member1ID))
				Expect(results[0].Role()).To(gstruct.PointTo(Equal(druidv1alpha1.EtcdRoleLeader)))
			})

			It("should set the affected condition to FAILED because Pod is not found", func() {
				defer test.WithVar(&TimeNow, func() time.Time {
					return now
				})()

				cl.EXPECT().Get(ctx, kutil.Key(etcd.Namespace, member1Name), gomock.AssignableToTypeOf(&corev1.Pod{})).DoAndReturn(
					func(_ context.Context, _ client.ObjectKey, pod *corev1.Pod, _ ...client.ListOption) error {
						return apierrors.NewNotFound(corev1.Resource("pods"), member1Name)
					},
				)

				results := check.Check(ctx, etcd)

				Expect(results).To(HaveLen(1))
				Expect(results[0].Status()).To(Equal(druidv1alpha1.EtcdMemberStatusNotReady))
				Expect(results[0].ID()).To(Equal(member1ID))
				Expect(results[0].Role()).To(gstruct.PointTo(Equal(druidv1alpha1.EtcdRoleLeader)))
			})
		})

		Context("when expired a while ago", func() {
			var (
				member2Name string
				member2ID   *string
			)

			BeforeEach(func() {
				member2Name = "member2"
				member2ID = pointer.String("2")

				var (
					shortExpirationTime = metav1.NewMicroTime(now.Add(-1 * unknownThreshold).Add(-1 * time.Second))
					longExpirationTime  = metav1.NewMicroTime(now.Add(-1 * unknownThreshold).Add(-1 * time.Second).Add(-1 * notReadyThreshold))
				)

				leasesList = &coordinationv1.LeaseList{
					Items: []coordinationv1.Lease{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      member1Name,
								Namespace: etcd.Namespace,
							},
							Spec: coordinationv1.LeaseSpec{
								HolderIdentity:       pointer.String(fmt.Sprintf("%s:%s", *member1ID, druidv1alpha1.EtcdRoleLeader)),
								LeaseDurationSeconds: leaseDurationSeconds,
								RenewTime:            &shortExpirationTime,
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      member2Name,
								Namespace: etcd.Namespace,
							},
							Spec: coordinationv1.LeaseSpec{
								HolderIdentity:       pointer.String(fmt.Sprintf("%s:%s", *member2ID, druidv1alpha1.EtcdRoleMember)),
								LeaseDurationSeconds: leaseDurationSeconds,
								RenewTime:            &longExpirationTime,
							},
						},
					},
				}
			})

			It("should set the affected condition to FAILED because status was Unknown for a while", func() {
				defer test.WithVar(&TimeNow, func() time.Time {
					return now
				})()

				cl.EXPECT().Get(ctx, kutil.Key(etcd.Namespace, member1Name), gomock.AssignableToTypeOf(&corev1.Pod{})).DoAndReturn(
					func(_ context.Context, _ client.ObjectKey, pod *corev1.Pod, _ ...client.ListOption) error {
						return errors.New("foo")
					},
				)

				results := check.Check(ctx, etcd)

				Expect(results).To(HaveLen(2))
				Expect(results[0].Status()).To(Equal(druidv1alpha1.EtcdMemberStatusUnknown))
				Expect(results[0].ID()).To(Equal(member1ID))
				Expect(results[0].Role()).To(gstruct.PointTo(Equal(druidv1alpha1.EtcdRoleLeader)))
				Expect(results[1].Status()).To(Equal(druidv1alpha1.EtcdMemberStatusNotReady))
				Expect(results[1].ID()).To(Equal(member2ID))
				Expect(results[1].Role()).To(gstruct.PointTo(Equal(druidv1alpha1.EtcdRoleMember)))
			})
		})

		Context("when lease is up-to-date", func() {
			var (
				member2Name, member3Name string
				member2ID, member3ID     *string
			)

			BeforeEach(func() {
				member2Name = "member2"
				member2ID = pointer.String("2")
				member3Name = "member3"
				member3ID = pointer.String("3")
				renewTime := metav1.NewMicroTime(now.Add(-1 * unknownThreshold))
				leasesList = &coordinationv1.LeaseList{
					Items: []coordinationv1.Lease{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      member1Name,
								Namespace: etcd.Namespace,
							},
							Spec: coordinationv1.LeaseSpec{
								HolderIdentity:       pointer.String(fmt.Sprintf("%s:%s", *member1ID, druidv1alpha1.EtcdRoleLeader)),
								LeaseDurationSeconds: leaseDurationSeconds,
								RenewTime:            &renewTime,
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      member2Name,
								Namespace: etcd.Namespace,
							},
							Spec: coordinationv1.LeaseSpec{
								HolderIdentity:       member2ID,
								LeaseDurationSeconds: leaseDurationSeconds,
								RenewTime:            &renewTime,
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      member3Name,
								Namespace: etcd.Namespace,
							},
							Spec: coordinationv1.LeaseSpec{
								HolderIdentity:       pointer.String(fmt.Sprintf("%s:%s", *member3ID, "foo")),
								LeaseDurationSeconds: leaseDurationSeconds,
								RenewTime:            &renewTime,
							},
						},
					},
				}
			})

			It("should set member ready", func() {
				defer test.WithVar(&TimeNow, func() time.Time {
					return now
				})()

				results := check.Check(ctx, etcd)

				Expect(results).To(HaveLen(3))
				Expect(results[0].Status()).To(Equal(druidv1alpha1.EtcdMemberStatusReady))
				Expect(results[0].ID()).To(Equal(member1ID))
				Expect(results[0].Role()).To(gstruct.PointTo(Equal(druidv1alpha1.EtcdRoleLeader)))
				Expect(results[1].Status()).To(Equal(druidv1alpha1.EtcdMemberStatusReady))
				Expect(results[1].ID()).To(Equal(member2ID))
				Expect(results[1].Role()).To(BeNil())
				Expect(results[2].Status()).To(Equal(druidv1alpha1.EtcdMemberStatusReady))
				Expect(results[2].ID()).To(Equal(member3ID))
				Expect(results[2].Role()).To(BeNil())
			})
		})

		Context("when lease has not been acquired", func() {
			var (
				member2Name string
			)

			BeforeEach(func() {
				member2Name = "member2"
				renewTime := metav1.NewMicroTime(now.Add(-1 * unknownThreshold))
				leasesList = &coordinationv1.LeaseList{
					Items: []coordinationv1.Lease{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      member1Name,
								Namespace: etcd.Namespace,
							},
							Spec: coordinationv1.LeaseSpec{
								HolderIdentity:       pointer.String(fmt.Sprintf("%s:%s", *member1ID, druidv1alpha1.EtcdRoleLeader)),
								LeaseDurationSeconds: leaseDurationSeconds,
								RenewTime:            &renewTime,
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      member2Name,
								Namespace: etcd.Namespace,
							},
							Spec: coordinationv1.LeaseSpec{
								HolderIdentity:       pointer.String("foo"),
								LeaseDurationSeconds: leaseDurationSeconds,
							},
						},
					},
				}
			})

			It("should only contain members which acquired lease once", func() {
				defer test.WithVar(&TimeNow, func() time.Time {
					return now
				})()

				results := check.Check(ctx, etcd)

				Expect(results).To(HaveLen(1))
				Expect(results[0].Status()).To(Equal(druidv1alpha1.EtcdMemberStatusReady))
				Expect(results[0].ID()).To(Equal(member1ID))
				Expect(results[0].Role()).To(gstruct.PointTo(Equal(druidv1alpha1.EtcdRoleLeader)))
			})
		})
	})
})
