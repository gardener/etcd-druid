// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package images

import (
	_ "embed"

	"github.com/gardener/gardener/pkg/utils/imagevector"
)

var (
	//go:embed images.yaml
	imagesYAML string
)

// CreateImageVector creates an image vector from the default images.yaml file or the images-wrapper.yaml file.
func CreateImageVector() (imagevector.ImageVector, error) {
	imgVec, err := imagevector.Read([]byte(imagesYAML))
	if err != nil {
		return nil, err
	}
	return imagevector.WithEnvOverride(imgVec)
}
