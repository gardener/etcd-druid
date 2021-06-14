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
	"time"

	kutil "github.com/gardener/gardener/pkg/utils/kubernetes"
	"github.com/gardener/gardener/pkg/utils/test"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/controllers/config"
	controllersconfig "github.com/gardener/etcd-druid/controllers/config"
	. "github.com/gardener/etcd-druid/pkg/health/etcdmember"
	mockclient "github.com/gardener/etcd-druid/pkg/mock/controller-runtime/client"
)

var _ = Describe("ReadyCheck", func() {
	Describe("#Check", func() {
		var (
			ctx                                 context.Context
			mockCtrl                            *gomock.Controller
			cl                                  *mockclient.MockClient
			unknownThreshold, notReadyThreshold time.Duration
			now                                 time.Time
			check                               Checker
		)
		BeforeEach(func() {
			ctx = context.Background()
			mockCtrl = gomock.NewController(GinkgoT())
			cl = mockclient.NewMockClient(mockCtrl)
			unknownThreshold = 300 * time.Second
			notReadyThreshold = 60 * time.Second
			now, _ = time.Parse(time.RFC3339, "2021-06-01T00:00:00Z")
			check = ReadyCheck(cl, controllersconfig.EtcdCustodianController{
				EtcdMember: controllersconfig.EtcdMemberConfig{
					EtcdMemberUnknownThreshold:  unknownThreshold,
					EtcdMemberNotReadyThreshold: notReadyThreshold,
				},
			})
		})

		AfterEach(func() {
			mockCtrl.Finish()
		})

		Context("when condition is outdated", func() {
			var (
				podName string
				etcd    druidv1alpha1.Etcd
			)

			BeforeEach(func() {
				podName = "member1"
				etcd = druidv1alpha1.Etcd{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "etcd",
						Namespace: "etcd-test",
					},
					Status: druidv1alpha1.EtcdStatus{
						Members: []druidv1alpha1.EtcdMemberStatus{
							{
								Name:               podName,
								ID:                 "1",
								Role:               druidv1alpha1.EtcdRoleMember,
								Status:             druidv1alpha1.EtcdMemeberStatusReady,
								Reason:             "foo reason",
								LastTransitionTime: metav1.Now(),
								LastUpdateTime:     metav1.NewTime(now.Add(-301 * time.Second)),
							},
							{
								Name:               "member2",
								ID:                 "2",
								Role:               druidv1alpha1.EtcdRoleMember,
								Status:             druidv1alpha1.EtcdMemeberStatusReady,
								Reason:             "bar reason",
								LastTransitionTime: metav1.Now(),
								LastUpdateTime:     metav1.NewTime(now.Add(-1 * unknownThreshold)),
							},
						},
					},
				}
			})

			It("should set the affected condition to UNKNOWN", func() {
				defer test.WithVar(&TimeNow, func() time.Time {
					return now
				})()

				check := ReadyCheck(cl, controllersconfig.EtcdCustodianController{
					EtcdMember: controllersconfig.EtcdMemberConfig{
						EtcdMemberUnknownThreshold: unknownThreshold,
					},
				})

				cl.EXPECT().Get(ctx, kutil.Key(etcd.Namespace, podName), gomock.AssignableToTypeOf(&corev1.Pod{})).DoAndReturn(
					func(_ context.Context, _ client.ObjectKey, pod *corev1.Pod) error {
						*pod = corev1.Pod{
							Status: corev1.PodStatus{
								Phase: corev1.PodRunning,
							},
						}
						return nil
					},
				)

				results := check.Check(ctx, etcd)

				Expect(results).To(HaveLen(1))
				Expect(results[0].Status()).To(Equal(druidv1alpha1.EtcdMemeberStatusUnknown))
				Expect(results[0].ID()).To(Equal("1"))
			})

			It("should set the affected condition to UNKNOWN because Pod cannot be received", func() {
				defer test.WithVar(&TimeNow, func() time.Time {
					return now
				})()

				check := ReadyCheck(cl, controllersconfig.EtcdCustodianController{
					EtcdMember: controllersconfig.EtcdMemberConfig{
						EtcdMemberUnknownThreshold: unknownThreshold,
					},
				})

				cl.EXPECT().Get(ctx, kutil.Key(etcd.Namespace, podName), gomock.AssignableToTypeOf(&corev1.Pod{})).DoAndReturn(
					func(_ context.Context, _ client.ObjectKey, pod *corev1.Pod) error {
						return errors.New("foo")
					},
				)

				results := check.Check(ctx, etcd)

				Expect(results).To(HaveLen(1))
				Expect(results[0].Status()).To(Equal(druidv1alpha1.EtcdMemeberStatusUnknown))
				Expect(results[0].ID()).To(Equal("1"))
			})

			It("should set the affected condition to FAILED because Pod is not running", func() {
				defer test.WithVar(&TimeNow, func() time.Time {
					return now
				})()

				check := ReadyCheck(cl, controllersconfig.EtcdCustodianController{
					EtcdMember: controllersconfig.EtcdMemberConfig{
						EtcdMemberUnknownThreshold: unknownThreshold,
					},
				})

				cl.EXPECT().Get(ctx, kutil.Key(etcd.Namespace, podName), gomock.AssignableToTypeOf(&corev1.Pod{})).DoAndReturn(
					func(_ context.Context, _ client.ObjectKey, pod *corev1.Pod) error {
						*pod = corev1.Pod{
							Status: corev1.PodStatus{
								Phase: corev1.PodFailed,
							},
						}
						return nil
					},
				)

				results := check.Check(ctx, etcd)

				Expect(results).To(HaveLen(1))
				Expect(results[0].Status()).To(Equal(druidv1alpha1.EtcdMemeberStatusNotReady))
				Expect(results[0].ID()).To(Equal("1"))
			})

			It("should set the affected condition to FAILED because Pod is not found", func() {
				defer test.WithVar(&TimeNow, func() time.Time {
					return now
				})()

				check := ReadyCheck(cl, config.EtcdCustodianController{
					EtcdMember: controllersconfig.EtcdMemberConfig{
						EtcdMemberUnknownThreshold: unknownThreshold,
					},
				})

				cl.EXPECT().Get(ctx, kutil.Key(etcd.Namespace, podName), gomock.AssignableToTypeOf(&corev1.Pod{})).DoAndReturn(
					func(_ context.Context, _ client.ObjectKey, pod *corev1.Pod) error {
						return apierrors.NewNotFound(corev1.Resource("pods"), podName)
					},
				)

				results := check.Check(ctx, etcd)

				Expect(results).To(HaveLen(1))
				Expect(results[0].Status()).To(Equal(druidv1alpha1.EtcdMemeberStatusNotReady))
				Expect(results[0].ID()).To(Equal("1"))
			})

			It("should set the affected condition to FAILED because status was Unknown for a while", func() {
				defer test.WithVar(&TimeNow, func() time.Time {
					return now
				})()

				latestWithinGrace := now.Add(-1 * notReadyThreshold)

				// member with unknown state within grace period
				etcd.Status.Members[0].Status = druidv1alpha1.EtcdMemeberStatusUnknown
				etcd.Status.Members[0].LastTransitionTime = metav1.NewTime(latestWithinGrace)

				// member with unknown state outside grace period
				etcd.Status.Members[1].Status = druidv1alpha1.EtcdMemeberStatusUnknown
				etcd.Status.Members[1].LastTransitionTime = metav1.NewTime(latestWithinGrace.Add(-1 * time.Second))

				cl.EXPECT().Get(ctx, kutil.Key(etcd.Namespace, podName), gomock.AssignableToTypeOf(&corev1.Pod{})).DoAndReturn(
					func(_ context.Context, _ client.ObjectKey, pod *corev1.Pod) error {
						return errors.New("foo")
					},
				)

				results := check.Check(ctx, etcd)

				Expect(results).To(HaveLen(2))
				Expect(results[0].Status()).To(Equal(druidv1alpha1.EtcdMemeberStatusUnknown))
				Expect(results[0].ID()).To(Equal("1"))
				Expect(results[1].Status()).To(Equal(druidv1alpha1.EtcdMemeberStatusNotReady))
				Expect(results[1].ID()).To(Equal("2"))
			})
		})

		Context("when condition is not outdated", func() {
			It("should not return any results", func() {
				defer test.WithVar(&TimeNow, func() time.Time {
					return now
				})()
				etcd := druidv1alpha1.Etcd{
					Status: druidv1alpha1.EtcdStatus{
						Members: []druidv1alpha1.EtcdMemberStatus{
							{
								Name:               "member1",
								ID:                 "1",
								Role:               druidv1alpha1.EtcdRoleMember,
								Status:             druidv1alpha1.EtcdMemeberStatusReady,
								Reason:             "foo reason",
								LastTransitionTime: metav1.Now(),
								LastUpdateTime:     metav1.Now(),
							},
							{
								Name:               "member2",
								ID:                 "2",
								Role:               druidv1alpha1.EtcdRoleMember,
								Status:             druidv1alpha1.EtcdMemeberStatusReady,
								Reason:             "bar reason",
								LastTransitionTime: metav1.Now(),
								LastUpdateTime:     metav1.NewTime(now.Add(-1 * unknownThreshold)),
							},
						},
					},
				}

				results := check.Check(ctx, etcd)

				Expect(results).To(BeEmpty())
			})
		})
	})
})
