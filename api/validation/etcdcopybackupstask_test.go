// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"fmt"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	gomegatypes "github.com/onsi/gomega/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/ptr"
	"testing"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

const (
	etcdCopyBackupTestTaskName  = "etcd-cp-bkp-test"
	etcdCopyBackupTestNamespace = "test-ns"
	sourceUUID                  = "5779aae8-989c-4674-a43a-a91c50157140"
	targetUUID                  = "acd2b219-4789-4bd0-a2d0-efe4769a9aaa"
)

func TestValidateEtcdCopyBackupsTask(t *testing.T) {
	testCases := []struct {
		description  string
		name         string
		namespace    string
		sourceStore  *druidv1alpha1.StoreSpec
		targetStore  *druidv1alpha1.StoreSpec
		expectedErrs int
		errMatcher   gomegatypes.GomegaMatcher
	}{
		{
			description:  "should fail when no name and namespace is set",
			name:         "",
			namespace:    "",
			expectedErrs: 2,
			errMatcher: ConsistOf(
				PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeRequired), "Field": Equal("metadata.name")})),
				PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeRequired), "Field": Equal("metadata.namespace")})),
			),
		},
		{
			description: "should fail when invalid source and target source prefixes",
			name:        etcdCopyBackupTestTaskName,
			namespace:   etcdTestNamespace,
			sourceStore: &druidv1alpha1.StoreSpec{
				Prefix:   "invalid-source",
				Provider: ptr.To(druidv1alpha1.StorageProvider("not-supported")),
			},
			targetStore: &druidv1alpha1.StoreSpec{
				Prefix:   "invalid-target",
				Provider: ptr.To(druidv1alpha1.StorageProvider("not-supported")),
			},
			expectedErrs: 4,
			errMatcher: ConsistOf(
				PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeInvalid), "Field": Equal("spec.sourceStore.prefix")})),
				PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeInvalid), "Field": Equal("spec.targetStore.prefix")})),
				PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeInvalid), "Field": Equal("spec.sourceStore.provider")})),
				PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeInvalid), "Field": Equal("spec.targetStore.provider")})),
			),
		},
		{
			description: "should allow task with valid source and target store config",
			name:        etcdCopyBackupTestTaskName,
			namespace:   etcdTestNamespace,
			sourceStore: &druidv1alpha1.StoreSpec{
				Prefix:   fmt.Sprintf("%s--%s/%s", etcdCopyBackupTestNamespace, sourceUUID, etcdCopyBackupTestTaskName),
				Provider: ptr.To(druidv1alpha1.StorageProvider("S3")),
			},
			targetStore: &druidv1alpha1.StoreSpec{
				Prefix:   fmt.Sprintf("%s--%s/%s", etcdCopyBackupTestNamespace, targetUUID, etcdCopyBackupTestTaskName),
				Provider: ptr.To(druidv1alpha1.StorageProvider("S3")),
			},
		},
	}

	g := NewWithT(t)
	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			t.Parallel()
			task := &druidv1alpha1.EtcdCopyBackupsTask{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tc.name,
					Namespace: tc.namespace,
				},
			}
			if tc.sourceStore != nil {
				task.Spec.SourceStore = *tc.sourceStore
			}
			if tc.targetStore != nil {
				task.Spec.TargetStore = *tc.targetStore
			}

			errs := ValidateEtcdCopyBackupsTask(task)
			g.Expect(errs).To(HaveLen(tc.expectedErrs))
			if tc.errMatcher != nil {
				g.Expect(errs).To(tc.errMatcher)
			}
		})
	}
}

func TestEtcdCopyBackupTaskUpdateWhenDeletionTimestampIsSet(t *testing.T) {
	oldTask := &druidv1alpha1.EtcdCopyBackupsTask{
		ObjectMeta: metav1.ObjectMeta{
			Name:              etcdCopyBackupTestTaskName,
			Namespace:         etcdCopyBackupTestNamespace,
			ResourceVersion:   "1",
			DeletionTimestamp: ptr.To(metav1.Now()),
		},
		Spec: druidv1alpha1.EtcdCopyBackupsTaskSpec{
			SourceStore: druidv1alpha1.StoreSpec{
				Prefix:   fmt.Sprintf("%s--%s/%s", etcdCopyBackupTestNamespace, sourceUUID, etcdCopyBackupTestTaskName),
				Provider: ptr.To(druidv1alpha1.StorageProvider("S3")),
			},
			TargetStore: druidv1alpha1.StoreSpec{
				Prefix:   fmt.Sprintf("%s--%s/%s", etcdCopyBackupTestNamespace, targetUUID, etcdCopyBackupTestTaskName),
				Provider: ptr.To(druidv1alpha1.StorageProvider("S3")),
			},
		},
	}

	newTask := oldTask.DeepCopy()
	newTask.ResourceVersion = "2"
	newTask.Spec.SourceStore.Container = ptr.To("dummy-image")
	g := NewWithT(t)
	errs := ValidateEtcdCopyBackupsTaskUpdate(newTask, oldTask)
	g.Expect(errs).To(HaveLen(1))
	g.Expect(errs).To(ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeInvalid), "Field": Equal("spec")}))))
}

func TestPreventSourceAndTargetStorePrefixUpdate(t *testing.T) {
	oldTask := &druidv1alpha1.EtcdCopyBackupsTask{
		ObjectMeta: metav1.ObjectMeta{
			Name:              etcdCopyBackupTestTaskName,
			Namespace:         etcdCopyBackupTestNamespace,
			ResourceVersion:   "1",
			DeletionTimestamp: ptr.To(metav1.Now()),
		},
		Spec: druidv1alpha1.EtcdCopyBackupsTaskSpec{
			SourceStore: druidv1alpha1.StoreSpec{
				Prefix:   fmt.Sprintf("%s--%s/%s", etcdCopyBackupTestNamespace, sourceUUID, etcdCopyBackupTestTaskName),
				Provider: ptr.To(druidv1alpha1.StorageProvider("S3")),
			},
			TargetStore: druidv1alpha1.StoreSpec{
				Prefix:   fmt.Sprintf("%s--%s/%s", etcdCopyBackupTestNamespace, targetUUID, etcdCopyBackupTestTaskName),
				Provider: ptr.To(druidv1alpha1.StorageProvider("S3")),
			},
		},
	}
	newTask := oldTask.DeepCopy()
	newTask.ResourceVersion = "2"
	const newUUID = "7e527270-ec8e-4cf3-b36f-ba3aa4f44816"
	newTask.Spec.SourceStore.Prefix = fmt.Sprintf("%s--%s/%s", etcdTestNamespace, newUUID, etcdTestName)
	newTask.Spec.TargetStore.Prefix = fmt.Sprintf("%s--%s/%s", etcdTestNamespace, newUUID, etcdTestName)

	g := NewWithT(t)
	errs := ValidateEtcdCopyBackupsTaskUpdate(newTask, oldTask)
	g.Expect(errs).To(HaveLen(3))
	g.Expect(errs).To(
		ConsistOf(
			PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeInvalid), "Field": Equal("spec")})),
			PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeInvalid), "Field": Equal("spec.sourceStore.prefix")})),
			PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeInvalid), "Field": Equal("spec.targetStore.prefix")})),
		),
	)
}

func TestAllowValidUpdatesToEtcdCopyBackupTask(t *testing.T) {
	oldTask := &druidv1alpha1.EtcdCopyBackupsTask{
		ObjectMeta: metav1.ObjectMeta{
			Name:            etcdCopyBackupTestTaskName,
			Namespace:       etcdCopyBackupTestNamespace,
			ResourceVersion: "1",
		},
		Spec: druidv1alpha1.EtcdCopyBackupsTaskSpec{
			SourceStore: druidv1alpha1.StoreSpec{
				Prefix:   fmt.Sprintf("%s--%s/%s", etcdCopyBackupTestNamespace, sourceUUID, etcdCopyBackupTestTaskName),
				Provider: ptr.To(druidv1alpha1.StorageProvider("S3")),
			},
			TargetStore: druidv1alpha1.StoreSpec{
				Prefix:   fmt.Sprintf("%s--%s/%s", etcdCopyBackupTestNamespace, targetUUID, etcdCopyBackupTestTaskName),
				Provider: ptr.To(druidv1alpha1.StorageProvider("S3")),
			},
		},
	}
	newTask := oldTask.DeepCopy()
	newTask.ResourceVersion = "2"
	newTask.Spec.SourceStore.Container = ptr.To("dummy-image")
	newTask.Spec.SourceStore.Provider = ptr.To(druidv1alpha1.StorageProvider("GCS"))
	newTask.Spec.TargetStore.Container = ptr.To("dummy-image")
	newTask.Spec.TargetStore.Provider = ptr.To(druidv1alpha1.StorageProvider("GCS"))

	g := NewWithT(t)
	errs := ValidateEtcdCopyBackupsTaskUpdate(newTask, oldTask)
	g.Expect(errs).To(HaveLen(0))
}
