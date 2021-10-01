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

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	"github.com/onsi/gomega/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/pointer"
)

var _ = Describe("Etcd validation tests", func() {
	Describe("#ValidateEtcd", func() {
		It("should forbid empty Etcd resources", func() {
			errorList := validation.ValidateEtcd(new(v1alpha1.Etcd))

			Expect(errorList).To(ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{
				"Type":  Equal(field.ErrorTypeRequired),
				"Field": Equal("metadata.name"),
			})), PointTo(MatchFields(IgnoreExtras, Fields{
				"Type":  Equal(field.ErrorTypeRequired),
				"Field": Equal("metadata.namespace"),
			}))))
		})

		DescribeTable("#BackupStorePrefix",
			func(e *v1alpha1.Etcd, m types.GomegaMatcher) { Expect(validation.ValidateEtcd(e)).To(m) },

			Entry("should forbid some random name", newEtcd("non-valid"), ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{
				"Type":  Equal(field.ErrorTypeInvalid),
				"Field": Equal("spec.backup.store.prefix"),
			})))),
			Entry("should forbid name equal to etcd's name", newEtcd("etcd-name"), ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{
				"Type":  Equal(field.ErrorTypeInvalid),
				"Field": Equal("spec.backup.store.prefix"),
			})))),
			Entry("should validate succesfully the object", newEtcd(""), BeNil()),
		)

	})

	Describe("#ValidateEtcdUpdate", func() {
		It("should prevent updating anything if deletion time stamp is set", func() {
			old := newEtcd("")

			now := metav1.Now()
			old.DeletionTimestamp = &now
			old.ResourceVersion = "1"

			newetcd := newUpdatableEtcd(old)
			newetcd.DeletionTimestamp = &now
			newetcd.Spec.Backup.Port = pointer.Int32Ptr(42)

			errList := validation.ValidateEtcdUpdate(newetcd, old)

			Expect(errList).To(ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{
				"Type":  Equal(field.ErrorTypeInvalid),
				"Field": Equal("spec"),
			}))))
		})

		It("should allow updating everything else", func() {
			old := newEtcd("")
			old.ResourceVersion = "1"

			newetcd := newUpdatableEtcd(old)
			newetcd.Status = v1alpha1.EtcdStatus{
				Replicas: int32(42),
			}
			newetcd.Spec.Etcd = v1alpha1.EtcdConfig{}
			newetcd.Spec.Common = v1alpha1.SharedConfig{}
			newetcd.Spec.Replicas = 42

			errList := validation.ValidateEtcdUpdate(newetcd, old)

			Expect(errList).To(BeEmpty())
		})
	})
})

func newUpdatableEtcd(e *v1alpha1.Etcd) *v1alpha1.Etcd {
	res := e.DeepCopy()
	res.ResourceVersion = "2"

	return res
}

func newEtcd(prefix string) *v1alpha1.Etcd {
	var (
		name = "etcd-name"
		ns   = "shoot1--ns"
	)

	if prefix == "" {
		prefix = fmt.Sprintf("%s--F1A38EDD-E506-412A-82E6-E0FA839D0707/%s", ns, name)
	}

	return &v1alpha1.Etcd{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: v1alpha1.EtcdSpec{
			Backup: v1alpha1.BackupSpec{
				Store: &v1alpha1.StoreSpec{
					Prefix: prefix,
				},
			},
		},
	}
}
