// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package condition_test

import (
	"context"
	"k8s.io/utils/pointer"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/health/condition"
	testutils "github.com/gardener/etcd-druid/test/utils"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("AllMembersUpdatedCheck", func() {
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
				Spec: druidv1alpha1.EtcdSpec{},
			}
			sts = &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test",
					Namespace:       "default",
					OwnerReferences: []metav1.OwnerReference{druidv1alpha1.GetAsOwnerReference(etcd.ObjectMeta)},
				},
				Spec:   appsv1.StatefulSetSpec{},
				Status: appsv1.StatefulSetStatus{},
			}
		)

		Context("when error in fetching statefulset", func() {
			It("should return that the condition is unknown", func() {
				cl := testutils.CreateTestFakeClientForObjects(&internalErr, nil, nil, nil, []client.Object{sts}, client.ObjectKeyFromObject(sts))
				check := condition.AllMembersUpdatedCheck(cl)
				result := check.Check(context.Background(), etcd)

				Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeAllMembersUpdated))
				Expect(result.Status()).To(Equal(druidv1alpha1.ConditionUnknown))
				Expect(result.Reason()).To(Equal("UnableToFetchStatefulSet"))
			})
		})

		Context("when statefulset not found", func() {
			It("should return that the condition is unknown", func() {
				cl := testutils.CreateTestFakeClientForObjects(&notFoundErr, nil, nil, nil, nil, client.ObjectKeyFromObject(sts))
				check := condition.AllMembersUpdatedCheck(cl)
				result := check.Check(context.Background(), etcd)

				Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeAllMembersUpdated))
				Expect(result.Status()).To(Equal(druidv1alpha1.ConditionUnknown))
				Expect(result.Reason()).To(Equal("StatefulSetNotFound"))
			})
		})

		Context("when statefulset is found", func() {
			Context("when sts observed generation does not match sts generation", func() {
				It("should return that the condition is false", func() {
					sts.Status.ObservedGeneration = 1
					sts.Generation = 2

					cl := testutils.CreateTestFakeClientForObjects(nil, nil, nil, nil, []client.Object{sts}, client.ObjectKeyFromObject(sts))
					check := condition.AllMembersUpdatedCheck(cl)
					result := check.Check(context.Background(), etcd)

					Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeAllMembersUpdated))
					Expect(result.Status()).To(Equal(druidv1alpha1.ConditionFalse))
					Expect(result.Reason()).To(Equal("NotAllMembersUpdated"))
				})
			})

			Context("when sts updated replicas does not match spec replicas", func() {
				It("should return that the condition is false", func() {
					sts.Status.UpdatedReplicas = 1
					sts.Spec.Replicas = pointer.Int32(2)

					cl := testutils.CreateTestFakeClientForObjects(nil, nil, nil, nil, []client.Object{sts}, client.ObjectKeyFromObject(sts))
					check := condition.AllMembersUpdatedCheck(cl)
					result := check.Check(context.Background(), etcd)

					Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeAllMembersUpdated))
					Expect(result.Status()).To(Equal(druidv1alpha1.ConditionFalse))
					Expect(result.Reason()).To(Equal("NotAllMembersUpdated"))
				})
			})

			Context("when sts update revision does not match sts current revision", func() {
				It("should return that the condition is false", func() {
					sts.Spec.Replicas = pointer.Int32(2)
					sts.Status.UpdatedReplicas = 2
					sts.Status.UpdateRevision = "123456"
					sts.Status.CurrentRevision = "654321"

					cl := testutils.CreateTestFakeClientForObjects(nil, nil, nil, nil, []client.Object{sts}, client.ObjectKeyFromObject(sts))
					check := condition.AllMembersUpdatedCheck(cl)
					result := check.Check(context.Background(), etcd)

					Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeAllMembersUpdated))
					Expect(result.Status()).To(Equal(druidv1alpha1.ConditionFalse))
					Expect(result.Reason()).To(Equal("NotAllMembersUpdated"))
				})
			})

			Context("when all sts replicas are updated", func() {
				It("should return that the condition is true", func() {
					sts.Status.ObservedGeneration = 1
					sts.Generation = 1
					sts.Status.UpdatedReplicas = 1
					sts.Spec.Replicas = pointer.Int32(1)
					sts.Status.UpdateRevision = "123456"
					sts.Status.CurrentRevision = "123456"

					cl := testutils.CreateTestFakeClientForObjects(nil, nil, nil, nil, []client.Object{sts}, client.ObjectKeyFromObject(sts))
					check := condition.AllMembersUpdatedCheck(cl)
					result := check.Check(context.Background(), etcd)

					Expect(result.ConditionType()).To(Equal(druidv1alpha1.ConditionTypeAllMembersUpdated))
					Expect(result.Status()).To(Equal(druidv1alpha1.ConditionTrue))
					Expect(result.Reason()).To(Equal("AllMembersUpdated"))
				})
			})
		})
	})
})
