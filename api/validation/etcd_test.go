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
	"time"

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

const (
	name      = "etcd-main"
	namespace = "shoot--foo--bar"
	uuid      = "f1a38edd-e506-412a-82e6-e0fa839d0707"
	provider  = "aws"
)

var _ = Describe("Etcd validation tests", func() {
	var etcd *v1alpha1.Etcd

	BeforeEach(func() {
		etcd = &v1alpha1.Etcd{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: v1alpha1.EtcdSpec{
				Backup: v1alpha1.BackupSpec{
					Store: &v1alpha1.StoreSpec{
						Prefix:   fmt.Sprintf("%s--%s/%s", namespace, uuid, name),
						Provider: (*v1alpha1.StorageProvider)(pointer.StringPtr(provider)),
					},
					OwnerCheck: &v1alpha1.OwnerCheckSpec{
						Name:        "owner.foo.example.com",
						ID:          "bar",
						Interval:    &metav1.Duration{Duration: 30 * time.Second},
						Timeout:     &metav1.Duration{Duration: 2 * time.Minute},
						DNSCacheTTL: &metav1.Duration{Duration: 1 * time.Minute},
					},
				},
			},
		}
	})

	Describe("#ValidateEtcd", func() {
		It("should forbid empty Etcd resources", func() {
			errorList := validation.ValidateEtcd(&v1alpha1.Etcd{})

			Expect(errorList).To(ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{
				"Type":  Equal(field.ErrorTypeRequired),
				"Field": Equal("metadata.name"),
			})), PointTo(MatchFields(IgnoreExtras, Fields{
				"Type":  Equal(field.ErrorTypeRequired),
				"Field": Equal("metadata.namespace"),
			}))))
		})

		DescribeTable("validate spec.backup.store",
			func(store *v1alpha1.StoreSpec, m types.GomegaMatcher) {
				etcd.Spec.Backup.Store = store
				Expect(validation.ValidateEtcd(etcd)).To(m)
			},

			Entry("should forbid invalid spec.backup.store", &v1alpha1.StoreSpec{
				Prefix:   "invalid",
				Provider: (*v1alpha1.StorageProvider)(pointer.StringPtr("invalid")),
			}, ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{
				"Type":  Equal(field.ErrorTypeInvalid),
				"Field": Equal("spec.backup.store.prefix"),
			})), PointTo(MatchFields(IgnoreExtras, Fields{
				"Type":  Equal(field.ErrorTypeInvalid),
				"Field": Equal("spec.backup.store.provider"),
			})))),

			Entry("should allow valid spec.backup.store", &v1alpha1.StoreSpec{
				Prefix:   fmt.Sprintf("%s--%s/%s", namespace, uuid, name),
				Provider: (*v1alpha1.StorageProvider)(pointer.StringPtr(provider)),
			}, BeNil()),

			Entry("should allow nil spec.backup.store", nil, BeNil()),
		)

		DescribeTable("validate spec.backup.ownerCheck",
			func(ownerCheck *v1alpha1.OwnerCheckSpec, m types.GomegaMatcher) {
				if ownerCheck != nil {
					etcd.Spec.Backup.OwnerCheck = ownerCheck
				}
				Expect(validation.ValidateEtcd(etcd)).To(m)
			},

			Entry("should forbid invalid spec.backup.ownerCheck", &v1alpha1.OwnerCheckSpec{
				Name:        "",
				ID:          "",
				Interval:    &metav1.Duration{Duration: -30 * time.Second},
				Timeout:     &metav1.Duration{Duration: -2 * time.Minute},
				DNSCacheTTL: &metav1.Duration{Duration: -1 * time.Minute},
			}, ConsistOf(
				PointTo(MatchFields(IgnoreExtras, Fields{
					"Type":  Equal(field.ErrorTypeRequired),
					"Field": Equal("spec.backup.ownerCheck.name"),
				})),
				PointTo(MatchFields(IgnoreExtras, Fields{
					"Type":  Equal(field.ErrorTypeRequired),
					"Field": Equal("spec.backup.ownerCheck.id"),
				})),
				PointTo(MatchFields(IgnoreExtras, Fields{
					"Type":  Equal(field.ErrorTypeInvalid),
					"Field": Equal("spec.backup.ownerCheck.interval"),
				})),
				PointTo(MatchFields(IgnoreExtras, Fields{
					"Type":  Equal(field.ErrorTypeInvalid),
					"Field": Equal("spec.backup.ownerCheck.timeout"),
				})),
				PointTo(MatchFields(IgnoreExtras, Fields{
					"Type":  Equal(field.ErrorTypeInvalid),
					"Field": Equal("spec.backup.ownerCheck.dnsCacheTTL"),
				})),
			)),
			Entry("should allow valid spec.backup.ownerCheck", nil, BeNil()),
		)
	})

	Describe("#ValidateEtcdUpdate", func() {
		It("should prevent updating anything if deletion timestamp is set", func() {
			now := metav1.Now()
			etcd.DeletionTimestamp = &now
			etcd.ResourceVersion = "1"

			newEtcd := etcd.DeepCopy()
			newEtcd.ResourceVersion = "2"
			newEtcd.Spec.Backup.Port = pointer.Int32Ptr(42)

			errList := validation.ValidateEtcdUpdate(newEtcd, etcd)

			Expect(errList).To(ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{
				"Type":  Equal(field.ErrorTypeInvalid),
				"Field": Equal("spec"),
			}))))
		})

		It("should prevent updating spec.backup.store.prefix", func() {
			etcd.ResourceVersion = "1"

			newEtcd := etcd.DeepCopy()
			newEtcd.ResourceVersion = "2"
			newEtcd.Spec.Backup.Store.Prefix = namespace + "/" + name

			errList := validation.ValidateEtcdUpdate(newEtcd, etcd)

			Expect(errList).To(ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{
				"Type":  Equal(field.ErrorTypeInvalid),
				"Field": Equal("spec.backup.store.prefix"),
			}))))
		})

		It("should allow updating everything else", func() {
			etcd.ResourceVersion = "1"

			newEtcd := etcd.DeepCopy()
			newEtcd.ResourceVersion = "2"
			newEtcd.Spec.Replicas = 42
			newEtcd.Spec.Backup.OwnerCheck = &v1alpha1.OwnerCheckSpec{
				Name: "owner.foo.example.com",
				ID:   "baz",
			}
			newEtcd.Spec.Backup.Store.Container = pointer.StringPtr("foo")
			newEtcd.Spec.Backup.Store.Provider = (*v1alpha1.StorageProvider)(pointer.StringPtr("gcp"))

			errList := validation.ValidateEtcdUpdate(newEtcd, etcd)

			Expect(errList).To(BeEmpty())
		})
	})
})
