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
	"github.com/onsi/gomega/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

var _ = Describe("etcd", func() {
	DescribeTable("#ValidateEtcdCreate",
		func(e *v1alpha1.Etcd, m types.GomegaMatcher) { Expect(validation.ValidateEtcd(e)).To(m) },

		Entry("random name", newEtcd("non-valid"), ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{
			"Type":  Equal(field.ErrorTypeInvalid),
			"Field": Equal("spec.backup.store.prefix"),
		})))),
		Entry("name equal to etcd's name", newEtcd("etcd-name"), ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{
			"Type":  Equal(field.ErrorTypeInvalid),
			"Field": Equal("spec.backup.store.prefix"),
		})))),
		Entry("valid resource", newEtcd(""), BeNil()),
	)

	Describe("#ValidateEtcdUpdate", func() {
		It("Should prevent updating of spec.backup.store", func() {
			etcd := newEtcd("")

			old := etcd.DeepCopy()
			old.APIVersion = "1"

			etcd.Spec.Backup.Store.Prefix = "valid.but.new"

			errList := validation.ValidateEtcdUpdate(etcd, old)

			Expect(errList).To(ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{
				"Type":  Equal(field.ErrorTypeInvalid),
				"Field": Equal("spec"),
			}))))
		})
	})
})

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
