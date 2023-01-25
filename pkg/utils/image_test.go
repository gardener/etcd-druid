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
		etcdName  = "etcd-main-0"
		namespace = "default"
	)
	var (
		imageVector imagevector.ImageVector
		etcd        *druidv1alpha1.Etcd
		err         error
	)

	It("etcd spec defines etcd and backup-restore images", func() {
		etcd, err = testutils.EtcdBuilderWithDefaults(etcdName, namespace).Build()
		Expect(err).To(BeNil())
		imageVector = createImageVector(true, true)
		etcdImage, etcdBackupRestoreImage, err := GetEtcdImages(etcd, imageVector)
		Expect(err).To(BeNil())
		Expect(etcdImage).ToNot(BeNil())
		Expect(etcdImage).To(Equal(etcd.Spec.Etcd.Image))
		Expect(etcdBackupRestoreImage).ToNot(BeNil())
		Expect(etcdBackupRestoreImage).To(Equal(etcd.Spec.Backup.Image))
	})

	It("etcd spec has no image defined and image vector has both images set", func() {
		etcd, err = testutils.EtcdBuilderWithDefaults(etcdName, namespace).Build()
		Expect(err).To(BeNil())
		etcd.Spec.Etcd.Image = nil
		etcd.Spec.Backup.Image = nil
		imageVector = createImageVector(true, true)
		etcdImage, etcdBackupRestoreImage, err := GetEtcdImages(etcd, imageVector)
		Expect(err).To(BeNil())
		Expect(etcdImage).ToNot(BeNil())
		vectorEtcdImage, err := imageVector.FindImage(common.Etcd)
		Expect(err).To(BeNil())
		Expect(*etcdImage).To(Equal(vectorEtcdImage.String()))
		Expect(etcdBackupRestoreImage).ToNot(BeNil())
		vectorBackupRestoreImage, err := imageVector.FindImage(common.BackupRestore)
		Expect(err).To(BeNil())
		Expect(*etcdBackupRestoreImage).To(Equal(vectorBackupRestoreImage.String()))
	})

	It("etcd spec only has backup-restore image and image-vector has only etcd image", func() {
		etcd, err = testutils.EtcdBuilderWithDefaults(etcdName, namespace).Build()
		Expect(err).To(BeNil())
		etcd.Spec.Etcd.Image = nil
		imageVector = createImageVector(true, false)
		etcdImage, etcdBackupRestoreImage, err := GetEtcdImages(etcd, imageVector)
		Expect(err).To(BeNil())
		Expect(etcdImage).ToNot(BeNil())
		vectorEtcdImage, err := imageVector.FindImage(common.Etcd)
		Expect(err).To(BeNil())
		Expect(*etcdImage).To(Equal(vectorEtcdImage.String()))
		Expect(etcdBackupRestoreImage).ToNot(BeNil())
		Expect(etcdBackupRestoreImage).To(Equal(etcd.Spec.Backup.Image))
	})

	It("both spec and image vector do not have backup-restore image", func() {
		etcd, err = testutils.EtcdBuilderWithDefaults(etcdName, namespace).Build()
		Expect(err).To(BeNil())
		etcd.Spec.Backup.Image = nil
		imageVector = createImageVector(true, false)
		etcdImage, etcdBackupRestoreImage, err := GetEtcdImages(etcd, imageVector)
		Expect(err).ToNot(BeNil())
		Expect(etcdImage).To(BeNil())
		Expect(etcdBackupRestoreImage).To(BeNil())
	})
})

func createImageVector(withEtcdImage, withBackupRestoreImage bool) imagevector.ImageVector {
	var imageSources []*imagevector.ImageSource
	const (
		repo             = "test-repo"
		etcdTag          = "etcd-test-tag"
		backupRestoreTag = "backup-restore-test-tag"
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
	return imageSources
}