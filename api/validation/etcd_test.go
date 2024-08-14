// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	"fmt"

	"github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/api/validation"

	"github.com/onsi/gomega/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/pointer"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

const (
	name      = "etcd-test"
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
						Provider: (*v1alpha1.StorageProvider)(pointer.String(provider)),
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
				Provider: (*v1alpha1.StorageProvider)(pointer.String("invalid")),
			}, ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{
				"Type":  Equal(field.ErrorTypeInvalid),
				"Field": Equal("spec.backup.store.prefix"),
			})), PointTo(MatchFields(IgnoreExtras, Fields{
				"Type":  Equal(field.ErrorTypeInvalid),
				"Field": Equal("spec.backup.store.provider"),
			})))),

			Entry("should allow valid spec.backup.store", &v1alpha1.StoreSpec{
				Prefix:   fmt.Sprintf("%s--%s/%s", namespace, uuid, name),
				Provider: (*v1alpha1.StorageProvider)(pointer.String(provider)),
			}, BeNil()),

			Entry("should allow nil spec.backup.store", nil, BeNil()),
		)
	})

	Describe("#ValidateEtcdUpdate", func() {
		It("should prevent updating anything if deletion timestamp is set", func() {
			now := metav1.Now()
			etcd.DeletionTimestamp = &now
			etcd.ResourceVersion = "1"

			newEtcd := etcd.DeepCopy()
			newEtcd.ResourceVersion = "2"
			newEtcd.Spec.Backup.Port = pointer.Int32(42)

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
			newEtcd.Spec.Backup.Store.Container = pointer.String("foo")
			newEtcd.Spec.Backup.Store.Provider = (*v1alpha1.StorageProvider)(pointer.String("gcp"))

			errList := validation.ValidateEtcdUpdate(newEtcd, etcd)

			Expect(errList).To(BeEmpty())
		})
	})
})
