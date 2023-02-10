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

// GetEtcdCopyBackupsBaseChartPath returns the path to the etcd-copy-backups chart.
func GetEtcdCopyBackupsBaseChartPath() string {
	return filepath.Join("..", "..", "..", "..", "charts", "etcd-copy-backups")
}

// GetEtcdChartPath returns the path to the etcd chart.
func GetEtcdChartPath() string {
	return filepath.Join("..", "..", "..", "..", "charts", "etcd")
}

// CreateImageVector creates a image vector.
func CreateImageVector() imagevector.ImageVector {
	imageVectorPath := filepath.Join("..", "..", "..", "..", common.ChartPath, "images.yaml")
	imageVector, err := imagevector.ReadGlobalImageVectorWithEnvOverride(imageVectorPath)
	Expect(err).To(BeNil())
	return imageVector
}
