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
	. "github.com/onsi/ginkgo"
	"github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"

	. "github.com/gardener/etcd-druid/pkg/predicate"
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
				predicate := StatefulSetStatusChange()

				gomega.Expect(predicate.Create(createEvent)).To(gomega.BeTrue())
				gomega.Expect(predicate.Update(updateEvent)).To(gomega.BeFalse())
				gomega.Expect(predicate.Delete(deleteEvent)).To(gomega.BeTrue())
				gomega.Expect(predicate.Generic(genericEvent)).To(gomega.BeTrue())
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
				predicate := StatefulSetStatusChange()

				gomega.Expect(predicate.Create(createEvent)).To(gomega.BeTrue())
				gomega.Expect(predicate.Update(updateEvent)).To(gomega.BeTrue())
				gomega.Expect(predicate.Delete(deleteEvent)).To(gomega.BeTrue())
				gomega.Expect(predicate.Generic(genericEvent)).To(gomega.BeTrue())
			})
		})
	})

	Describe("#GenerationChanged", func() {
		Context("when generation matches", func() {
			BeforeEach(func() {
				obj = &druidv1alpha1.Etcd{
					ObjectMeta: metav1.ObjectMeta{
						Generation: 1,
					},
					Status: druidv1alpha1.EtcdStatus{
						ObservedGeneration: pointer.Int64Ptr(1),
					},
				}
			})

			It("should return false", func() {
				predicate := GenerationChangedPredicate{}

				gomega.Expect(predicate.Create(createEvent)).To(gomega.BeFalse())
				gomega.Expect(predicate.Update(updateEvent)).To(gomega.BeFalse())
				gomega.Expect(predicate.Delete(deleteEvent)).To(gomega.BeTrue())
				gomega.Expect(predicate.Generic(genericEvent)).To(gomega.BeTrue())
			})
		})

		Context("when generation differs", func() {
			BeforeEach(func() {
				obj = &druidv1alpha1.Etcd{
					ObjectMeta: metav1.ObjectMeta{
						Generation: 2,
					},
					Status: druidv1alpha1.EtcdStatus{
						ObservedGeneration: pointer.Int64Ptr(1),
					},
				}
			})

			It("should return true", func() {
				predicate := GenerationChangedPredicate{}

				gomega.Expect(predicate.Create(createEvent)).To(gomega.BeTrue())
				gomega.Expect(predicate.Update(updateEvent)).To(gomega.BeTrue())
				gomega.Expect(predicate.Delete(deleteEvent)).To(gomega.BeTrue())
				gomega.Expect(predicate.Generic(genericEvent)).To(gomega.BeTrue())
			})
		})

		Describe("#LastOperationNotSuccessful", func() {
			Context("when last error is not set", func() {
				BeforeEach(func() {
					obj = &druidv1alpha1.Etcd{
						Status: druidv1alpha1.EtcdStatus{
							LastError: nil,
						},
					}
				})

				It("should return false", func() {
					predicate := LastOperationNotSuccessful()

					gomega.Expect(predicate.Create(createEvent)).To(gomega.BeFalse())
					gomega.Expect(predicate.Update(updateEvent)).To(gomega.BeFalse())
					gomega.Expect(predicate.Delete(deleteEvent)).To(gomega.BeFalse())
					gomega.Expect(predicate.Generic(genericEvent)).To(gomega.BeFalse())
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
					predicate := LastOperationNotSuccessful()

					gomega.Expect(predicate.Create(createEvent)).To(gomega.BeTrue())
					gomega.Expect(predicate.Update(updateEvent)).To(gomega.BeTrue())
					gomega.Expect(predicate.Delete(deleteEvent)).To(gomega.BeTrue())
					gomega.Expect(predicate.Generic(genericEvent)).To(gomega.BeTrue())
				})
			})
		})

		Describe("#HasOperationAnnotation", func() {
			Context("when has no operation annotation", func() {
				BeforeEach(func() {
					obj = &druidv1alpha1.Etcd{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: make(map[string]string),
						},
					}
				})

				It("should return false", func() {
					predicate := HasOperationAnnotation()

					gomega.Expect(predicate.Create(createEvent)).To(gomega.BeFalse())
					gomega.Expect(predicate.Update(updateEvent)).To(gomega.BeFalse())
					gomega.Expect(predicate.Delete(deleteEvent)).To(gomega.BeTrue())
					gomega.Expect(predicate.Generic(genericEvent)).To(gomega.BeFalse())
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

				It("should return false", func() {
					predicate := HasOperationAnnotation()

					gomega.Expect(predicate.Create(createEvent)).To(gomega.BeTrue())
					gomega.Expect(predicate.Update(updateEvent)).To(gomega.BeTrue())
					gomega.Expect(predicate.Delete(deleteEvent)).To(gomega.BeTrue())
					gomega.Expect(predicate.Generic(genericEvent)).To(gomega.BeTrue())
				})
			})
		})

		Describe("#OR", func() {
			Context("when has no operation annotation", func() {
				BeforeEach(func() {
					obj = &druidv1alpha1.Etcd{
						ObjectMeta: metav1.ObjectMeta{
							Annotations: make(map[string]string),
						},
					}
				})

				It("should return false", func() {
					predicate := HasOperationAnnotation()

					gomega.Expect(predicate.Create(createEvent)).To(gomega.BeFalse())
					gomega.Expect(predicate.Update(updateEvent)).To(gomega.BeFalse())
					gomega.Expect(predicate.Delete(deleteEvent)).To(gomega.BeTrue())
					gomega.Expect(predicate.Generic(genericEvent)).To(gomega.BeFalse())
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

				It("should return false", func() {
					predicate := HasOperationAnnotation()

					gomega.Expect(predicate.Create(createEvent)).To(gomega.BeTrue())
					gomega.Expect(predicate.Update(updateEvent)).To(gomega.BeTrue())
					gomega.Expect(predicate.Delete(deleteEvent)).To(gomega.BeTrue())
					gomega.Expect(predicate.Generic(genericEvent)).To(gomega.BeTrue())
				})
			})
		})
	})
})
