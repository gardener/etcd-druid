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

package predicate_test

import (
	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"

	. "github.com/gardener/etcd-druid/pkg/predicate"

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
			pred = LeaseHolderIdentityChange()
		})

		Context("when holder identity is nil", func() {
			BeforeEach(func() {
				obj = &coordinationv1.Lease{}
				oldObj = &coordinationv1.Lease{}
			})

			It("should return false", func() {
				gomega.Expect(pred.Create(createEvent)).To(gomega.BeTrue())
				gomega.Expect(pred.Update(updateEvent)).To(gomega.BeFalse())
				gomega.Expect(pred.Delete(deleteEvent)).To(gomega.BeTrue())
				gomega.Expect(pred.Generic(genericEvent)).To(gomega.BeTrue())
			})
		})

		Context("when holder identity matches", func() {
			BeforeEach(func() {
				obj = &coordinationv1.Lease{
					Spec: coordinationv1.LeaseSpec{
						HolderIdentity: pointer.StringPtr("0"),
					},
				}
				oldObj = &coordinationv1.Lease{
					Spec: coordinationv1.LeaseSpec{
						HolderIdentity: pointer.StringPtr("0"),
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

		Context("when holder identity differs", func() {
			BeforeEach(func() {
				obj = &coordinationv1.Lease{
					Spec: coordinationv1.LeaseSpec{
						HolderIdentity: pointer.StringPtr("5"),
					},
				}
				oldObj = &coordinationv1.Lease{
					Spec: coordinationv1.LeaseSpec{
						HolderIdentity: pointer.StringPtr("0"),
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
						LastError: pointer.StringPtr("foo error"),
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
						LastError: pointer.StringPtr("error"),
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
						LastError: pointer.StringPtr("error"),
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

		Context("when etcd has no reconcile operation annotation and no ready status", func() {
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
				gomega.Expect(pred.Delete(deleteEvent)).To(gomega.BeFalse())
				gomega.Expect(pred.Generic(genericEvent)).To(gomega.BeFalse())
			})
		})

		Context("when etcd has no reconcile operation annotation but ready status", func() {
			BeforeEach(func() {
				obj = &druidv1alpha1.Etcd{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: make(map[string]string),
					},
					Status: druidv1alpha1.EtcdStatus{
						Ready: pointer.BoolPtr(false),
					},
				}
			})

			It("should return true", func() {
				gomega.Expect(pred.Create(createEvent)).To(gomega.BeTrue())
				gomega.Expect(pred.Update(updateEvent)).To(gomega.BeFalse())
				gomega.Expect(pred.Delete(deleteEvent)).To(gomega.BeFalse())
				gomega.Expect(pred.Generic(genericEvent)).To(gomega.BeTrue())
			})
		})

		Context("when etcd has reconcile operation annotation", func() {
			BeforeEach(func() {
				obj = &druidv1alpha1.Etcd{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							v1beta1constants.GardenerOperation: v1beta1constants.GardenerOperationReconcile,
						},
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

		Context("when etcd changed ready status to false", func() {
			BeforeEach(func() {
				oldObj = &druidv1alpha1.Etcd{
					Status: druidv1alpha1.EtcdStatus{
						Ready: nil,
					},
				}
				obj = &druidv1alpha1.Etcd{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							v1beta1constants.GardenerOperation: v1beta1constants.GardenerOperationReconcile,
						},
					},
					Status: druidv1alpha1.EtcdStatus{
						Ready: pointer.BoolPtr(false),
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

		Context("when etcd changed ready status to true", func() {
			BeforeEach(func() {
				oldObj = &druidv1alpha1.Etcd{
					Status: druidv1alpha1.EtcdStatus{
						Ready: nil,
					},
				}
				obj = &druidv1alpha1.Etcd{
					Status: druidv1alpha1.EtcdStatus{
						Ready: pointer.BoolPtr(true),
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
	})

	Describe("#IsSnapshotLease", func() {
		var pred predicate.Predicate

		BeforeEach(func() {
			pred = IsSnapshotLease()
		})

		Context("when lease is delta snapshot lease", func() {
			BeforeEach(func() {
				obj = &coordinationv1.Lease{
					ObjectMeta: metav1.ObjectMeta{
						Name: "foo-delta-snap",
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

		Context("when lease is full snapshot lease", func() {
			BeforeEach(func() {
				obj = &coordinationv1.Lease{
					ObjectMeta: metav1.ObjectMeta{
						Name: "foo-full-snap",
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

		Context("when lease is any other lease", func() {
			BeforeEach(func() {
				obj = &coordinationv1.Lease{
					ObjectMeta: metav1.ObjectMeta{
						Name: "foo",
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
