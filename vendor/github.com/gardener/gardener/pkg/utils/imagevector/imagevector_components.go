// Copyright (c) 2020 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package imagevector

import (
	"os"

	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/yaml"
)

const (
	// ComponentOverrideEnv is the name of the environment variable for image vector overrides of components deployed
	// by Gardener.
	ComponentOverrideEnv = "IMAGEVECTOR_OVERWRITE_COMPONENTS"
)

// ReadComponentOverwrite reads an ComponentImageVector from the given io.Reader.
func ReadComponentOverwrite(buf []byte) (ComponentImageVectors, error) {
	data := struct {
		Components []ComponentImageVector `json:"components" yaml:"components"`
	}{}

	if err := yaml.Unmarshal(buf, &data); err != nil {
		return nil, err
	}

	componentImageVectors := make(ComponentImageVectors, len(data.Components))
	for _, component := range data.Components {
		componentImageVectors[component.Name] = component.ImageVectorOverwrite
	}

	if errs := ValidateComponentImageVectors(componentImageVectors, field.NewPath("components")); len(errs) > 0 {
		return nil, errs.ToAggregate()
	}

	return componentImageVectors, nil
}

// ReadComponentOverwriteFile reads an ComponentImageVector from the file with the given name.
func ReadComponentOverwriteFile(name string) (ComponentImageVectors, error) {
	buf, err := os.ReadFile(name)
	if err != nil {
		return nil, err
	}

	return ReadComponentOverwrite(buf)
}
