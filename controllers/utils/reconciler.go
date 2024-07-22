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
	"path/filepath"

	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/gardener/etcd-druid/pkg/common"
	"github.com/gardener/gardener/pkg/utils/imagevector"
)

const (
	// defaultImageVector is a constant for the path to the default image vector file.
	defaultImageVector = "images.yaml"
)

// getImageYAMLPath returns the path to the image vector YAML file.
// The path to the default image vector YAML path is returned, unless `useEtcdWrapperImageVector`
// is set to true, in which case the path to the etcd wrapper image vector YAML is returned.
func getImageYAMLPath() string {
	return filepath.Join(common.ChartPath, defaultImageVector)
}

// CreateImageVector creates an image vector from the default images.yaml file or the images-wrapper.yaml file.
func CreateImageVector() (imagevector.ImageVector, error) {
	imageVector, err := imagevector.ReadGlobalImageVectorWithEnvOverride(getImageYAMLPath())
	if err != nil {
		return nil, err
	}
	return imageVector, nil
}

// HasOperationAnnotation checks if the given object has the operation annotation and its value is set to Reconcile.
func HasOperationAnnotation(obj client.Object) bool {
	return obj.GetAnnotations()[v1beta1constants.GardenerOperation] == v1beta1constants.GardenerOperationReconcile
}
