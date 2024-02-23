// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"path/filepath"

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
