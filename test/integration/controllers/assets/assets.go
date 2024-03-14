// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package assets

import (
	"path/filepath"

	"github.com/gardener/etcd-druid/internal/common"
	"github.com/gardener/gardener/pkg/utils/imagevector"

	. "github.com/onsi/gomega"
)

// GetEtcdCrdPath returns the path to the Etcd CRD.
func GetEtcdCrdPath() string {
	return filepath.Join("..", "..", "..", "..", "config", "crd", "bases", "10-crd-druid.gardener.cloud_etcds.yaml")
}

// GetEtcdCopyBackupsTaskCrdPath returns the path to the EtcdCopyBackupsTask CRD.
func GetEtcdCopyBackupsTaskCrdPath() string {
	return filepath.Join("..", "..", "..", "..", "config", "crd", "bases", "10-crd-druid.gardener.cloud_etcdcopybackupstasks.yaml")
}

// CreateImageVector creates an image vector.
func CreateImageVector() imagevector.ImageVector {
	imageVectorPath := filepath.Join("..", "..", "..", "..", common.DefaultImageVectorFilePath)
	imageVector, err := imagevector.ReadGlobalImageVectorWithEnvOverride(imageVectorPath)
	Expect(err).To(BeNil())
	return imageVector
}
