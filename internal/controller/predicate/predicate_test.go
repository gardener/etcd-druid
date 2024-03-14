// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package predicate_test

import (
	. "github.com/gardener/etcd-druid/internal/controller/predicate"

	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

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
})
