// Copyright (c) 2021 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package validation_test

import (
	"fmt"

	"github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/api/validation"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	"github.com/onsi/gomega/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/pointer"
)

var _ = Describe("Etcd validation tests", func() {
	var task *v1alpha1.EtcdCopyBackupsTask

	BeforeEach(func() {
		task = &v1alpha1.EtcdCopyBackupsTask{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: v1alpha1.EtcdCopyBackupsTaskSpec{
				SourceStore: v1alpha1.StoreSpec{
					Prefix:   fmt.Sprintf("%s--%s/%s", namespace, uuid, name),
					Provider: (*v1alpha1.StorageProvider)(pointer.StringPtr(provider)),
				},
				TargetStore: v1alpha1.StoreSpec{
					Prefix:   fmt.Sprintf("%s--%s/%s", namespace, uuid, name),
					Provider: (*v1alpha1.StorageProvider)(pointer.StringPtr(provider)),
				},
			},
		}
	})

	Describe("#ValidateEtcdCopyBackupsTask", func() {
		It("should forbid empty EtcdCopyBackupsTask resources", func() {
			errorList := validation.ValidateEtcdCopyBackupsTask(&v1alpha1.EtcdCopyBackupsTask{})

			Expect(errorList).To(ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{
				"Type":  Equal(field.ErrorTypeRequired),
				"Field": Equal("metadata.name"),
			})), PointTo(MatchFields(IgnoreExtras, Fields{
				"Type":  Equal(field.ErrorTypeRequired),
				"Field": Equal("metadata.namespace"),
			}))))
		})

		DescribeTable("validate spec.sourceStore and spec.targetStore",
			func(sourceStore, targetStore *v1alpha1.StoreSpec, m types.GomegaMatcher) {
				task.Spec.SourceStore = *sourceStore
				task.Spec.TargetStore = *targetStore
				Expect(validation.ValidateEtcdCopyBackupsTask(task)).To(m)
			},

			Entry("should forbid invalid spec.sourceStore and spec.targetStore", &v1alpha1.StoreSpec{
				Prefix:   "invalid",
				Provider: (*v1alpha1.StorageProvider)(pointer.StringPtr("invalid")),
			}, &v1alpha1.StoreSpec{
				Prefix:   "invalid",
				Provider: (*v1alpha1.StorageProvider)(pointer.StringPtr("invalid")),
			}, ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{
				"Type":  Equal(field.ErrorTypeInvalid),
				"Field": Equal("spec.sourceStore.prefix"),
			})), PointTo(MatchFields(IgnoreExtras, Fields{
				"Type":  Equal(field.ErrorTypeInvalid),
				"Field": Equal("spec.sourceStore.provider"),
			})), PointTo(MatchFields(IgnoreExtras, Fields{
				"Type":  Equal(field.ErrorTypeInvalid),
				"Field": Equal("spec.targetStore.prefix"),
			})), PointTo(MatchFields(IgnoreExtras, Fields{
				"Type":  Equal(field.ErrorTypeInvalid),
				"Field": Equal("spec.targetStore.provider"),
			})))),

			Entry("should allow valid spec.sourceStore and spec.targetStore", &v1alpha1.StoreSpec{
				Prefix:   fmt.Sprintf("%s--%s/%s", namespace, uuid, name),
				Provider: (*v1alpha1.StorageProvider)(pointer.StringPtr(provider)),
			}, &v1alpha1.StoreSpec{
				Prefix:   fmt.Sprintf("%s--%s/%s", namespace, uuid, name),
				Provider: (*v1alpha1.StorageProvider)(pointer.StringPtr(provider)),
			}, BeNil()),
		)
	})

	Describe("#ValidateEtcdCopyBackupsTaskUpdate", func() {
		It("should prevent updating anything if deletion timestamp is set", func() {
			now := metav1.Now()
			task.DeletionTimestamp = &now
			task.ResourceVersion = "1"

			newTask := task.DeepCopy()
			newTask.ResourceVersion = "2"
			newTask.Spec.SourceStore.Container = pointer.StringPtr("foo")

			errList := validation.ValidateEtcdCopyBackupsTaskUpdate(newTask, task)

			Expect(errList).To(ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{
				"Type":  Equal(field.ErrorTypeInvalid),
				"Field": Equal("spec"),
			}))))
		})

		It("should prevent updating spec.sourceStore.prefix and spec.targetStore.prefix", func() {
			task.ResourceVersion = "1"

			newTask := task.DeepCopy()
			newTask.ResourceVersion = "2"
			newTask.Spec.SourceStore.Prefix = namespace + "/" + name
			newTask.Spec.TargetStore.Prefix = namespace + "/" + name

			errList := validation.ValidateEtcdCopyBackupsTaskUpdate(newTask, task)

			Expect(errList).To(ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{
				"Type":  Equal(field.ErrorTypeInvalid),
				"Field": Equal("spec.sourceStore.prefix"),
			})), PointTo(MatchFields(IgnoreExtras, Fields{
				"Type":  Equal(field.ErrorTypeInvalid),
				"Field": Equal("spec.targetStore.prefix"),
			}))))
		})

		It("should allow updating everything else", func() {
			task.ResourceVersion = "1"

			newTask := task.DeepCopy()
			newTask.ResourceVersion = "2"
			newTask.Spec.SourceStore.Container = pointer.StringPtr("foo")
			newTask.Spec.SourceStore.Provider = (*v1alpha1.StorageProvider)(pointer.StringPtr("gcp"))
			newTask.Spec.TargetStore.Container = pointer.StringPtr("bar")
			newTask.Spec.TargetStore.Provider = (*v1alpha1.StorageProvider)(pointer.StringPtr("gcp"))

			errList := validation.ValidateEtcdCopyBackupsTaskUpdate(newTask, task)

			Expect(errList).To(BeEmpty())
		})
	})
})
