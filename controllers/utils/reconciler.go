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

package utils

import (
	"bytes"
	"fmt"
	"path/filepath"

	"github.com/gardener/etcd-druid/pkg/common"
	"github.com/gardener/gardener/pkg/chartrenderer"
	"github.com/gardener/gardener/pkg/utils/imagevector"
	"k8s.io/apimachinery/pkg/util/yaml"
)

const (
	// DefaultImageVector is a constant for the path to the default image vector file.
	DefaultImageVector = "images.yaml"
)

// GetDefaultImageYAMLPath returns the path to the default images.yaml file.
func GetDefaultImageYAMLPath() string {
	return filepath.Join(common.ChartPath, DefaultImageVector)
}

// CreateImageVector creates an image vector from the default images.yaml file.
func CreateImageVector() (imagevector.ImageVector, error) {
	imageVector, err := imagevector.ReadGlobalImageVectorWithEnvOverride(GetDefaultImageYAMLPath())
	if err != nil {
		return nil, err
	}
	return imageVector, nil
}

// DecodeObject decodes and returns the specified object from the provided rendered chart.
func DecodeObject(renderedChart *chartrenderer.RenderedChart, path string, object interface{}) error {
	if content, ok := renderedChart.Files()[path]; ok {
		decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader([]byte(content)), 1024)
		return decoder.Decode(&object)
	}
	return fmt.Errorf("missing file %s in the rendered chart", path)
}
