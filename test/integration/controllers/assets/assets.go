package assets

import (
	"path/filepath"

	"github.com/gardener/etcd-druid/pkg/common"
	"github.com/gardener/gardener/pkg/utils/imagevector"

	. "github.com/onsi/gomega"
)

func GetEtcdCrdPath() string {
	return filepath.Join("..", "..", "..", "..", "config", "crd", "bases", "10-crd-druid.gardener.cloud_etcds.yaml")
}

func GetEtcdCopyBackupsTaskCrdPath() string {
	return filepath.Join("..", "..", "..", "..", "config", "crd", "bases", "10-crd-druid.gardener.cloud_etcdcopybackupstasks.yaml")
}

func GetEtcdCopyBackupsBaseChartPath() string {
	return filepath.Join("..", "..", "..", "..", "charts", "etcd-copy-backups")
}

func GetEtcdChartPath() string {
	return filepath.Join("..", "..", "..", "..", "charts", "etcd")
}

func CreateImageVector() imagevector.ImageVector {
	imageVectorPath := filepath.Join("..", "..", "..", "..", common.ChartPath, "images.yaml")
	imageVector, err := imagevector.ReadGlobalImageVectorWithEnvOverride(imageVectorPath)
	Expect(err).To(BeNil())
	return imageVector
}
