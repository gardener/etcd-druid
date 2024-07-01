// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package condition_test

import (
	"context"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/health/condition"
	testutils "github.com/gardener/etcd-druid/test/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("DataVolumesReadyCheck", func() {
	Describe("#Check", func() {
		var (
			notFoundErr = apierrors.StatusError{
				ErrStatus: metav1.Status{
					Reason: metav1.StatusReasonNotFound,
				},
			}
			internalErr = apierrors.StatusError{
				ErrStatus: metav1.Status{
					Reason: metav1.StatusReasonInternalError,
				},
			}

			etcd = druidv1alpha1.Etcd{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "default",
				},
				Spec: druidv1alpha1.EtcdSpec{
					Replicas: 1,
				},
			}
			sts = &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test",
					Namespace:       "default",
					OwnerReferences: []metav1.OwnerReference{druidv1alpha1.GetAsOwnerReference(etcd.ObjectMeta)},
				},
				Spec: appsv1.StatefulSetSpec{
					Replicas: pointer.Int32(1),
					VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
						{
							ObjectMeta: metav1.ObjectMeta{
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
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-test-0",
					Namespace: "default",
				},
				Status: corev1.PersistentVolumeClaimStatus{
					Phase: corev1.ClaimPending,
				},
			}
			event = &corev1.Event{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-event",
					Namespace: "default",
				},
				InvolvedObject: corev1.ObjectReference{
					APIVersion: "v1",
					Kind:       "PersistentVolumeClaim",
					Name:       "test-test-0",
					Namespace:  "default",
				},
				Reason:  "FailedMount",
				Message: "MountVolume.SetUp failed for volume \"test-pvc\" : kubernetes.io/csi: mounter.SetUpAt failed to get CSI client: driver name csi.gardener.cloud not found in the list of registered CSI drivers",
				Type:    corev1.EventTypeWarning,
			}
		)

		Context("when error in fetching statefulset", func() {
			It("should return that the condition is unknown", func() {
				cl := testutils.CreateTestFakeClientForObjects(&internalErr, nil, nil, nil, []client.Object{sts}, client.ObjectKeyFromObject(sts))
				check := condition.DataVolumesReadyCheck(cl)
				result := check.Check(context.Background(), etcd)

				Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeDataVolumesReady))
				Expect(result.Status()).To(Equal(druidv1alpha1.ConditionUnknown))
				Expect(result.Reason()).To(Equal("UnableToFetchStatefulSet"))
			})
		})

		Context("when statefulset not found", func() {
			It("should return that the condition is unknown", func() {
				cl := testutils.CreateTestFakeClientForObjects(&notFoundErr, nil, nil, nil, nil, client.ObjectKeyFromObject(sts))
				check := condition.DataVolumesReadyCheck(cl)
				result := check.Check(context.Background(), etcd)

				Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeDataVolumesReady))
				Expect(result.Status()).To(Equal(druidv1alpha1.ConditionUnknown))
				Expect(result.Reason()).To(Equal("StatefulSetNotFound"))
			})
		})

		Context("when error in fetching warning events for PVCs", func() {
			It("should return that the condition is unknown", func() {
				cl := testutils.NewTestClientBuilder().
					WithObjects(sts, pvc).
					RecordErrorForObjectsWithGVK(testutils.ClientMethodList, etcd.Namespace, corev1.SchemeGroupVersion.WithKind("EventList"), &internalErr).
					Build()

				check := condition.DataVolumesReadyCheck(cl)
				result := check.Check(context.Background(), etcd)

				Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeDataVolumesReady))
				Expect(result.Status()).To(Equal(druidv1alpha1.ConditionUnknown))
				Expect(result.Reason()).To(Equal("UnableToFetchWarningEventsForDataVolumes"))
			})
		})

		Context("when warning events found for PVCs", func() {
			It("should return that the condition is false", func() {
				cl := testutils.CreateTestFakeClientForObjects(nil, nil, nil, nil, []client.Object{sts, pvc, event}, client.ObjectKeyFromObject(sts), client.ObjectKeyFromObject(pvc), client.ObjectKeyFromObject(event))
				check := condition.DataVolumesReadyCheck(cl)
				result := check.Check(context.Background(), etcd)

				Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeDataVolumesReady))
				Expect(result.Status()).To(Equal(druidv1alpha1.ConditionFalse))
				Expect(result.Reason()).To(Equal("FoundWarningsForDataVolumes"))
			})
		})

		Context("when no warning events found for PVCs", func() {
			It("should return that the condition is true", func() {
				cl := testutils.CreateTestFakeClientForObjects(nil, nil, nil, nil, []client.Object{sts, pvc}, client.ObjectKeyFromObject(sts), client.ObjectKeyFromObject(pvc))
				check := condition.DataVolumesReadyCheck(cl)
				result := check.Check(context.Background(), etcd)

				Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeDataVolumesReady))
				Expect(result.Status()).To(Equal(druidv1alpha1.ConditionTrue))
				Expect(result.Reason()).To(Equal("NoWarningsFoundForDataVolumes"))
			})
		})
	})
})
