// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package predicate_test

import (
	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	. "github.com/gardener/etcd-druid/controllers/predicate"

	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"

	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

var _ = Describe("Druid Predicate", func() {
	var (
		obj, oldObj client.Object

		createEvent  event.CreateEvent
		updateEvent  event.UpdateEvent
		deleteEvent  event.DeleteEvent
		genericEvent event.GenericEvent
	)

	JustBeforeEach(func() {
		createEvent = event.CreateEvent{
			Object: obj,
		}
		updateEvent = event.UpdateEvent{
			ObjectOld: oldObj,
			ObjectNew: obj,
		}
		deleteEvent = event.DeleteEvent{
			Object: obj,
		}
		genericEvent = event.GenericEvent{
			Object: obj,
		}
	})

	Describe("#StatefulSet", func() {
		var pred predicate.Predicate

		JustBeforeEach(func() {
			pred = StatefulSetStatusChange()
		})

		Context("when status matches", func() {
			BeforeEach(func() {
				obj = &appsv1.StatefulSet{
					Status: appsv1.StatefulSetStatus{
						Replicas: 1,
					},
				}
				oldObj = &appsv1.StatefulSet{
					Status: appsv1.StatefulSetStatus{
						Replicas: 1,
					},
				}
			})

			It("should return false", func() {
				gomega.Expect(pred.Create(createEvent)).To(gomega.BeTrue())
				gomega.Expect(pred.Update(updateEvent)).To(gomega.BeFalse())
				gomega.Expect(pred.Delete(deleteEvent)).To(gomega.BeTrue())
				gomega.Expect(pred.Generic(genericEvent)).To(gomega.BeTrue())
			})
		})

		Context("when status differs", func() {
			BeforeEach(func() {
				obj = &appsv1.StatefulSet{
					Status: appsv1.StatefulSetStatus{
						Replicas: 2,
					},
				}
				oldObj = &appsv1.StatefulSet{
					Status: appsv1.StatefulSetStatus{
						Replicas: 1,
					},
				}
			})

			It("should return true", func() {
				gomega.Expect(pred.Create(createEvent)).To(gomega.BeTrue())
				gomega.Expect(pred.Update(updateEvent)).To(gomega.BeTrue())
				gomega.Expect(pred.Delete(deleteEvent)).To(gomega.BeTrue())
				gomega.Expect(pred.Generic(genericEvent)).To(gomega.BeTrue())
			})
		})
	})

	Describe("#Lease", func() {
		var pred predicate.Predicate

		JustBeforeEach(func() {
			pred = SnapshotRevisionChanged()
		})

		Context("when holder identity is nil for delta snap leases", func() {
			BeforeEach(func() {
				obj = &coordinationv1.Lease{
					ObjectMeta: metav1.ObjectMeta{
						Name: "foo-delta-snap",
					},
				}
				oldObj = &coordinationv1.Lease{
					ObjectMeta: metav1.ObjectMeta{
						Name: "foo-delta-snap",
					},
				}
			})

			It("should return false", func() {
				gomega.Expect(pred.Create(createEvent)).To(gomega.BeTrue())
				gomega.Expect(pred.Update(updateEvent)).To(gomega.BeFalse())
				gomega.Expect(pred.Delete(deleteEvent)).To(gomega.BeFalse())
				gomega.Expect(pred.Generic(genericEvent)).To(gomega.BeFalse())
			})
		})

		Context("when holder identity matches for delta snap leases", func() {
			BeforeEach(func() {
				obj = &coordinationv1.Lease{
					ObjectMeta: metav1.ObjectMeta{
						Name: "foo-delta-snap",
					},
					Spec: coordinationv1.LeaseSpec{
						HolderIdentity: pointer.String("0"),
					},
				}
				oldObj = &coordinationv1.Lease{
					ObjectMeta: metav1.ObjectMeta{
						Name: "foo-delta-snap",
					},
					Spec: coordinationv1.LeaseSpec{
						HolderIdentity: pointer.String("0"),
					},
				}
			})

			It("should return false", func() {
				gomega.Expect(pred.Create(createEvent)).To(gomega.BeTrue())
				gomega.Expect(pred.Update(updateEvent)).To(gomega.BeFalse())
				gomega.Expect(pred.Delete(deleteEvent)).To(gomega.BeFalse())
				gomega.Expect(pred.Generic(genericEvent)).To(gomega.BeFalse())
			})
		})

		Context("when holder identity differs for delta snap leases", func() {
			BeforeEach(func() {
				obj = &coordinationv1.Lease{
					ObjectMeta: metav1.ObjectMeta{
						Name: "foo-delta-snap",
					},
					Spec: coordinationv1.LeaseSpec{
						HolderIdentity: pointer.String("5"),
					},
				}
				oldObj = &coordinationv1.Lease{
					ObjectMeta: metav1.ObjectMeta{
						Name: "foo-delta-snap",
					},
					Spec: coordinationv1.LeaseSpec{
						HolderIdentity: pointer.String("0"),
					},
				}
			})

			It("should return true", func() {
				gomega.Expect(pred.Create(createEvent)).To(gomega.BeTrue())
				gomega.Expect(pred.Update(updateEvent)).To(gomega.BeTrue())
				gomega.Expect(pred.Delete(deleteEvent)).To(gomega.BeFalse())
				gomega.Expect(pred.Generic(genericEvent)).To(gomega.BeFalse())
			})
		})

		Context("when holder identity is nil for full snap leases", func() {
			BeforeEach(func() {
				obj = &coordinationv1.Lease{
					ObjectMeta: metav1.ObjectMeta{
						Name: "foo-full-snap",
					},
				}
				oldObj = &coordinationv1.Lease{
					ObjectMeta: metav1.ObjectMeta{
						Name: "foo-full-snap",
					},
				}
			})

			It("should return false", func() {
				gomega.Expect(pred.Create(createEvent)).To(gomega.BeTrue())
				gomega.Expect(pred.Update(updateEvent)).To(gomega.BeFalse())
				gomega.Expect(pred.Delete(deleteEvent)).To(gomega.BeFalse())
				gomega.Expect(pred.Generic(genericEvent)).To(gomega.BeFalse())
			})
		})

		Context("when holder identity matches for full snap leases", func() {
			BeforeEach(func() {
				obj = &coordinationv1.Lease{
					ObjectMeta: metav1.ObjectMeta{
						Name: "foo-full-snap",
					},
					Spec: coordinationv1.LeaseSpec{
						HolderIdentity: pointer.String("0"),
					},
				}
				oldObj = &coordinationv1.Lease{
					ObjectMeta: metav1.ObjectMeta{
						Name: "foo-full-snap",
					},
					Spec: coordinationv1.LeaseSpec{
						HolderIdentity: pointer.String("0"),
					},
				}
			})

			It("should return false", func() {
				gomega.Expect(pred.Create(createEvent)).To(gomega.BeTrue())
				gomega.Expect(pred.Update(updateEvent)).To(gomega.BeFalse())
				gomega.Expect(pred.Delete(deleteEvent)).To(gomega.BeFalse())
				gomega.Expect(pred.Generic(genericEvent)).To(gomega.BeFalse())
			})
		})

		Context("when holder identity differs for full snap leases", func() {
			BeforeEach(func() {
				obj = &coordinationv1.Lease{
					ObjectMeta: metav1.ObjectMeta{
						Name: "foo-full-snap",
					},
					Spec: coordinationv1.LeaseSpec{
						HolderIdentity: pointer.String("5"),
					},
				}
				oldObj = &coordinationv1.Lease{
					ObjectMeta: metav1.ObjectMeta{
						Name: "foo-full-snap",
					},
					Spec: coordinationv1.LeaseSpec{
						HolderIdentity: pointer.String("0"),
					},
				}
			})

			It("should return true", func() {
				gomega.Expect(pred.Create(createEvent)).To(gomega.BeTrue())
				gomega.Expect(pred.Update(updateEvent)).To(gomega.BeTrue())
				gomega.Expect(pred.Delete(deleteEvent)).To(gomega.BeFalse())
				gomega.Expect(pred.Generic(genericEvent)).To(gomega.BeFalse())
			})
		})

		Context("when holder identity differs for any other leases", func() {
			BeforeEach(func() {
				obj = &coordinationv1.Lease{
					ObjectMeta: metav1.ObjectMeta{
						Name: "foo",
					},
					Spec: coordinationv1.LeaseSpec{
						HolderIdentity: pointer.String("5"),
					},
				}
				oldObj = &coordinationv1.Lease{
					ObjectMeta: metav1.ObjectMeta{
						Name: "foo",
					},
					Spec: coordinationv1.LeaseSpec{
						HolderIdentity: pointer.String("0"),
					},
				}
			})

			It("should return false", func() {
				gomega.Expect(pred.Create(createEvent)).To(gomega.BeFalse())
				gomega.Expect(pred.Update(updateEvent)).To(gomega.BeFalse())
				gomega.Expect(pred.Delete(deleteEvent)).To(gomega.BeFalse())
				gomega.Expect(pred.Generic(genericEvent)).To(gomega.BeFalse())
			})
		})
	})

	Describe("#Job", func() {
		var pred predicate.Predicate

		JustBeforeEach(func() {
			pred = JobStatusChanged()
		})

		Context("when status matches", func() {
			BeforeEach(func() {
				now := metav1.Now()
				obj = &batchv1.Job{
					Status: batchv1.JobStatus{
						CompletionTime: &now,
					},
				}
				oldObj = &batchv1.Job{
					Status: batchv1.JobStatus{
						CompletionTime: &now,
					},
				}
			})

			It("should return false", func() {
				gomega.Expect(pred.Create(createEvent)).To(gomega.BeFalse())
				gomega.Expect(pred.Update(updateEvent)).To(gomega.BeFalse())
				gomega.Expect(pred.Delete(deleteEvent)).To(gomega.BeFalse())
				gomega.Expect(pred.Generic(genericEvent)).To(gomega.BeFalse())
			})
		})

		Context("when status differs", func() {
			BeforeEach(func() {
				now := metav1.Now()
				obj = &batchv1.Job{
					Status: batchv1.JobStatus{
						CompletionTime: nil,
					},
				}
				oldObj = &batchv1.Job{
					Status: batchv1.JobStatus{
						CompletionTime: &now,
					},
				}
			})

			It("should return true", func() {
				gomega.Expect(pred.Create(createEvent)).To(gomega.BeFalse())
				gomega.Expect(pred.Update(updateEvent)).To(gomega.BeTrue())
				gomega.Expect(pred.Delete(deleteEvent)).To(gomega.BeFalse())
				gomega.Expect(pred.Generic(genericEvent)).To(gomega.BeFalse())
			})
		})
	})

	Describe("#LastOperationNotSuccessful", func() {
		var pred predicate.Predicate

		JustBeforeEach(func() {
			pred = LastOperationNotSuccessful()
		})

		Context("when last error is not set", func() {
			BeforeEach(func() {
				obj = &druidv1alpha1.Etcd{
					Status: druidv1alpha1.EtcdStatus{
						LastError: nil,
					},
				}
			})

			It("should return false", func() {
				gomega.Expect(pred.Create(createEvent)).To(gomega.BeFalse())
				gomega.Expect(pred.Update(updateEvent)).To(gomega.BeFalse())
				gomega.Expect(pred.Delete(deleteEvent)).To(gomega.BeFalse())
				gomega.Expect(pred.Generic(genericEvent)).To(gomega.BeFalse())
			})
		})

		Context("when last error is set", func() {
			BeforeEach(func() {
				obj = &druidv1alpha1.Etcd{
					Status: druidv1alpha1.EtcdStatus{
						LastError: pointer.String("foo error"),
					},
				}
			})

			It("should return true", func() {
				gomega.Expect(pred.Create(createEvent)).To(gomega.BeTrue())
				gomega.Expect(pred.Update(updateEvent)).To(gomega.BeTrue())
				gomega.Expect(pred.Delete(deleteEvent)).To(gomega.BeTrue())
				gomega.Expect(pred.Generic(genericEvent)).To(gomega.BeTrue())
			})
		})
	})

	Describe("#HasOperationAnnotation", func() {
		var pred predicate.Predicate

		JustBeforeEach(func() {
			pred = HasOperationAnnotation()
		})

		Context("when has no operation annotation", func() {
			BeforeEach(func() {
				obj = &druidv1alpha1.Etcd{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: make(map[string]string),
					},
				}
			})

			It("should return false", func() {
				gomega.Expect(pred.Create(createEvent)).To(gomega.BeFalse())
				gomega.Expect(pred.Update(updateEvent)).To(gomega.BeFalse())
				gomega.Expect(pred.Delete(deleteEvent)).To(gomega.BeTrue())
				gomega.Expect(pred.Generic(genericEvent)).To(gomega.BeFalse())
			})
		})

		Context("when has operation annotation", func() {
			BeforeEach(func() {
				obj = &druidv1alpha1.Etcd{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							v1beta1constants.GardenerOperation: v1beta1constants.GardenerOperationReconcile,
						},
					},
				}
			})

			It("should return true", func() {
				gomega.Expect(pred.Create(createEvent)).To(gomega.BeTrue())
				gomega.Expect(pred.Update(updateEvent)).To(gomega.BeTrue())
				gomega.Expect(pred.Delete(deleteEvent)).To(gomega.BeTrue())
				gomega.Expect(pred.Generic(genericEvent)).To(gomega.BeTrue())
			})
		})
	})

	Describe("#OR", func() {
		var pred predicate.Predicate

		JustBeforeEach(func() {
			pred = predicate.Or(
				HasOperationAnnotation(),
				LastOperationNotSuccessful(),
			)
		})

		Context("when has neither operation annotation nor last error", func() {
			BeforeEach(func() {
				obj = &druidv1alpha1.Etcd{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: make(map[string]string),
					},
				}
			})

			It("should return false", func() {
				gomega.Expect(pred.Create(createEvent)).To(gomega.BeFalse())
				gomega.Expect(pred.Update(updateEvent)).To(gomega.BeFalse())
				gomega.Expect(pred.Delete(deleteEvent)).To(gomega.BeTrue())
				gomega.Expect(pred.Generic(genericEvent)).To(gomega.BeFalse())
			})
		})

		Context("when has operation annotation", func() {
			BeforeEach(func() {
				obj = &druidv1alpha1.Etcd{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							v1beta1constants.GardenerOperation: v1beta1constants.GardenerOperationReconcile,
						},
					},
				}
			})

			It("should return true", func() {
				gomega.Expect(pred.Create(createEvent)).To(gomega.BeTrue())
				gomega.Expect(pred.Update(updateEvent)).To(gomega.BeTrue())
				gomega.Expect(pred.Delete(deleteEvent)).To(gomega.BeTrue())
				gomega.Expect(pred.Generic(genericEvent)).To(gomega.BeTrue())
			})
		})

		Context("when has last error", func() {
			BeforeEach(func() {
				obj = &druidv1alpha1.Etcd{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: make(map[string]string),
					},
					Status: druidv1alpha1.EtcdStatus{
						LastError: pointer.String("error"),
					},
				}
			})

			It("should return true", func() {
				gomega.Expect(pred.Create(createEvent)).To(gomega.BeTrue())
				gomega.Expect(pred.Update(updateEvent)).To(gomega.BeTrue())
				gomega.Expect(pred.Delete(deleteEvent)).To(gomega.BeTrue())
				gomega.Expect(pred.Generic(genericEvent)).To(gomega.BeTrue())
			})
		})

		Context("when has both operation annotation and last error", func() {
			BeforeEach(func() {
				obj = &druidv1alpha1.Etcd{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							v1beta1constants.GardenerOperation: v1beta1constants.GardenerOperationReconcile,
						},
					},
					Status: druidv1alpha1.EtcdStatus{
						LastError: pointer.String("error"),
					},
				}
			})

			It("should return true", func() {
				gomega.Expect(pred.Create(createEvent)).To(gomega.BeTrue())
				gomega.Expect(pred.Update(updateEvent)).To(gomega.BeTrue())
				gomega.Expect(pred.Delete(deleteEvent)).To(gomega.BeTrue())
				gomega.Expect(pred.Generic(genericEvent)).To(gomega.BeTrue())
			})
		})
	})

	Describe("#EtcdReconciliationFinished", func() {
		var pred predicate.Predicate

		BeforeEach(func() {
			pred = EtcdReconciliationFinished(false)
		})

		Context("when etcd has no reconcile operation annotation and observedGeneration is not present", func() {
			BeforeEach(func() {
				obj = &druidv1alpha1.Etcd{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: make(map[string]string),
						Generation:  2,
					},
				}
			})

			It("should return false", func() {
				gomega.Expect(pred.Create(createEvent)).To(gomega.BeFalse())
				gomega.Expect(pred.Update(updateEvent)).To(gomega.BeFalse())
				gomega.Expect(pred.Delete(deleteEvent)).To(gomega.BeFalse())
				gomega.Expect(pred.Generic(genericEvent)).To(gomega.BeFalse())
			})
		})

		Context("when etcd has no reconcile operation annotation and observedGeneration is not equal to generation", func() {
			BeforeEach(func() {
				obj = &druidv1alpha1.Etcd{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: make(map[string]string),
						Generation:  2,
					},
					Status: druidv1alpha1.EtcdStatus{
						ObservedGeneration: pointer.Int64(1),
					},
				}
			})

			It("should return false", func() {
				gomega.Expect(pred.Create(createEvent)).To(gomega.BeFalse())
				gomega.Expect(pred.Update(updateEvent)).To(gomega.BeFalse())
				gomega.Expect(pred.Delete(deleteEvent)).To(gomega.BeFalse())
				gomega.Expect(pred.Generic(genericEvent)).To(gomega.BeFalse())
			})
		})

		Context("when etcd has no reconcile operation annotation and observedGeneration is equal to generation", func() {
			BeforeEach(func() {
				obj = &druidv1alpha1.Etcd{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: make(map[string]string),
						Generation:  2,
					},
					Status: druidv1alpha1.EtcdStatus{
						ObservedGeneration: pointer.Int64(2),
					},
				}
			})

			It("should return true", func() {
				gomega.Expect(pred.Create(createEvent)).To(gomega.BeTrue())
				gomega.Expect(pred.Update(updateEvent)).To(gomega.BeTrue())
				gomega.Expect(pred.Delete(deleteEvent)).To(gomega.BeFalse())
				gomega.Expect(pred.Generic(genericEvent)).To(gomega.BeTrue())
			})
		})

		Context("when etcd has reconcile operation annotation and observedGeneration is not present", func() {
			BeforeEach(func() {
				obj = &druidv1alpha1.Etcd{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							v1beta1constants.GardenerOperation: v1beta1constants.GardenerOperationReconcile,
						},
						Generation: 1,
					},
				}
			})

			It("should return false", func() {
				gomega.Expect(pred.Create(createEvent)).To(gomega.BeFalse())
				gomega.Expect(pred.Update(updateEvent)).To(gomega.BeFalse())
				gomega.Expect(pred.Delete(deleteEvent)).To(gomega.BeFalse())
				gomega.Expect(pred.Generic(genericEvent)).To(gomega.BeFalse())
			})
		})

		Context("when etcd has reconcile operation annotation and observedGeneration is not equal to generation", func() {
			BeforeEach(func() {
				obj = &druidv1alpha1.Etcd{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							v1beta1constants.GardenerOperation: v1beta1constants.GardenerOperationReconcile,
						},
						Generation: 2,
					},
					Status: druidv1alpha1.EtcdStatus{
						ObservedGeneration: pointer.Int64(1),
					},
				}
			})

			It("should return false", func() {
				gomega.Expect(pred.Create(createEvent)).To(gomega.BeFalse())
				gomega.Expect(pred.Update(updateEvent)).To(gomega.BeFalse())
				gomega.Expect(pred.Delete(deleteEvent)).To(gomega.BeFalse())
				gomega.Expect(pred.Generic(genericEvent)).To(gomega.BeFalse())
			})
		})

		Context("when etcd has reconcile operation annotation and observedGeneration is equal to generation", func() {
			BeforeEach(func() {
				obj = &druidv1alpha1.Etcd{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							v1beta1constants.GardenerOperation: v1beta1constants.GardenerOperationReconcile,
						},
						Generation: 2,
					},
					Status: druidv1alpha1.EtcdStatus{
						ObservedGeneration: pointer.Int64(2),
					},
				}
			})

			It("should return false", func() {
				gomega.Expect(pred.Create(createEvent)).To(gomega.BeFalse())
				gomega.Expect(pred.Update(updateEvent)).To(gomega.BeFalse())
				gomega.Expect(pred.Delete(deleteEvent)).To(gomega.BeFalse())
				gomega.Expect(pred.Generic(genericEvent)).To(gomega.BeFalse())
			})
		})
	})
})
