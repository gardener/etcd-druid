// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"errors"
	"fmt"
)

// Constants for feature names.
const (
	// UseEtcdWrapper is the name of the feature which enables usage of etcd-wrapper.
	// This is now GA. Any attempt to disable this feature will be an error.
	UseEtcdWrapper = "UseEtcdWrapper"
	// AllowEmptyDir is the name of the feature which allows the usage of emptyDir volumes for etcd pods for data directories.
	// This feature is currently in alpha.
	AllowEmptyDir = "AllowEmptyDir"
)

// maturityLevelSpec is the specification of maturity level for a feature.
type maturityLevelSpec struct {
	maturityLevel    maturityLevel
	enabledByDefault bool
	lockedToDefault  bool
}

// These are pre-defined maturityLevelSpec which should be used instead of creating new ones.
// These ensure that invalid values for maturityLevelSpec.enabledByDefault and maturityLevelSpec.lockedToDefault are not used.
var (
	maturityLevelSpecAlpha = maturityLevelSpec{
		maturityLevel:    alpha,
		enabledByDefault: false,
		lockedToDefault:  false,
	}
	maturityLevelSpecBeta = maturityLevelSpec{
		maturityLevel:    beta,
		enabledByDefault: true,
		lockedToDefault:  false,
	}
	maturityLevelSpecGA = maturityLevelSpec{
		maturityLevel:    ga,
		enabledByDefault: true,
		lockedToDefault:  true,
	}
)

// maturityLevel is the maturity level of a feature.
type maturityLevel string

const (
	// alpha indicates that the feature is in an alpha state.
	// Consumer needs to explicitly enable this feature via
	alpha maturityLevel = "alpha"
	// beta indicates that the feature is in a beta state.
	// It is more stable than alpha but may still have some issues.
	// It is recommended for testing and evaluation.
	beta maturityLevel = "beta"
	// ga (General Availability) indicates that the feature is stable and ready for production use.
	// It has been tested and is expected to work reliably.
	ga maturityLevel = "ga"
)

// featureGate allows consumers to add features and to query whether a feature is enabled or not.
type featureGate struct {
	knownFeatures   map[string]maturityLevelSpec
	enabledFeatures map[string]bool
}

// newFeatureGate creates a new featureGate instance.
func newFeatureGate() featureGate {
	return featureGate{
		knownFeatures:   make(map[string]maturityLevelSpec),
		enabledFeatures: make(map[string]bool),
	}
}

// DefaultFeatureGates is the only instance of featureGate that should be used in the codebase.
// It is initialized with all existing feature maturityLevelSpec's and is used to also record the enabled state of features.
var DefaultFeatureGates = newFeatureGate()

// init initializes the featureGate with all existing feature specifications.
// If and when a new feature is introduced then it should be ensured that it is added to the featureGate using this function.
func init() {
	DefaultFeatureGates.knownFeatures[UseEtcdWrapper] = maturityLevelSpecGA
	DefaultFeatureGates.knownFeatures[AllowEmptyDir] = maturityLevelSpecAlpha
}

// IsEnabled checks if a feature is enabled.
func (f *featureGate) IsEnabled(feature string) bool {
	return f.enabledFeatures[feature]
}

// SetEnabledFeaturesFromMap sets enabled state for features from the given map.
// It will return an error if one of the following conditions is met:
// 1. The feature is not known.
// 2. The feature is locked to a default value. As an example if a consumer attempts to disable a ga feature then that will be disallowed as it has been locked to true by default.
func (f *featureGate) SetEnabledFeaturesFromMap(featureMap map[string]bool) error {
	var errs []error
	for feature, enabled := range featureMap {
		maturityLevelSpec, ok := f.knownFeatures[feature]
		if !ok {
			errs = append(errs, fmt.Errorf("unknown feature %s", feature))
			continue
		}
		if maturityLevelSpec.lockedToDefault && enabled != maturityLevelSpec.enabledByDefault {
			errs = append(errs, fmt.Errorf("feature %s is locked to default value %t", feature, maturityLevelSpec.enabledByDefault))
			continue
		}
		f.enabledFeatures[feature] = enabled
	}
	return errors.Join(errs...)
}
