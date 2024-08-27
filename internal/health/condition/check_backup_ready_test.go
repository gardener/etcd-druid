// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package condition_test

import (
	"context"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	mockclient "github.com/gardener/etcd-druid/internal/mock/controller-runtime/client"

	"go.uber.org/mock/gomock"
	coordinationv1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/gardener/etcd-druid/internal/health/condition"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("BackupsReadyCheck", func() {
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
	Describe("#SnapshotsBackupReadyCheck", func() {
		Describe("#DeltaSnapshotBackupReadyCheck", func() {

			Context("With no delta snapshot lease present", func() {
				It("Should return Unknown readiness", func() {
					cl.EXPECT().Get(context.TODO(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
						func(_ context.Context, _ client.ObjectKey, _ *coordinationv1.Lease, _ ...client.GetOption) error {
							return &noLeaseError
						},
					).AnyTimes()

					check := DeltaSnapshotBackupReadyCheck(cl)
					result := check.Check(context.TODO(), etcd)
					Expect(result).ToNot(BeNil())
					Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeDeltaSnapshotBackupReady))
					Expect(result.Status()).To(Equal(druidv1alpha1.ConditionUnknown))
					Expect(result.Reason()).To(Equal(Unknown))
				})
			})

			Context("With delta snapshot lease present", func() {

				Context("With lease not renewed even once", func() {
					It("Should set status to True if lease has not been renewed within the deltaSnapshotPeriod duration of lease creation", func() {
						cl.EXPECT().Get(context.TODO(), types.NamespacedName{Name: "test-etcd-delta-snap", Namespace: "default"}, gomock.Any(), gomock.Any()).DoAndReturn(
							func(_ context.Context, _ client.ObjectKey, le *coordinationv1.Lease, _ ...client.GetOption) error {
								*le = lease
								le.Spec.RenewTime = nil
								le.Spec.HolderIdentity = nil
								le.ObjectMeta.CreationTimestamp = v1.Now()
								return nil
							},
						).AnyTimes()

						check := DeltaSnapshotBackupReadyCheck(cl)
						result := check.Check(context.TODO(), etcd)

						Expect(result).ToNot(BeNil())
						Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeDeltaSnapshotBackupReady))
						Expect(result.Status()).To(Equal(druidv1alpha1.ConditionTrue))
						Expect(result.Reason()).To(Equal(SnapshotProcessNotStarted))
					})

					It("Should set status to Unknown if lease has not been renewed until 3*deltaSnapshotPeriod duration of lease creation", func() {
						cl.EXPECT().Get(context.TODO(), types.NamespacedName{Name: "test-etcd-delta-snap", Namespace: "default"}, gomock.Any(), gomock.Any()).DoAndReturn(
							func(_ context.Context, _ client.ObjectKey, le *coordinationv1.Lease, _ ...client.GetOption) error {
								*le = lease
								le.Spec.RenewTime = nil
								le.Spec.HolderIdentity = nil
								le.ObjectMeta.CreationTimestamp = v1.Time{Time: time.Now().Add(-2 * deltaSnapshotDuration)}
								return nil
							},
						).AnyTimes()

						check := DeltaSnapshotBackupReadyCheck(cl)
						result := check.Check(context.TODO(), etcd)

						Expect(result).ToNot(BeNil())
						Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeDeltaSnapshotBackupReady))
						Expect(result.Status()).To(Equal(druidv1alpha1.ConditionUnknown))
						Expect(result.Reason()).To(Equal(Unknown))
					})

					It("Should set status to False if lease has not been renewed even after 3*deltaSnapshotPeriod duration of lease creation", func() {
						cl.EXPECT().Get(context.TODO(), types.NamespacedName{Name: "test-etcd-delta-snap", Namespace: "default"}, gomock.Any(), gomock.Any()).DoAndReturn(
							func(_ context.Context, _ client.ObjectKey, le *coordinationv1.Lease, _ ...client.GetOption) error {
								*le = lease
								le.Spec.RenewTime = nil
								le.Spec.HolderIdentity = nil
								le.ObjectMeta.CreationTimestamp = v1.Time{Time: time.Now().Add(-4 * deltaSnapshotDuration)}
								return nil
							},
						).AnyTimes()

						etcd.Status.Conditions = []druidv1alpha1.Condition{
							{
								Type:    druidv1alpha1.ConditionTypeDeltaSnapshotBackupReady,
								Status:  druidv1alpha1.ConditionUnknown,
								Message: "Unknown",
							},
						}

						check := DeltaSnapshotBackupReadyCheck(cl)
						result := check.Check(context.TODO(), etcd)

						Expect(result).ToNot(BeNil())
						Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeDeltaSnapshotBackupReady))
						Expect(result.Status()).To(Equal(druidv1alpha1.ConditionFalse))
						Expect(result.Reason()).To(Equal(SnapshotMissedSchedule))
					})
				})

				Context("With lease renewed at least once", func() {
					It("Should set status to True if lease has been renewed within the deltaSnapshotPeriod duration", func() {
						cl.EXPECT().Get(context.TODO(), types.NamespacedName{Name: "test-etcd-delta-snap", Namespace: "default"}, gomock.Any(), gomock.Any()).DoAndReturn(
							func(_ context.Context, _ client.ObjectKey, le *coordinationv1.Lease, _ ...client.GetOption) error {
								*le = lease
								le.Spec.RenewTime = &v1.MicroTime{Time: time.Now()}
								return nil
							},
						).AnyTimes()

						check := DeltaSnapshotBackupReadyCheck(cl)
						result := check.Check(context.TODO(), etcd)

						Expect(result).ToNot(BeNil())
						Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeDeltaSnapshotBackupReady))
						Expect(result.Status()).To(Equal(druidv1alpha1.ConditionTrue))
						Expect(result.Reason()).To(Equal(SnapshotUploadedOnSchedule))
					})

					It("Should set status to Unknown if lease has not been renewed within the 3*deltaSnapshotPeriod duration", func() {
						cl.EXPECT().Get(context.TODO(), types.NamespacedName{Name: "test-etcd-delta-snap", Namespace: "default"}, gomock.Any(), gomock.Any()).DoAndReturn(
							func(_ context.Context, _ client.ObjectKey, le *coordinationv1.Lease, _ ...client.GetOption) error {
								*le = lease
								le.Spec.RenewTime = &v1.MicroTime{Time: time.Now().Add(-2 * deltaSnapshotDuration)}
								return nil
							},
						).AnyTimes()

						check := DeltaSnapshotBackupReadyCheck(cl)
						result := check.Check(context.TODO(), etcd)

						Expect(result).ToNot(BeNil())
						Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeDeltaSnapshotBackupReady))
						Expect(result.Status()).To(Equal(druidv1alpha1.ConditionUnknown))
						Expect(result.Reason()).To(Equal(Unknown))
					})

					It("Should set status to False if lease has not been renewed even after 3*deltaSnapshotPeriod duration", func() {
						cl.EXPECT().Get(context.TODO(), types.NamespacedName{Name: "test-etcd-delta-snap", Namespace: "default"}, gomock.Any(), gomock.Any()).DoAndReturn(
							func(_ context.Context, _ client.ObjectKey, le *coordinationv1.Lease, _ ...client.GetOption) error {
								*le = lease
								le.Spec.RenewTime = &v1.MicroTime{Time: time.Now().Add(-4 * deltaSnapshotDuration)}
								return nil
							},
						).AnyTimes()

						etcd.Status.Conditions = []druidv1alpha1.Condition{
							{
								Type:    druidv1alpha1.ConditionTypeDeltaSnapshotBackupReady,
								Status:  druidv1alpha1.ConditionUnknown,
								Message: "Unknown",
							},
						}

						check := DeltaSnapshotBackupReadyCheck(cl)
						result := check.Check(context.TODO(), etcd)

						Expect(result).ToNot(BeNil())
						Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeDeltaSnapshotBackupReady))
						Expect(result.Status()).To(Equal(druidv1alpha1.ConditionFalse))
						Expect(result.Reason()).To(Equal(SnapshotMissedSchedule))
					})
				})
			})

			Context("With no backup store configured", func() {
				It("Should return nil condition", func() {
					cl.EXPECT().Get(context.TODO(), gomock.Any(), gomock.Any()).DoAndReturn(
						func(_ context.Context, _ client.ObjectKey, _ *coordinationv1.Lease) error {
							return &noLeaseError
						},
					).AnyTimes()

					etcd.Spec.Backup.Store = nil
					check := DeltaSnapshotBackupReadyCheck(cl)
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
						func(_ context.Context, _ client.ObjectKey, _ *coordinationv1.Lease) error {
							return &noLeaseError
						},
					).AnyTimes()

					etcd.Spec.Backup.Store.Provider = nil
					check := DeltaSnapshotBackupReadyCheck(cl)
					result := check.Check(context.TODO(), etcd)

					Expect(result).To(BeNil())
					etcd.Spec.Backup.Store.Provider = &storageProvider
				})
			})
		})
		Describe("#FullSnapshotBackupReadyCheck", func() {
			Context("With no full snapshot lease present", func() {
				It("Should return Unknown readiness", func() {
					cl.EXPECT().Get(context.TODO(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
						func(_ context.Context, _ client.ObjectKey, _ *coordinationv1.Lease, _ ...client.GetOption) error {
							return &noLeaseError
						},
					).AnyTimes()

					check := FullSnapshotBackupReadyCheck(cl)
					result := check.Check(context.TODO(), etcd)
					Expect(result).ToNot(BeNil())
					Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeFullSnapshotBackupReady))
					Expect(result.Status()).To(Equal(druidv1alpha1.ConditionUnknown))
					Expect(result.Reason()).To(Equal(Unknown))
				})
			})

			Context("With full snapshot lease present", func() {
				Context("With lease not renewed even once", func() {
					It("Should set status to Unknown if lease has not been renewed at all within the first 24 hours of lease creation", func() {
						cl.EXPECT().Get(context.TODO(), types.NamespacedName{Name: "test-etcd-full-snap", Namespace: "default"}, gomock.Any(), gomock.Any()).DoAndReturn(
							func(_ context.Context, _ client.ObjectKey, le *coordinationv1.Lease, _ ...client.GetOption) error {
								*le = lease
								le.Spec.RenewTime = nil
								le.Spec.HolderIdentity = nil
								le.ObjectMeta.CreationTimestamp = v1.Now()
								return nil
							},
						).AnyTimes()

						check := FullSnapshotBackupReadyCheck(cl)
						result := check.Check(context.TODO(), etcd)

						Expect(result).ToNot(BeNil())
						Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeFullSnapshotBackupReady))
						Expect(result.Status()).To(Equal(druidv1alpha1.ConditionUnknown))
						Expect(result.Reason()).To(Equal(Unknown))
					})

					It("Should set status to False if lease has not been renewed at all even after 24 hours of lease creation", func() {
						cl.EXPECT().Get(context.TODO(), types.NamespacedName{Name: "test-etcd-full-snap", Namespace: "default"}, gomock.Any(), gomock.Any()).DoAndReturn(
							func(_ context.Context, _ client.ObjectKey, le *coordinationv1.Lease, _ ...client.GetOption) error {
								*le = lease
								le.Spec.RenewTime = nil
								le.Spec.HolderIdentity = nil
								le.ObjectMeta.CreationTimestamp = v1.Time{Time: time.Now().Add(-25 * time.Hour)}
								return nil
							},
						).AnyTimes()

						check := FullSnapshotBackupReadyCheck(cl)
						result := check.Check(context.TODO(), etcd)

						Expect(result).ToNot(BeNil())
						Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeFullSnapshotBackupReady))
						Expect(result.Status()).To(Equal(druidv1alpha1.ConditionFalse))
						Expect(result.Reason()).To(Equal(SnapshotMissedSchedule))
					})
				})

				Context("With lease renewed at least once", func() {
					It("Should set status to True if lease has been renewed within the last 24 hours", func() {
						cl.EXPECT().Get(context.TODO(), types.NamespacedName{Name: "test-etcd-full-snap", Namespace: "default"}, gomock.Any(), gomock.Any()).DoAndReturn(
							func(_ context.Context, _ client.ObjectKey, le *coordinationv1.Lease, _ ...client.GetOption) error {
								*le = lease
								le.Spec.RenewTime = &v1.MicroTime{Time: time.Now()}
								return nil
							},
						).AnyTimes()

						check := FullSnapshotBackupReadyCheck(cl)
						result := check.Check(context.TODO(), etcd)

						Expect(result).ToNot(BeNil())
						Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeFullSnapshotBackupReady))
						Expect(result.Status()).To(Equal(druidv1alpha1.ConditionTrue))
						Expect(result.Reason()).To(Equal(SnapshotUploadedOnSchedule))
					})

					It("Should set status to False if lease has not been renewed within the last 24 hours", func() {
						cl.EXPECT().Get(context.TODO(), types.NamespacedName{Name: "test-etcd-full-snap", Namespace: "default"}, gomock.Any(), gomock.Any()).DoAndReturn(
							func(_ context.Context, _ client.ObjectKey, le *coordinationv1.Lease, _ ...client.GetOption) error {
								*le = lease
								le.Spec.RenewTime = &v1.MicroTime{Time: time.Now().Add(-25 * time.Hour)}
								return nil
							},
						).AnyTimes()

						check := FullSnapshotBackupReadyCheck(cl)
						result := check.Check(context.TODO(), etcd)

						Expect(result).ToNot(BeNil())
						Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeFullSnapshotBackupReady))
						Expect(result.Status()).To(Equal(druidv1alpha1.ConditionFalse))
						Expect(result.Reason()).To(Equal(SnapshotMissedSchedule))
					})
				})
			})

			Context("With no backup store configured", func() {
				It("Should return nil condition", func() {
					cl.EXPECT().Get(context.TODO(), gomock.Any(), gomock.Any()).DoAndReturn(
						func(_ context.Context, _ client.ObjectKey, _ *coordinationv1.Lease) error {
							return &noLeaseError
						},
					).AnyTimes()

					etcd.Spec.Backup.Store = nil
					check := FullSnapshotBackupReadyCheck(cl)
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
						func(_ context.Context, _ client.ObjectKey, _ *coordinationv1.Lease) error {
							return &noLeaseError
						},
					).AnyTimes()

					etcd.Spec.Backup.Store.Provider = nil
					check := FullSnapshotBackupReadyCheck(cl)
					result := check.Check(context.TODO(), etcd)

					Expect(result).To(BeNil())
					etcd.Spec.Backup.Store.Provider = &storageProvider
				})
			})
		})
	})
	Describe("#BackupReadyCheck", func() {
		var results []Result
		BeforeEach(func() {
			results = []Result{
				&result{
					ConType:   druidv1alpha1.ConditionTypeFullSnapshotBackupReady,
					ConStatus: druidv1alpha1.ConditionTrue,
				},
				&result{
					ConType:   druidv1alpha1.ConditionTypeDeltaSnapshotBackupReady,
					ConStatus: druidv1alpha1.ConditionTrue,
				},
				nil,
				&result{
					ConType:   druidv1alpha1.ConditionTypeReady,
					ConStatus: druidv1alpha1.ConditionTrue,
				},
				&result{
					ConType:   druidv1alpha1.ConditionTypeDataVolumesReady,
					ConStatus: druidv1alpha1.ConditionTrue,
				},
			}
		})
		Context("With at least one of Full or Delta snapshot backup condition check is nil", func() {
			It("Should return nil condition", func() {
				results = append(results[:1], results[2:]...)
				check := BackupReadyCheck(cl, results)
				result := check.Check(context.TODO(), etcd)
				Expect(result).To(BeNil())

				results = append(results[:], results[1:]...)
				check = BackupReadyCheck(cl, results)
				result = check.Check(context.TODO(), etcd)
				Expect(result).To(BeNil())
			})
		})

		Context("With at least one of Full or Delta snapshot backup condition check is False", func() {
			It("Should return False readiness", func() {
				results[0].(*result).ConStatus = druidv1alpha1.ConditionFalse
				check := BackupReadyCheck(cl, results)
				result := check.Check(context.TODO(), etcd)
				Expect(result).ToNot(BeNil())
				Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeBackupReady))
				Expect(result.Status()).To(Equal(druidv1alpha1.ConditionFalse))
				Expect(result.Reason()).To(Equal(BackupFailed))
			})
		})

		Context("With Full snapshot backup condition check is Unknown", func() {
			It("Should return Unknown readiness", func() {
				results[0].(*result).ConStatus = druidv1alpha1.ConditionUnknown
				check := BackupReadyCheck(cl, results)
				result := check.Check(context.TODO(), etcd)
				Expect(result).ToNot(BeNil())
				Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeBackupReady))
				Expect(result.Status()).To(Equal(druidv1alpha1.ConditionUnknown))
				Expect(result.Reason()).To(Equal(Unknown))
			})
		})

		Context("With Full snapshot backup condition check is True and Delta snapshot backup condition check is Unknown or True", func() {
			It("Should return True readiness", func() {
				results[1].(*result).ConStatus = druidv1alpha1.ConditionUnknown
				check := BackupReadyCheck(cl, results)
				result := check.Check(context.TODO(), etcd)
				Expect(result).ToNot(BeNil())
				Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeBackupReady))
				Expect(result.Status()).To(Equal(druidv1alpha1.ConditionTrue))
				Expect(result.Reason()).To(Equal(BackupSucceeded))
			})
		})
	})
})
