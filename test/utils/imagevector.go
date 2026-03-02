// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"github.com/gardener/etcd-druid/internal/common"
	"github.com/gardener/etcd-druid/internal/utils/imagevector"

	"k8s.io/utils/ptr"
)

// CreateImageVector creates an image vector initializing it will different image sources.
func CreateImageVector(withEtcdWrapperImage, withBackupRestoreImage bool) imagevector.ImageVector {
	var imageSources []*imagevector.ImageSource
	if withEtcdWrapperImage {
		imageSources = append(imageSources,
			&imagevector.ImageSource{
				Name:       common.ImageKeyEtcdWrapper,
				Repository: ptr.To(TestImageRepo),
				Tag:        ptr.To(ETCDWrapperImageTag),
			},
			&imagevector.ImageSource{
				Name:       common.ImageKeyEtcdWrapperV3_5,
				Repository: ptr.To(TestImageRepo),
				Tag:        ptr.To(ETCDWrapperV3_5ImageTag),
			},
		)

	}
	if withBackupRestoreImage {
		imageSources = append(imageSources,
			&imagevector.ImageSource{
				Name:       common.ImageKeyEtcdBackupRestore,
				Repository: ptr.To(TestImageRepo),
				Tag:        ptr.To(ETCDBRImageTag),
			},
			&imagevector.ImageSource{
				Name:       common.ImageKeyEtcdBackupRestoreV3_5,
				Repository: ptr.To(TestImageRepo),
				Tag:        ptr.To(ETCDBRV3_5ImageTag),
			},
		)
	}
	imageSources = append(imageSources, &imagevector.ImageSource{
		Name:       common.ImageKeyAlpine,
		Repository: ptr.To(TestImageRepo),
		Tag:        ptr.To(InitContainerTag),
	})
	return imageSources
}
