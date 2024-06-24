// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

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
	// beta:  v0.22
	UseEtcdWrapper featuregate.Feature = "UseEtcdWrapper"
)

var defaultFeatures = map[featuregate.Feature]featuregate.FeatureSpec{
	UseEtcdWrapper: {Default: true, PreRelease: featuregate.Beta},
}

// GetDefaultFeatures returns the default feature gates known to etcd-druid.
func GetDefaultFeatures() map[featuregate.Feature]featuregate.FeatureSpec {
	return defaultFeatures
}
