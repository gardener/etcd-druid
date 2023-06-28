// Copyright (c) 2022 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package condition_test

import (
	"context"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	. "github.com/gardener/etcd-druid/pkg/health/condition"
	mockclient "github.com/gardener/etcd-druid/pkg/mock/controller-runtime/client"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	coordinationv1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

var _ = Describe("BackupReadyCheck", func() {
	Describe("#Check", func() {
		var (
			storageProvider druidv1alpha1.StorageProvider = "testStorageProvider"
			mockCtrl        *gomock.Controller
			cl              *mockclient.MockClient
			holderIDString  = "123455"
			noLeaseError    = apierrors.StatusError{
				ErrStatus: v1.Status{
					Reason: v1.StatusReasonNotFound,
				},
			}
			deltaSnapshotDuration = 2 * time.Minute

			etcd = druidv1alpha1.Etcd{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-etcd",
					Namespace: "default",
				},
				Spec: druidv1alpha1.EtcdSpec{
					Replicas: 1,
					Backup: druidv1alpha1.BackupSpec{
						FullSnapshotSchedule: pointer.String("0 0 * * *"), // at 00:00 every day
						DeltaSnapshotPeriod: &v1.Duration{
							Duration: deltaSnapshotDuration,
						},
						Store: &druidv1alpha1.StoreSpec{
							Prefix:   "test-prefix",
							Provider: &storageProvider,
						},
					},
				},
				Status: druidv1alpha1.EtcdStatus{},
			}
			lease = coordinationv1.Lease{
				ObjectMeta: v1.ObjectMeta{
					Name: "test-etcd-snap",
				},
				Spec: coordinationv1.LeaseSpec{
					HolderIdentity: &holderIDString,
					RenewTime: &v1.MicroTime{
						Time: time.Now(),
					},
				},
			}
		)

		BeforeEach(func() {
			mockCtrl = gomock.NewController(GinkgoT())
			cl = mockclient.NewMockClient(mockCtrl)
		})

		AfterEach(func() {
			mockCtrl.Finish()
		})

		Context("With no snapshot leases present", func() {
			It("Should return Unknown readiness", func() {
				cl.EXPECT().Get(context.TODO(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ context.Context, _ client.ObjectKey, er *coordinationv1.Lease, _ ...client.GetOption) error {
						return &noLeaseError
					},
				).AnyTimes()

				check := BackupReadyCheck(cl)
				result := check.Check(context.TODO(), etcd)

				Expect(result).ToNot(BeNil())
				Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeBackupReady))
				Expect(result.Status()).To(Equal(druidv1alpha1.ConditionUnknown))
				Expect(result.Reason()).To(Equal(Unknown))
			})
		})

		Context("With both snapshot leases present", func() {
			It("Should set status to Unknown if both leases are recently created but not renewed", func() {
				cl.EXPECT().Get(context.TODO(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ context.Context, _ client.ObjectKey, le *coordinationv1.Lease, _ ...client.GetOption) error {
						*le = lease
						le.Spec.RenewTime = nil
						le.Spec.HolderIdentity = nil
						return nil
					},
				).AnyTimes()

				etcd.Status.Conditions = []druidv1alpha1.Condition{
					{
						Type:    druidv1alpha1.ConditionTypeBackupReady,
						Status:  druidv1alpha1.ConditionTrue,
						Message: "True",
					},
				}

				check := BackupReadyCheck(cl)
				result := check.Check(context.TODO(), etcd)

				Expect(result).ToNot(BeNil())
				Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeBackupReady))
				Expect(result.Status()).To(Equal(druidv1alpha1.ConditionUnknown))
				Expect(result.Reason()).To(Equal(Unknown))
				Expect(result.Message()).To(Equal("Snapshotter has not started yet"))
			})

			It("Should set status to BackupSucceeded if delta snap lease is renewed recently, and empty full snap lease has been created in the last 24h", func() {
				cl.EXPECT().Get(context.TODO(), types.NamespacedName{Name: "test-etcd-full-snap", Namespace: "default"}, gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ context.Context, _ client.ObjectKey, le *coordinationv1.Lease, _ ...client.GetOption) error {
						*le = lease
						le.Spec.RenewTime = nil
						le.Spec.HolderIdentity = nil
						le.ObjectMeta.CreationTimestamp = v1.Now()
						return nil
					},
				).AnyTimes()
				cl.EXPECT().Get(context.TODO(), types.NamespacedName{Name: "test-etcd-delta-snap", Namespace: "default"}, gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ context.Context, _ client.ObjectKey, le *coordinationv1.Lease, _ ...client.GetOption) error {
						*le = lease
						return nil
					},
				).AnyTimes()

				check := BackupReadyCheck(cl)
				result := check.Check(context.TODO(), etcd)

				Expect(result).ToNot(BeNil())
				Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeBackupReady))
				Expect(result.Status()).To(Equal(druidv1alpha1.ConditionTrue))
				Expect(result.Reason()).To(Equal(BackupSucceeded))
				Expect(result.Message()).To(Equal("Delta snapshot backup succeeded"))
			})

			It("Should set status to Unknown if delta snap lease is renewed recently, and no full snap lease has been created in the last 24h", func() {
				cl.EXPECT().Get(context.TODO(), types.NamespacedName{Name: "test-etcd-full-snap", Namespace: "default"}, gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ context.Context, _ client.ObjectKey, le *coordinationv1.Lease, _ ...client.GetOption) error {
						*le = lease
						le.Spec.RenewTime = nil
						le.Spec.HolderIdentity = nil
						le.ObjectMeta.CreationTimestamp = v1.NewTime(time.Now().Add(-25 * time.Hour))
						return nil
					},
				).AnyTimes()
				cl.EXPECT().Get(context.TODO(), types.NamespacedName{Name: "test-etcd-delta-snap", Namespace: "default"}, gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ context.Context, _ client.ObjectKey, le *coordinationv1.Lease, _ ...client.GetOption) error {
						*le = lease
						return nil
					},
				).AnyTimes()

				check := BackupReadyCheck(cl)
				result := check.Check(context.TODO(), etcd)

				Expect(result).ToNot(BeNil())
				Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeBackupReady))
				Expect(result.Status()).To(Equal(druidv1alpha1.ConditionFalse))
				Expect(result.Reason()).To(Equal(BackupFailed))
				Expect(result.Message()).To(Equal("Full snapshot backup failed. Full snapshot lease created long ago, but not renewed"))
			})

			It("Should set status to Unknown if empty delta snap lease is present but full snap lease has been renewed recently", func() {
				cl.EXPECT().Get(context.TODO(), types.NamespacedName{Name: "test-etcd-full-snap", Namespace: "default"}, gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ context.Context, _ client.ObjectKey, le *coordinationv1.Lease, _ ...client.GetOption) error {
						*le = lease
						le.Spec.RenewTime = &v1.MicroTime{Time: lease.Spec.RenewTime.Time.Add(-4 * deltaSnapshotDuration)}
						return nil
					},
				).AnyTimes()
				cl.EXPECT().Get(context.TODO(), types.NamespacedName{Name: "test-etcd-delta-snap", Namespace: "default"}, gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ context.Context, _ client.ObjectKey, le *coordinationv1.Lease, _ ...client.GetOption) error {
						*le = lease
						le.Spec.RenewTime = nil
						le.Spec.HolderIdentity = nil
						return nil
					},
				).AnyTimes()

				check := BackupReadyCheck(cl)
				result := check.Check(context.TODO(), etcd)

				Expect(result).ToNot(BeNil())
				Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeBackupReady))
				Expect(result.Status()).To(Equal(druidv1alpha1.ConditionUnknown))
				Expect(result.Reason()).To(Equal(Unknown))
				Expect(result.Message()).To(Equal("Waiting for delta snapshotting to begin"))
			})

			It("Should set status to Unknown if empty delta snap lease is present but full snap lease has not been renewed recently", func() {
				cl.EXPECT().Get(context.TODO(), types.NamespacedName{Name: "test-etcd-full-snap", Namespace: "default"}, gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ context.Context, _ client.ObjectKey, le *coordinationv1.Lease, _ ...client.GetOption) error {
						*le = lease
						le.Spec.RenewTime = &v1.MicroTime{Time: lease.Spec.RenewTime.Time.Add(-6 * deltaSnapshotDuration)}
						return nil
					},
				).AnyTimes()
				cl.EXPECT().Get(context.TODO(), types.NamespacedName{Name: "test-etcd-delta-snap", Namespace: "default"}, gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ context.Context, _ client.ObjectKey, le *coordinationv1.Lease, _ ...client.GetOption) error {
						*le = lease
						le.Spec.RenewTime = nil
						le.Spec.HolderIdentity = nil
						return nil
					},
				).AnyTimes()

				check := BackupReadyCheck(cl)
				result := check.Check(context.TODO(), etcd)

				Expect(result).ToNot(BeNil())
				Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeBackupReady))
				Expect(result.Status()).To(Equal(druidv1alpha1.ConditionFalse))
				Expect(result.Reason()).To(Equal(BackupFailed))
				Expect(result.Message()).To(Equal("Delta snapshot backup failed. Delta snapshot lease not renewed in a long time"))
			})

			It("Should set status to BackupSucceeded if both leases are recently renewed", func() {
				cl.EXPECT().Get(context.TODO(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ context.Context, _ client.ObjectKey, le *coordinationv1.Lease, _ ...client.GetOption) error {
						*le = lease
						return nil
					},
				).AnyTimes()

				check := BackupReadyCheck(cl)
				result := check.Check(context.TODO(), etcd)

				Expect(result).ToNot(BeNil())
				Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeBackupReady))
				Expect(result.Status()).To(Equal(druidv1alpha1.ConditionTrue))
				Expect(result.Reason()).To(Equal(BackupSucceeded))
				Expect(result.Message()).To(Equal("Snapshot backup succeeded"))
			})

			It("Should set status to BackupFailed if both leases are stale", func() {
				cl.EXPECT().Get(context.TODO(), types.NamespacedName{Name: "test-etcd-full-snap", Namespace: "default"}, gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ context.Context, _ client.ObjectKey, le *coordinationv1.Lease, _ ...client.GetOption) error {
						*le = lease
						le.Spec.RenewTime = &v1.MicroTime{
							Time: time.Now().Add(-25 * time.Hour),
						}
						return nil
					},
				).AnyTimes()
				cl.EXPECT().Get(context.TODO(), types.NamespacedName{Name: "test-etcd-delta-snap", Namespace: "default"}, gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ context.Context, _ client.ObjectKey, le *coordinationv1.Lease, _ ...client.GetOption) error {
						*le = lease
						le.Spec.RenewTime = &v1.MicroTime{
							Time: time.Now().Add(-5 * time.Minute),
						}
						return nil
					},
				).AnyTimes()

				check := BackupReadyCheck(cl)
				result := check.Check(context.TODO(), etcd)

				Expect(result).ToNot(BeNil())
				Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeBackupReady))
				Expect(result.Status()).To(Equal(druidv1alpha1.ConditionFalse))
				Expect(result.Reason()).To(Equal(BackupFailed))
				Expect(result.Message()).To(Equal("Stale snapshot leases. Not renewed in a long time"))
			})
		})

		Context("With no backup store configured", func() {
			It("Should return nil condition", func() {
				cl.EXPECT().Get(context.TODO(), gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ context.Context, _ client.ObjectKey, er *coordinationv1.Lease) error {
						return &noLeaseError
					},
				).AnyTimes()

				etcd.Spec.Backup.Store = nil
				check := BackupReadyCheck(cl)
				result := check.Check(context.TODO(), etcd)

				Expect(result).To(BeNil())
				etcd.Spec.Backup.Store = &druidv1alpha1.StoreSpec{
					Prefix:   "test-prefix",
					Provider: &storageProvider,
				}
			})
		})

		Context("With backup store is configured but provider is nil", func() {
			It("Should return nil condition", func() {
				cl.EXPECT().Get(context.TODO(), gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ context.Context, _ client.ObjectKey, er *coordinationv1.Lease) error {
						return &noLeaseError
					},
				).AnyTimes()

				etcd.Spec.Backup.Store.Provider = nil
				check := BackupReadyCheck(cl)
				result := check.Check(context.TODO(), etcd)

				Expect(result).To(BeNil())
				etcd.Spec.Backup.Store.Provider = &storageProvider
			})
		})
	})
})
