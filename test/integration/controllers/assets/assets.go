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

package assets

import (
	"path/filepath"

	"github.com/gardener/etcd-druid/pkg/common"
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
