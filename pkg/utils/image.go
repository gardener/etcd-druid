package utils

import (
	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/pkg/common"
	"github.com/gardener/gardener/pkg/utils/imagevector"
	"k8s.io/utils/pointer"
)

// GetEtcdImages returns images for etcd and backup-restore by inspecting the etcd spec and the image vector.
// It will give preference to images that are set in the etcd spec and only if the image is not found in it should
// it be picked up from the image vector if its set there.
// A return value of nil for either of the images indicates that the image is not set.
func GetEtcdImages(etcd *druidv1alpha1.Etcd, iv imagevector.ImageVector) (*string, *string, error) {
	etcdSpecImage := etcd.Spec.Etcd.Image
	etcdSpecBackupRestoreImage := etcd.Spec.Backup.Image

	// return early if both images in spec are not nil
	if etcdSpecImage != nil && etcdSpecBackupRestoreImage != nil {
		return etcdSpecImage, etcdSpecBackupRestoreImage, nil
	}

	etcdImage, err := chooseImage(common.Etcd, etcdSpecImage, iv)
	if err != nil {
		return nil, nil, err
	}
	etcdBackupRestoreImage, err := chooseImage(common.BackupRestore, etcdSpecBackupRestoreImage, iv)
	if err != nil {
		return nil, nil, err
	}
	return etcdImage, etcdBackupRestoreImage, nil
}

func chooseImage(key string, specImage *string, iv imagevector.ImageVector) (*string, error) {
	if specImage != nil {
		return specImage, nil
	}
	// check if this image is present in the image vector.
	ivImage, err := imagevector.FindImages(iv, []string{key})
	if err != nil {
		return nil, err
	}
	return pointer.String(ivImage[key].String()), nil
}
