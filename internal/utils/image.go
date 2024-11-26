// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"

	"github.com/gardener/gardener/pkg/utils/imagevector"
	"k8s.io/utils/ptr"
)

// GetEtcdImages returns images for etcd and backup-restore by inspecting the etcd spec and the image vector
// and returns the image for the init container by inspecting the image vector.
// It will give preference to images that are set in the etcd spec and only if the image is not found in it should
// it be picked up from the image vector if it's set there.
// A return value of nil for either of the images indicates that the image is not set.
func GetEtcdImages(etcd *druidv1alpha1.Etcd, iv imagevector.ImageVector) (string, string, string, error) {
	etcdImageKey, etcdBRImageKey, initContainerImageKey := getEtcdImageKeys()
	etcdImage, err := chooseImage(etcdImageKey, etcd.Spec.Etcd.Image, iv)
	if err != nil {
		return "", "", "", err
	}
	etcdBackupRestoreImage, err := chooseImage(etcdBRImageKey, etcd.Spec.Backup.Image, iv)
	if err != nil {
		return "", "", "", err
	}
	initContainerImage, err := chooseImage(initContainerImageKey, nil, iv)
	if err != nil {
		return "", "", "", err
	}

	return *etcdImage, *etcdBackupRestoreImage, *initContainerImage, nil
}

func getEtcdImageKeys() (string, string, string) {
	return common.ImageKeyEtcdWrapper, common.ImageKeyEtcdBackupRestore, common.ImageKeyAlpine
}

// chooseImage selects an image based on the given key, specImage, and image vector.
// It returns the specImage if it is not nil; otherwise, it searches for the image in the image vector.
func chooseImage(key string, specImage *string, iv imagevector.ImageVector) (*string, error) {
	if specImage != nil {
		return specImage, nil
	}
	// Check if this image is present in the image vector.
	ivImage, err := imagevector.FindImages(iv, []string{key})
	if err != nil {
		return nil, err
	}
	return ptr.To(ivImage[key].String()), nil
}

// GetEtcdBackupRestoreImage returns the image for backup-restore from the given image vector.
func GetEtcdBackupRestoreImage(iv imagevector.ImageVector) (*string, error) {
	_, etcdbrImageKey, _ := getEtcdImageKeys()
	return chooseImage(etcdbrImageKey, nil, iv)
}

// GetInitContainerImage returns the image for init container from the given image vector.
func GetInitContainerImage(iv imagevector.ImageVector) (*string, error) {
	return chooseImage(common.ImageKeyAlpine, nil, iv)
}
