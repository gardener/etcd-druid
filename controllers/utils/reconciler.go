package utils

import (
	"bytes"
	"fmt"
	"path/filepath"

	"github.com/gardener/etcd-druid/pkg/common"
	"github.com/gardener/gardener/pkg/chartrenderer"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	"github.com/gardener/gardener/pkg/utils/imagevector"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/rest"
)

const (
	// DefaultImageVector is a constant for the path to the default image vector file.
	DefaultImageVector = "images.yaml"
)

func getDefaultImageYAMLPath() string {
	return filepath.Join(common.ChartPath, DefaultImageVector)
}

func CreateImageVector() (imagevector.ImageVector, error) {
	imageVector, err := imagevector.ReadGlobalImageVectorWithEnvOverride(getDefaultImageYAMLPath())
	if err != nil {
		return nil, err
	}
	return imageVector, nil
}

func CreateChartApplier(config *rest.Config) (kubernetes.ChartApplier, error) {
	renderer, err := chartrenderer.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	applier, err := kubernetes.NewApplierForConfig(config)
	if err != nil {
		return nil, err
	}
	return kubernetes.NewChartApplier(renderer, applier), nil
}

func DecodeObject(renderedChart *chartrenderer.RenderedChart, path string, object interface{}) error {
	if content, ok := renderedChart.Files()[path]; ok {
		decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader([]byte(content)), 1024)
		return decoder.Decode(&object)
	}
	return fmt.Errorf("missing file %s in the rendered chart", path)
}
