// Copyright 2023 SAP SE or an SAP affiliate company
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

package utils

import (
	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/pkg/common"
	testutils "github.com/gardener/etcd-druid/test/utils"

	"github.com/gardener/gardener/pkg/utils/imagevector"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/utils/pointer"
)

var _ = Describe("Image retrieval tests", func() {

	const (
		etcdName  = "etcd-test-0"
		namespace = "default"
	)
	var (
		imageVector imagevector.ImageVector
		etcd        *druidv1alpha1.Etcd
		err         error
	)

	It("etcd spec defines etcd and backup-restore images", func() {
		etcd = testutils.EtcdBuilderWithDefaults(etcdName, namespace).Build()
		imageVector = createImageVector(true, true, false, false)
		etcdImage, etcdBackupRestoreImage, initContainerImage, err := GetEtcdImages(etcd, imageVector, false)
		Expect(err).To(BeNil())
		Expect(etcdImage).ToNot(BeNil())
		Expect(etcdImage).To(Equal(etcd.Spec.Etcd.Image))
		Expect(etcdBackupRestoreImage).ToNot(BeNil())
		Expect(etcdBackupRestoreImage).To(Equal(etcd.Spec.Backup.Image))
		vectorInitContainerImage, err := imageVector.FindImage(common.Alpine)
		Expect(err).To(BeNil())
		Expect(*initContainerImage).To(Equal(vectorInitContainerImage.String()))
	})

	It("etcd spec has no image defined and image vector has both images set", func() {
		etcd = testutils.EtcdBuilderWithDefaults(etcdName, namespace).Build()
		Expect(err).To(BeNil())
		etcd.Spec.Etcd.Image = nil
		etcd.Spec.Backup.Image = nil
		imageVector = createImageVector(true, true, false, false)
		etcdImage, etcdBackupRestoreImage, initContainerImage, err := GetEtcdImages(etcd, imageVector, false)
		Expect(err).To(BeNil())
		Expect(etcdImage).ToNot(BeNil())
		vectorEtcdImage, err := imageVector.FindImage(common.Etcd)
		Expect(err).To(BeNil())
		Expect(*etcdImage).To(Equal(vectorEtcdImage.String()))
		Expect(etcdBackupRestoreImage).ToNot(BeNil())
		vectorBackupRestoreImage, err := imageVector.FindImage(common.BackupRestore)
		Expect(err).To(BeNil())
		Expect(*etcdBackupRestoreImage).To(Equal(vectorBackupRestoreImage.String()))
		vectorInitContainerImage, err := imageVector.FindImage(common.Alpine)
		Expect(err).To(BeNil())
		Expect(*initContainerImage).To(Equal(vectorInitContainerImage.String()))
	})

	It("etcd spec only has backup-restore image and image-vector has only etcd image", func() {
		etcd = testutils.EtcdBuilderWithDefaults(etcdName, namespace).Build()
		Expect(err).To(BeNil())
		etcd.Spec.Etcd.Image = nil
		imageVector = createImageVector(true, false, false, false)
		etcdImage, etcdBackupRestoreImage, initContainerImage, err := GetEtcdImages(etcd, imageVector, false)
		Expect(err).To(BeNil())
		Expect(etcdImage).ToNot(BeNil())
		vectorEtcdImage, err := imageVector.FindImage(common.Etcd)
		Expect(err).To(BeNil())
		Expect(*etcdImage).To(Equal(vectorEtcdImage.String()))
		Expect(etcdBackupRestoreImage).ToNot(BeNil())
		Expect(etcdBackupRestoreImage).To(Equal(etcd.Spec.Backup.Image))
		vectorInitContainerImage, err := imageVector.FindImage(common.Alpine)
		Expect(err).To(BeNil())
		Expect(*initContainerImage).To(Equal(vectorInitContainerImage.String()))
	})

	It("both spec and image vector do not have backup-restore image", func() {
		etcd = testutils.EtcdBuilderWithDefaults(etcdName, namespace).Build()
		Expect(err).To(BeNil())
		etcd.Spec.Backup.Image = nil
		imageVector = createImageVector(true, false, false, false)
		etcdImage, etcdBackupRestoreImage, initContainerImage, err := GetEtcdImages(etcd, imageVector, false)
		Expect(err).ToNot(BeNil())
		Expect(etcdImage).To(BeNil())
		Expect(etcdBackupRestoreImage).To(BeNil())
		Expect(initContainerImage).To(BeNil())
	})

	It("etcd spec has no images defined, image vector has all images, and UseEtcdWrapper feature gate is turned on", func() {
		etcd = testutils.EtcdBuilderWithDefaults(etcdName, namespace).Build()
		Expect(err).To(BeNil())
		etcd.Spec.Etcd.Image = nil
		etcd.Spec.Backup.Image = nil
		imageVector = createImageVector(true, true, true, true)
		etcdImage, etcdBackupRestoreImage, initContainerImage, err := GetEtcdImages(etcd, imageVector, true)
		Expect(err).To(BeNil())
		Expect(etcdImage).ToNot(BeNil())
		vectorEtcdImage, err := imageVector.FindImage(common.EtcdWrapper)
		Expect(err).To(BeNil())
		Expect(*etcdImage).To(Equal(vectorEtcdImage.String()))
		Expect(etcdBackupRestoreImage).ToNot(BeNil())
		vectorBackupRestoreImage, err := imageVector.FindImage(common.BackupRestoreDistroless)
		Expect(err).To(BeNil())
		Expect(*etcdBackupRestoreImage).To(Equal(vectorBackupRestoreImage.String()))
		vectorInitContainerImage, err := imageVector.FindImage(common.Alpine)
		Expect(err).To(BeNil())
		Expect(*initContainerImage).To(Equal(vectorInitContainerImage.String()))
	})
})

func createImageVector(withEtcdImage, withBackupRestoreImage, withEtcdWrapperImage, withBackupRestoreDistrolessImage bool) imagevector.ImageVector {
	var imageSources []*imagevector.ImageSource
	const (
		repo                       = "test-repo"
		etcdTag                    = "etcd-test-tag"
		etcdWrapperTag             = "etcd-wrapper-test-tag"
		backupRestoreTag           = "backup-restore-test-tag"
		backupRestoreDistrolessTag = "backup-restore-distroless-test-tag"
		initContainerTag           = "init-container-test-tag"
	)
	if withEtcdImage {
		imageSources = append(imageSources, &imagevector.ImageSource{
			Name:       common.Etcd,
			Repository: repo,
			Tag:        pointer.String(etcdTag),
		})
	}
	if withBackupRestoreImage {
		imageSources = append(imageSources, &imagevector.ImageSource{
			Name:       common.BackupRestore,
			Repository: repo,
			Tag:        pointer.String(backupRestoreTag),
		})

	}
	if withEtcdWrapperImage {
		imageSources = append(imageSources, &imagevector.ImageSource{
			Name:       common.EtcdWrapper,
			Repository: repo,
			Tag:        pointer.String(etcdWrapperTag),
		})
	}
	if withBackupRestoreDistrolessImage {
		imageSources = append(imageSources, &imagevector.ImageSource{
			Name:       common.BackupRestoreDistroless,
			Repository: repo,
			Tag:        pointer.String(backupRestoreDistrolessTag),
		})
	}
	imageSources = append(imageSources, &imagevector.ImageSource{
		Name:       common.Alpine,
		Repository: repo,
		Tag:        pointer.String(initContainerTag),
	})
	return imageSources
}
