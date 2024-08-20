// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"github.com/gardener/etcd-druid/internal/common"

	"github.com/gardener/gardener/pkg/utils/imagevector"
	"k8s.io/utils/pointer"
)

// CreateImageVector creates an image vector initializing it will different image sources.
func CreateImageVector(withEtcdImage, withBackupRestoreImage, withEtcdWrapperImage, withBackupRestoreDistrolessImage bool) imagevector.ImageVector {
	var imageSources []*imagevector.ImageSource
	if withEtcdImage {
		imageSources = append(imageSources, &imagevector.ImageSource{
			Name:       common.ImageKeyEtcd,
			Repository: TestImageRepo,
			Tag:        pointer.String(ETCDImageSourceTag),
		})
	}
	if withBackupRestoreImage {
		imageSources = append(imageSources, &imagevector.ImageSource{
			Name:       common.ImageKeyEtcdBackupRestore,
			Repository: TestImageRepo,
			Tag:        pointer.String(ETCDBRImageTag),
		})

	}
	if withEtcdWrapperImage {
		imageSources = append(imageSources, &imagevector.ImageSource{
			Name:       common.ImageKeyEtcdWrapper,
			Repository: TestImageRepo,
			Tag:        pointer.String(ETCDWrapperImageTag),
		})
	}
	if withBackupRestoreDistrolessImage {
		imageSources = append(imageSources, &imagevector.ImageSource{
			Name:       common.ImageKeyEtcdBackupRestoreDistroless,
			Repository: TestImageRepo,
			Tag:        pointer.String(ETCDBRDistrolessImageTag),
		})
	}
	imageSources = append(imageSources, &imagevector.ImageSource{
		Name:       common.ImageKeyAlpine,
		Repository: TestImageRepo,
		Tag:        pointer.String(InitContainerTag),
	})
	return imageSources
}
