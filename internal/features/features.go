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
)

// GetDefaultFeatures returns the default feature gates known to etcd-druid.
func GetDefaultFeatures() map[featuregate.Feature]featuregate.FeatureSpec {
	return nil
}
