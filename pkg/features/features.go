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

package features

import (
	"k8s.io/component-base/featuregate"
)

const (
	// Every feature should add method here following this template:
	//
	// // MyFeature enables Foo.
	// // owner: @username
	// // alpha: v0.X
	// MyFeature featuregate.Feature = "MyFeature"

	// UseEtcdWrapper enables the use of etcd-wrapper image and a compatible version
	// of etcd-backup-restore, along with component-specific configuration
	// changes required for the usage of the etcd-wrapper image.
	// owner @unmarshall @aaronfern
	// alpha: v0.19
	UseEtcdWrapper featuregate.Feature = "UseEtcdWrapper"
)

// FeatureDescriptions stores descriptions for each defined feature
var FeatureDescriptions = map[featuregate.Feature]string{
	UseEtcdWrapper: "Enables the use of etcd-wrapper image and a compatible version of etcd-backup-restore, along with component-specific configuration changes necessary for the usage of the etcd-wrapper image.",
}

var defaultFeatures = map[featuregate.Feature]featuregate.FeatureSpec{
	UseEtcdWrapper: {Default: false, PreRelease: featuregate.Alpha},
}

// GetDefaultFeatures returns the default feature gates known to etcd-druid.
func GetDefaultFeatures() map[featuregate.Feature]featuregate.FeatureSpec {
	return defaultFeatures
}
