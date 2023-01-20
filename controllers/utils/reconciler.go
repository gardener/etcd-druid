package utils

import (
	"path/filepath"

	"github.com/gardener/etcd-druid/pkg/common"
	"github.com/gardener/gardener/pkg/utils/imagevector"
)

const (
	// DefaultImageVector is a constant for the path to the default image vector file.
	DefaultImageVector = "images.yaml"
)

func getDefaultImageYAMLPath() string {
	return filepath.Join(common.ChartPath, DefaultImageVector)
}

func CreateDefaultImageVector() (imagevector.ImageVector, error) {
	imageVector, err := imagevector.ReadGlobalImageVectorWithEnvOverride(getDefaultImageYAMLPath())
	if err != nil {
		return nil, err
	}
	return imageVector, nil
}
