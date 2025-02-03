// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"fmt"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	gomegatypes "github.com/onsi/gomega/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/ptr"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

const (
	etcdTestName      = "etcd-test"
	etcdTestNamespace = "test-ns"
	testUUID          = ""
)

func TestValidateEtcd(t *testing.T) {

	testCases := []struct {
		description   string
		etcdName      string
		etcdNamespace string
		store         *druidv1alpha1.StoreSpec
		expectedErrs  int
		errMatcher    gomegatypes.GomegaMatcher
	}{
		{
			"should fail when no name and namespace is set",
			"",
			"",
			nil,
			2,
			ConsistOf(
				PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeRequired), "Field": Equal("metadata.name")})),
				PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeRequired), "Field": Equal("metadata.namespace")})),
			),
		},
		{
			"should allow nil store",
			etcdTestName,
			etcdTestNamespace,
			nil,
			0,
			nil,
		},
		{
			"should fail when prefix does not contain name and namespace",
			etcdTestName,
			etcdTestNamespace,
			&druidv1alpha1.StoreSpec{
				Prefix:   "invalid",
				Provider: ptr.To[druidv1alpha1.StorageProvider](s3),
			},
			1,
			ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeInvalid), "Field": Equal("spec.backup.store.prefix")}))),
		},
		{
			"should fail when unsupported provider is configured",
			etcdTestName,
			etcdTestNamespace,
			&druidv1alpha1.StoreSpec{
				Prefix:   fmt.Sprintf("%s--%s/%s", etcdTestNamespace, testUUID, etcdTestName),
				Provider: (*druidv1alpha1.StorageProvider)(ptr.To("not-supported")),
			},
			1,
			ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeInvalid), "Field": Equal("spec.backup.store.provider")}))),
		},
		{
			"should allow etcd with valid store config",
			etcdTestName,
			etcdTestNamespace,
			&druidv1alpha1.StoreSpec{
				Prefix:   fmt.Sprintf("%s--%s/%s", etcdTestNamespace, testUUID, etcdTestName),
				Provider: (*druidv1alpha1.StorageProvider)(ptr.To("GCS")),
			},
			0,
			nil,
		},
	}

	g := NewWithT(t)
	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			etcd := &druidv1alpha1.Etcd{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tc.etcdName,
					Namespace: tc.etcdNamespace,
				},
			}
			if tc.store != nil {
				etcd.Spec = druidv1alpha1.EtcdSpec{
					Backup: druidv1alpha1.BackupSpec{
						Store: tc.store,
					},
				}
			}
			errs := ValidateEtcd(etcd)
			g.Expect(errs).To(HaveLen(tc.expectedErrs))
			if tc.errMatcher != nil {
				g.Expect(errs).To(tc.errMatcher)
			}
		})
	}
}

func TestEtcdUpdateWhenDeletionTimestampIsSet(t *testing.T) {
	oldEtcd := &druidv1alpha1.Etcd{
		ObjectMeta: metav1.ObjectMeta{
			Name:              etcdTestName,
			Namespace:         etcdTestNamespace,
			ResourceVersion:   "1",
			DeletionTimestamp: ptr.To(metav1.Now()),
		},
		Spec: druidv1alpha1.EtcdSpec{
			Replicas: int32(1),
		},
	}

	newEtcd := oldEtcd.DeepCopy()
	newEtcd.Spec.Replicas = 3
	g := NewWithT(t)
	errs := ValidateEtcdUpdate(newEtcd, oldEtcd)
	g.Expect(errs).To(HaveLen(1))
	g.Expect(errs).To(ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeInvalid), "Field": Equal("spec")}))))
}

func TestPreventStorePrefixUpdate(t *testing.T) {
	oldEtcd := &druidv1alpha1.Etcd{
		ObjectMeta: metav1.ObjectMeta{
			Name:            etcdTestName,
			Namespace:       etcdTestNamespace,
			ResourceVersion: "1",
		},
		Spec: druidv1alpha1.EtcdSpec{
			Replicas: int32(1),
			Backup: druidv1alpha1.BackupSpec{
				Store: &druidv1alpha1.StoreSpec{
					Prefix:   fmt.Sprintf("%s--%s/%s", etcdTestNamespace, testUUID, etcdTestName),
					Provider: ptr.To(druidv1alpha1.StorageProvider("S3")),
				},
			},
		},
	}
	g := NewWithT(t)
	newUUID := "8047c75b-6b97-44ad-a389-84a9bedc2063"
	newEtcd := oldEtcd.DeepCopy()
	newEtcd.ObjectMeta.ResourceVersion = "2"
	newEtcd.Spec.Backup.Store.Prefix = fmt.Sprintf("%s--%s/%s", etcdTestNamespace, newUUID, etcdTestName)
	errs := ValidateEtcdUpdate(newEtcd, oldEtcd)
	g.Expect(errs).To(HaveLen(1))
	g.Expect(errs).To(ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeInvalid), "Field": Equal("spec.backup.store.prefix")}))))
}

func TestAllowValidUpdatesToEtcd(t *testing.T) {
	oldEtcd := &druidv1alpha1.Etcd{
		ObjectMeta: metav1.ObjectMeta{
			Name:            etcdTestName,
			Namespace:       etcdTestNamespace,
			ResourceVersion: "1",
		},
		Spec: druidv1alpha1.EtcdSpec{
			Replicas: int32(1),
			Backup: druidv1alpha1.BackupSpec{
				Store: &druidv1alpha1.StoreSpec{
					Prefix:   fmt.Sprintf("%s--%s/%s", etcdTestNamespace, testUUID, etcdTestName),
					Provider: ptr.To(druidv1alpha1.StorageProvider("S3")),
				},
			},
		},
	}

	g := NewWithT(t)
	newEtcd := oldEtcd.DeepCopy()
	newEtcd.ObjectMeta.ResourceVersion = "2"
	newEtcd.Spec.Replicas = 3
	newEtcd.Spec.Backup.Store.Provider = ptr.To(druidv1alpha1.StorageProvider("GCS"))
	errs := ValidateEtcdUpdate(newEtcd, oldEtcd)
	g.Expect(errs).To(HaveLen(0))
}
