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

package etcdmember_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	kutil "github.com/gardener/gardener/pkg/utils/kubernetes"
	"github.com/gardener/gardener/pkg/utils/test"
	"github.com/go-logr/logr"
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
	. "github.com/gardener/etcd-druid/pkg/health/etcdmember"
	mockclient "github.com/gardener/etcd-druid/pkg/mock/controller-runtime/client"
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

			etcdName      = "test"
			etcdNamespace = "test"
			member0Name   = fmt.Sprintf("%s-%d", etcdName, 0)
			member0ID     = pointer.String("0")
			member1Name   = fmt.Sprintf("%s-%d", etcdName, 1)
			member1ID     = pointer.String("1")
			member2Name   = fmt.Sprintf("%s-%d", etcdName, 2)
			member2ID     = pointer.String("2")

			etcd       druidv1alpha1.Etcd
			leasesList *coordinationv1.LeaseList
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

			etcd = druidv1alpha1.Etcd{
				ObjectMeta: metav1.ObjectMeta{
					Name:      etcdName,
					Namespace: etcdNamespace,
				},
			}
		})

		AfterEach(func() {
			mockCtrl.Finish()
		})

		JustBeforeEach(func() {
			for _, lease := range leasesList.Items {
				lease := lease
				cl.EXPECT().Get(ctx, kutil.Key(lease.Namespace, lease.Name), gomock.AssignableToTypeOf(&coordinationv1.Lease{})).
					DoAndReturn(
						func(_ context.Context, _ client.ObjectKey, l *coordinationv1.Lease, _ ...client.GetOption) error {
							*l = lease
							return nil
						})
			}
		})

		Context("when just expired", func() {
			BeforeEach(func() {
				renewTime := metav1.NewMicroTime(now.Add(-1 * unknownThreshold).Add(-1 * time.Second))
				etcd.Spec.Replicas = 1
				leasesList = &coordinationv1.LeaseList{
					Items: []coordinationv1.Lease{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      member0Name,
								Namespace: etcd.Namespace,
							},
							Spec: coordinationv1.LeaseSpec{
								HolderIdentity:       pointer.String(fmt.Sprintf("%s:%s", *member0ID, druidv1alpha1.EtcdRoleLeader)),
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

				cl.EXPECT().Get(ctx, kutil.Key(etcd.Namespace, member0Name), gomock.AssignableToTypeOf(&corev1.Pod{})).DoAndReturn(
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
				Expect(results[0].ID()).To(Equal(member0ID))
				Expect(results[0].Role()).To(gstruct.PointTo(Equal(druidv1alpha1.EtcdRoleLeader)))
			})

			It("should set the affected condition to UNKNOWN because Pod cannot be received", func() {
				defer test.WithVar(&TimeNow, func() time.Time {
					return now
				})()

				cl.EXPECT().Get(ctx, kutil.Key(etcd.Namespace, member0Name), gomock.AssignableToTypeOf(&corev1.Pod{})).DoAndReturn(
					func(_ context.Context, _ client.ObjectKey, pod *corev1.Pod, _ ...client.ListOption) error {
						return errors.New("foo")
					},
				)

				results := check.Check(ctx, etcd)

				Expect(results).To(HaveLen(1))
				Expect(results[0].Status()).To(Equal(druidv1alpha1.EtcdMemberStatusUnknown))
				Expect(results[0].ID()).To(Equal(member0ID))
				Expect(results[0].Role()).To(gstruct.PointTo(Equal(druidv1alpha1.EtcdRoleLeader)))
			})

			It("should set the affected condition to FAILED because containers are not ready", func() {
				defer test.WithVar(&TimeNow, func() time.Time {
					return now
				})()

				cl.EXPECT().Get(ctx, kutil.Key(etcd.Namespace, member0Name), gomock.AssignableToTypeOf(&corev1.Pod{})).DoAndReturn(
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
				Expect(results[0].ID()).To(Equal(member0ID))
				Expect(results[0].Role()).To(gstruct.PointTo(Equal(druidv1alpha1.EtcdRoleLeader)))
			})

			It("should set the affected condition to FAILED because Pod is not found", func() {
				defer test.WithVar(&TimeNow, func() time.Time {
					return now
				})()

				cl.EXPECT().Get(ctx, kutil.Key(etcd.Namespace, member0Name), gomock.AssignableToTypeOf(&corev1.Pod{})).DoAndReturn(
					func(_ context.Context, _ client.ObjectKey, pod *corev1.Pod, _ ...client.ListOption) error {
						return apierrors.NewNotFound(corev1.Resource("pods"), member0Name)
					},
				)

				results := check.Check(ctx, etcd)

				Expect(results).To(HaveLen(1))
				Expect(results[0].Status()).To(Equal(druidv1alpha1.EtcdMemberStatusNotReady))
				Expect(results[0].ID()).To(Equal(member0ID))
				Expect(results[0].Role()).To(gstruct.PointTo(Equal(druidv1alpha1.EtcdRoleLeader)))
			})
		})

		Context("when expired a while ago", func() {
			BeforeEach(func() {

				var (
					shortExpirationTime = metav1.NewMicroTime(now.Add(-1 * unknownThreshold).Add(-1 * time.Second))
					longExpirationTime  = metav1.NewMicroTime(now.Add(-1 * unknownThreshold).Add(-1 * time.Second).Add(-1 * notReadyThreshold))
				)

				etcd.Spec.Replicas = 2
				leasesList = &coordinationv1.LeaseList{
					Items: []coordinationv1.Lease{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      member0Name,
								Namespace: etcd.Namespace,
							},
							Spec: coordinationv1.LeaseSpec{
								HolderIdentity:       pointer.String(fmt.Sprintf("%s:%s", *member0ID, druidv1alpha1.EtcdRoleLeader)),
								LeaseDurationSeconds: leaseDurationSeconds,
								RenewTime:            &shortExpirationTime,
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      member1Name,
								Namespace: etcd.Namespace,
							},
							Spec: coordinationv1.LeaseSpec{
								HolderIdentity:       pointer.String(fmt.Sprintf("%s:%s", *member1ID, druidv1alpha1.EtcdRoleMember)),
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

				cl.EXPECT().Get(ctx, kutil.Key(etcd.Namespace, member0Name), gomock.AssignableToTypeOf(&corev1.Pod{})).DoAndReturn(
					func(_ context.Context, _ client.ObjectKey, pod *corev1.Pod, _ ...client.ListOption) error {
						return errors.New("foo")
					},
				)

				results := check.Check(ctx, etcd)

				Expect(results).To(HaveLen(2))
				Expect(results[0].Status()).To(Equal(druidv1alpha1.EtcdMemberStatusUnknown))
				Expect(results[0].ID()).To(Equal(member0ID))
				Expect(results[0].Role()).To(gstruct.PointTo(Equal(druidv1alpha1.EtcdRoleLeader)))
				Expect(results[1].Status()).To(Equal(druidv1alpha1.EtcdMemberStatusNotReady))
				Expect(results[1].ID()).To(Equal(member1ID))
				Expect(results[1].Role()).To(gstruct.PointTo(Equal(druidv1alpha1.EtcdRoleMember)))
			})
		})

		Context("when lease is up-to-date", func() {
			BeforeEach(func() {
				renewTime := metav1.NewMicroTime(now.Add(-1 * unknownThreshold))
				etcd.Spec.Replicas = 3
				leasesList = &coordinationv1.LeaseList{
					Items: []coordinationv1.Lease{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      member0Name,
								Namespace: etcd.Namespace,
							},
							Spec: coordinationv1.LeaseSpec{
								HolderIdentity:       pointer.String(fmt.Sprintf("%s:%s", *member0ID, druidv1alpha1.EtcdRoleLeader)),
								LeaseDurationSeconds: leaseDurationSeconds,
								RenewTime:            &renewTime,
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      member1Name,
								Namespace: etcd.Namespace,
							},
							Spec: coordinationv1.LeaseSpec{
								HolderIdentity:       member1ID,
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
								HolderIdentity:       pointer.String(fmt.Sprintf("%s:%s", *member2ID, "foo")),
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
				Expect(results[0].ID()).To(Equal(member0ID))
				Expect(results[0].Role()).To(gstruct.PointTo(Equal(druidv1alpha1.EtcdRoleLeader)))
				Expect(results[1].Status()).To(Equal(druidv1alpha1.EtcdMemberStatusReady))
				Expect(results[1].ID()).To(Equal(member1ID))
				Expect(results[1].Role()).To(BeNil())
				Expect(results[2].Status()).To(Equal(druidv1alpha1.EtcdMemberStatusReady))
				Expect(results[2].ID()).To(Equal(member2ID))
				Expect(results[2].Role()).To(BeNil())
			})
		})

		Context("when lease has not been acquired", func() {
			BeforeEach(func() {
				renewTime := metav1.NewMicroTime(now.Add(-1 * unknownThreshold))
				etcd.Spec.Replicas = 2
				leasesList = &coordinationv1.LeaseList{
					Items: []coordinationv1.Lease{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      member0Name,
								Namespace: etcd.Namespace,
							},
							Spec: coordinationv1.LeaseSpec{
								HolderIdentity:       pointer.String(fmt.Sprintf("%s:%s", *member0ID, druidv1alpha1.EtcdRoleLeader)),
								LeaseDurationSeconds: leaseDurationSeconds,
								RenewTime:            &renewTime,
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:      member1Name,
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
				Expect(results[0].ID()).To(Equal(member0ID))
				Expect(results[0].Role()).To(gstruct.PointTo(Equal(druidv1alpha1.EtcdRoleLeader)))
			})
		})
	})
})
