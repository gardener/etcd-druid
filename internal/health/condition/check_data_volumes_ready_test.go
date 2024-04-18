// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package condition_test

import (
	"context"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/health/condition"
	mockclient "github.com/gardener/etcd-druid/internal/mock/controller-runtime/client"
	"go.uber.org/mock/gomock"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("DataVolumesReadyCheck", func() {
	Describe("#Check", func() {
		var (
			mockCtrl *gomock.Controller
			cl       *mockclient.MockClient

			notFoundErr = apierrors.StatusError{
				ErrStatus: v1.Status{
					Reason: v1.StatusReasonNotFound,
				},
			}
			internalErr = apierrors.StatusError{
				ErrStatus: v1.Status{
					Reason: v1.StatusReasonInternalError,
				},
			}

			etcd = druidv1alpha1.Etcd{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test",
					Namespace: "default",
				},
				Spec: druidv1alpha1.EtcdSpec{
					Replicas: 1,
				},
			}
			sts = &appsv1.StatefulSet{
				ObjectMeta: v1.ObjectMeta{
					Name:            "test",
					Namespace:       "default",
					OwnerReferences: []metav1.OwnerReference{etcd.GetAsOwnerReference()},
				},
				Spec: appsv1.StatefulSetSpec{
					Replicas: pointer.Int32(1),
					VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
						{
							ObjectMeta: v1.ObjectMeta{
								Name: "test",
							},
						},
					},
				},
				Status: appsv1.StatefulSetStatus{
					CurrentReplicas: 1,
					ReadyReplicas:   1,
				},
			}
			pvc = &corev1.PersistentVolumeClaim{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-test-0",
					Namespace: "default",
				},
				Status: corev1.PersistentVolumeClaimStatus{
					Phase: corev1.ClaimPending,
				},
			}
			event = &corev1.Event{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-event",
					Namespace: "default",
				},
				InvolvedObject: corev1.ObjectReference{
					Kind:      "PersistentVolumeClaim",
					Name:      "test-pvc",
					Namespace: "default",
				},
				Reason:  "FailedMount",
				Message: "MountVolume.SetUp failed for volume \"test-pvc\" : kubernetes.io/csi: mounter.SetUpAt failed to get CSI client: driver name csi.gardener.cloud not found in the list of registered CSI drivers",
				Type:    corev1.EventTypeWarning,
			}
		)

		BeforeEach(func() {
			mockCtrl = gomock.NewController(GinkgoT())
			cl = mockclient.NewMockClient(mockCtrl)
		})

		AfterEach(func() {
			mockCtrl.Finish()
		})

		Context("when error in fetching statefulset", func() {
			It("should return that the condition is unknown", func() {
				cl.EXPECT().List(gomock.Any(), gomock.AssignableToTypeOf(&appsv1.StatefulSetList{}), gomock.Any()).DoAndReturn(
					func(_ context.Context, stsList *appsv1.StatefulSetList, _ ...client.ListOption) error {
						*stsList = appsv1.StatefulSetList{
							Items: []appsv1.StatefulSet{},
						}
						return &internalErr
					}).AnyTimes()

				check := condition.DataVolumesReadyCheck(cl)
				result := check.Check(context.Background(), etcd)

				Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeDataVolumesReady))
				Expect(result.Status()).To(Equal(druidv1alpha1.ConditionUnknown))
				Expect(result.Reason()).To(Equal("UnableToFetchStatefulSet"))
			})
		})

		Context("when statefulset not found", func() {
			It("should return that the condition is unknown", func() {
				cl.EXPECT().List(gomock.Any(), gomock.AssignableToTypeOf(&appsv1.StatefulSetList{}), gomock.Any()).DoAndReturn(
					func(_ context.Context, stsList *appsv1.StatefulSetList, _ ...client.ListOption) error {
						*stsList = appsv1.StatefulSetList{
							Items: []appsv1.StatefulSet{},
						}
						return &notFoundErr
					}).AnyTimes()

				check := condition.DataVolumesReadyCheck(cl)
				result := check.Check(context.Background(), etcd)

				Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeDataVolumesReady))
				Expect(result.Status()).To(Equal(druidv1alpha1.ConditionUnknown))
				Expect(result.Reason()).To(Equal("StatefulSetNotFound"))
			})
		})

		Context("when error in fetching warning events for PVCs", func() {
			It("should return that the condition is unknown", func() {
				cl.EXPECT().List(gomock.Any(), gomock.AssignableToTypeOf(&appsv1.StatefulSetList{}), gomock.Any()).DoAndReturn(
					func(_ context.Context, stsList *appsv1.StatefulSetList, _ ...client.ListOption) error {
						*stsList = appsv1.StatefulSetList{
							Items: []appsv1.StatefulSet{*sts},
						}
						return nil
					}).AnyTimes()

				cl.EXPECT().List(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ context.Context, _ client.ObjectList, _ ...client.ListOption) error {
						return &internalErr
					}).AnyTimes()

				check := condition.DataVolumesReadyCheck(cl)
				result := check.Check(context.Background(), etcd)

				Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeDataVolumesReady))
				Expect(result.Status()).To(Equal(druidv1alpha1.ConditionUnknown))
				Expect(result.Reason()).To(Equal("UnableToFetchWarningEventsForDataVolumes"))
			})
		})

		Context("when warning events found for PVCs", func() {
			It("should return that the condition is false", func() {
				cl.EXPECT().List(gomock.Any(), gomock.AssignableToTypeOf(&appsv1.StatefulSetList{}), gomock.Any()).DoAndReturn(
					func(_ context.Context, stsList *appsv1.StatefulSetList, _ ...client.ListOption) error {
						*stsList = appsv1.StatefulSetList{
							Items: []appsv1.StatefulSet{*sts},
						}
						return nil
					}).AnyTimes()

				cl.EXPECT().List(gomock.Any(), gomock.AssignableToTypeOf(&corev1.PersistentVolumeClaimList{}), gomock.Any()).DoAndReturn(
					func(_ context.Context, pvcs *corev1.PersistentVolumeClaimList, _ ...client.ListOption) error {
						*pvcs = corev1.PersistentVolumeClaimList{
							Items: []corev1.PersistentVolumeClaim{*pvc},
						}
						return nil
					}).AnyTimes()

				scheme := runtime.NewScheme()
				Expect(corev1.AddToScheme(scheme)).To(Succeed())
				cl.EXPECT().Scheme().Return(scheme).AnyTimes()

				cl.EXPECT().List(gomock.Any(), gomock.AssignableToTypeOf(&corev1.EventList{}), gomock.Any()).DoAndReturn(
					func(_ context.Context, eventsList *corev1.EventList, _ ...client.ListOption) error {
						*eventsList = corev1.EventList{
							Items: []corev1.Event{*event},
						}
						return nil
					}).AnyTimes()

				check := condition.DataVolumesReadyCheck(cl)
				result := check.Check(context.Background(), etcd)

				Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeDataVolumesReady))
				Expect(result.Status()).To(Equal(druidv1alpha1.ConditionFalse))
				Expect(result.Reason()).To(Equal("FoundWarningsForDataVolumes"))
			})
		})

		Context("when no warning events found for PVCs", func() {
			It("should return that the condition is true", func() {
				cl.EXPECT().List(gomock.Any(), gomock.AssignableToTypeOf(&appsv1.StatefulSetList{}), gomock.Any()).DoAndReturn(
					func(_ context.Context, stsList *appsv1.StatefulSetList, _ ...client.ListOption) error {
						*stsList = appsv1.StatefulSetList{
							Items: []appsv1.StatefulSet{*sts},
						}
						return nil
					}).AnyTimes()

				cl.EXPECT().List(gomock.Any(), gomock.AssignableToTypeOf(&corev1.PersistentVolumeClaimList{}), gomock.Any()).DoAndReturn(
					func(_ context.Context, pvcs *corev1.PersistentVolumeClaimList, _ ...client.ListOption) error {
						*pvcs = corev1.PersistentVolumeClaimList{
							Items: []corev1.PersistentVolumeClaim{*pvc},
						}
						return nil
					}).AnyTimes()

				scheme := runtime.NewScheme()
				Expect(corev1.AddToScheme(scheme)).To(Succeed())
				cl.EXPECT().Scheme().Return(scheme).AnyTimes()

				cl.EXPECT().List(gomock.Any(), gomock.AssignableToTypeOf(&corev1.EventList{}), gomock.Any()).DoAndReturn(
					func(_ context.Context, eventsList *corev1.EventList, _ ...client.ListOption) error {
						*eventsList = corev1.EventList{
							Items: []corev1.Event{},
						}
						return nil
					}).AnyTimes()

				check := condition.DataVolumesReadyCheck(cl)
				result := check.Check(context.Background(), etcd)

				Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeDataVolumesReady))
				Expect(result.Status()).To(Equal(druidv1alpha1.ConditionTrue))
				Expect(result.Reason()).To(Equal("NoWarningsFoundForDataVolumes"))
			})
		})
	})
})
