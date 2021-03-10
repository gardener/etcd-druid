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

package helper

import (
	"fmt"

	gardencore "github.com/gardener/gardener/pkg/apis/core"
	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	"github.com/gardener/gardener/pkg/gardenlet/apis/config"
	configv1alpha1 "github.com/gardener/gardener/pkg/gardenlet/apis/config/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

// SeedNameFromSeedConfig returns an empty string if the given seed config is nil, or the
// name inside the seed config.
func SeedNameFromSeedConfig(seedConfig *config.SeedConfig) string {
	if seedConfig == nil {
		return ""
	}
	return seedConfig.SeedTemplate.Name
}

// StaleExtensionHealthChecksThreshold returns nil if the given config is nil or the check
// for stale health checks is not enabled. Otherwise it returns the threshold from the given config.
func StaleExtensionHealthChecksThreshold(c *config.StaleExtensionHealthChecks) *metav1.Duration {
	if c != nil && c.Enabled {
		return c.Threshold
	}

	return nil
}

var scheme *runtime.Scheme

func init() {
	scheme = runtime.NewScheme()
	// core schemes are needed here to properly convert the embedded SeedTemplate objects
	utilruntime.Must(gardencore.AddToScheme(scheme))
	utilruntime.Must(gardencorev1beta1.AddToScheme(scheme))
	utilruntime.Must(config.AddToScheme(scheme))
	utilruntime.Must(configv1alpha1.AddToScheme(scheme))
}

// ConvertGardenletConfiguration converts the given object to an internal GardenletConfiguration version.
func ConvertGardenletConfiguration(obj runtime.Object) (*config.GardenletConfiguration, error) {
	obj, err := scheme.ConvertToVersion(obj, config.SchemeGroupVersion)
	if err != nil {
		return nil, err
	}
	result, ok := obj.(*config.GardenletConfiguration)
	if !ok {
		return nil, fmt.Errorf("could not convert GardenletConfiguration to internal version")
	}
	return result, nil
}

// ConvertGardenletConfigurationExternal converts the given object to an external  GardenletConfiguration version.
func ConvertGardenletConfigurationExternal(obj runtime.Object) (*configv1alpha1.GardenletConfiguration, error) {
	obj, err := scheme.ConvertToVersion(obj, configv1alpha1.SchemeGroupVersion)
	if err != nil {
		return nil, err
	}
	result, ok := obj.(*configv1alpha1.GardenletConfiguration)
	if !ok {
		return nil, fmt.Errorf("could not convert GardenletConfiguration to version %s", configv1alpha1.SchemeGroupVersion.String())
	}
	return result, nil
}
