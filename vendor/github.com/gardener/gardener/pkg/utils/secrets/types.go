// Copyright (c) 2018 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package secrets

import (
	"github.com/gardener/gardener/pkg/utils/infodata"
)

// ConfigInterface define functions needed for generating a specific secret.
type ConfigInterface interface {
	// GetName returns the name of the configuration.
	GetName() string
	// Generate generates a secret interface
	Generate() (DataInterface, error)
	// GenerateInfoData generates only the InfoData (metadata) which can later be used to generate a secret.
	GenerateInfoData() (infodata.InfoData, error)
	// GenerateFromInfoData combines the configuration and the provided InfoData (metadata) and generates a secret.
	GenerateFromInfoData(infoData infodata.InfoData) (DataInterface, error)
}

// DataInterface defines functions needed for defining the data map of a Kubernetes secret.
type DataInterface interface {
	// SecretData computes the data map which can be used in a Kubernetes secret.
	SecretData() map[string][]byte
}
