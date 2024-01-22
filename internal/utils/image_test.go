package utils

import (
	"testing"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	. "github.com/onsi/gomega"

	"github.com/gardener/etcd-druid/internal/common"
	testutils "github.com/gardener/etcd-druid/test/utils"
	"github.com/gardener/gardener/pkg/utils/imagevector"
	"k8s.io/utils/pointer"
)

// ************************** GetEtcdImages **************************
func TestGetEtcdImages(t *testing.T) {
	tests := []struct {
		name string
		run  func(g *WithT, etcd *druidv1alpha1.Etcd)
	}{
		{"etcd spec defines etcd and etcdBR images", testWithEtcdAndEtcdBRImages},
		{"etcd spec has no image defined and image vector has etcd and etcdBR images set", testWithNoImageInSpecAndIVWithEtcdAndBRImages},
		{"", testSpecWithEtcdBRImageAndIVWithEtcdImage},
		{"", testSpecAndIVWithoutEtcdBRImage},
		{"", testWithSpecAndIVNotHavingAnyImages},
		{"", testWithNoImagesInSpecAndIVWithAllImagesWithWrapper},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			etcd := testutils.EtcdBuilderWithDefaults(testutils.TestEtcdName, testutils.TestNamespace).Build()
			g := NewWithT(t)
			test.run(g, etcd)
		})
	}
}

func testWithEtcdAndEtcdBRImages(g *WithT, etcd *druidv1alpha1.Etcd) {
	iv := createImageVector(true, true, false, false)
	etcdImg, etcdBRImg, initContainerImg, err := GetEtcdImages(etcd, iv, false)
	g.Expect(err).To(BeNil())
	g.Expect(etcdImg).ToNot(BeEmpty())
	g.Expect(etcdImg).To(Equal(*etcd.Spec.Etcd.Image))
	g.Expect(etcdBRImg).ToNot(BeEmpty())
	g.Expect(etcdBRImg).To(Equal(*etcd.Spec.Backup.Image))
	vectorInitContainerImage, err := iv.FindImage(common.Alpine)
	g.Expect(err).To(BeNil())
	g.Expect(initContainerImg).To(Equal(vectorInitContainerImage.String()))
}

func testWithNoImageInSpecAndIVWithEtcdAndBRImages(g *WithT, etcd *druidv1alpha1.Etcd) {
	etcd.Spec.Etcd.Image = nil
	etcd.Spec.Backup.Image = nil
	iv := createImageVector(true, true, false, false)
	etcdImage, etcdBackupRestoreImage, initContainerImage, err := GetEtcdImages(etcd, iv, false)
	g.Expect(err).To(BeNil())
	g.Expect(etcdImage).ToNot(BeEmpty())
	vectorEtcdImage, err := iv.FindImage(common.Etcd)
	g.Expect(err).To(BeNil())
	g.Expect(etcdImage).To(Equal(vectorEtcdImage.String()))
	g.Expect(etcdBackupRestoreImage).ToNot(BeNil())
	vectorBackupRestoreImage, err := iv.FindImage(common.BackupRestore)
	g.Expect(err).To(BeNil())
	g.Expect(etcdBackupRestoreImage).To(Equal(vectorBackupRestoreImage.String()))
	vectorInitContainerImage, err := iv.FindImage(common.Alpine)
	g.Expect(err).To(BeNil())
	g.Expect(initContainerImage).To(Equal(vectorInitContainerImage.String()))
}

func testSpecWithEtcdBRImageAndIVWithEtcdImage(g *WithT, etcd *druidv1alpha1.Etcd) {
	etcd.Spec.Etcd.Image = nil
	iv := createImageVector(true, false, false, false)
	etcdImage, etcdBackupRestoreImage, initContainerImage, err := GetEtcdImages(etcd, iv, false)
	g.Expect(err).To(BeNil())
	g.Expect(etcdImage).ToNot(BeEmpty())
	vectorEtcdImage, err := iv.FindImage(common.Etcd)
	g.Expect(err).To(BeNil())
	g.Expect(etcdImage).To(Equal(vectorEtcdImage.String()))
	g.Expect(etcdBackupRestoreImage).ToNot(BeNil())
	g.Expect(etcdBackupRestoreImage).To(Equal(*etcd.Spec.Backup.Image))
	vectorInitContainerImage, err := iv.FindImage(common.Alpine)
	g.Expect(err).To(BeNil())
	g.Expect(initContainerImage).To(Equal(vectorInitContainerImage.String()))
}

func testSpecAndIVWithoutEtcdBRImage(g *WithT, etcd *druidv1alpha1.Etcd) {
	etcd.Spec.Backup.Image = nil
	iv := createImageVector(true, false, false, false)
	etcdImage, etcdBackupRestoreImage, initContainerImage, err := GetEtcdImages(etcd, iv, false)
	g.Expect(err).ToNot(BeNil())
	g.Expect(etcdImage).To(BeEmpty())
	g.Expect(etcdBackupRestoreImage).To(BeEmpty())
	g.Expect(initContainerImage).To(BeEmpty())
}

func testWithSpecAndIVNotHavingAnyImages(g *WithT, etcd *druidv1alpha1.Etcd) {
	etcd.Spec.Backup.Image = nil
	iv := createImageVector(false, false, false, false)
	etcdImage, etcdBackupRestoreImage, initContainerImage, err := GetEtcdImages(etcd, iv, false)
	g.Expect(err).ToNot(BeNil())
	g.Expect(etcdImage).To(BeEmpty())
	g.Expect(etcdBackupRestoreImage).To(BeEmpty())
	g.Expect(initContainerImage).To(BeEmpty())
}

func testWithNoImagesInSpecAndIVWithAllImagesWithWrapper(g *WithT, etcd *druidv1alpha1.Etcd) {
	etcd.Spec.Etcd.Image = nil
	etcd.Spec.Backup.Image = nil
	iv := createImageVector(true, true, true, true)
	etcdImage, etcdBackupRestoreImage, initContainerImage, err := GetEtcdImages(etcd, iv, true)
	g.Expect(err).To(BeNil())
	g.Expect(etcdImage).ToNot(BeEmpty())
	vectorEtcdImage, err := iv.FindImage(common.EtcdWrapper)
	g.Expect(err).To(BeNil())
	g.Expect(etcdImage).To(Equal(vectorEtcdImage.String()))
	g.Expect(etcdBackupRestoreImage).ToNot(BeEmpty())
	vectorBackupRestoreImage, err := iv.FindImage(common.BackupRestoreDistroless)
	g.Expect(err).To(BeNil())
	g.Expect(etcdBackupRestoreImage).To(Equal(vectorBackupRestoreImage.String()))
	vectorInitContainerImage, err := iv.FindImage(common.Alpine)
	g.Expect(err).To(BeNil())
	g.Expect(initContainerImage).To(Equal(vectorInitContainerImage.String()))
}

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
