package v1alpha1

type FeatureName string

const (
	UseEtcdWrapper FeatureName = "UseEtcdWrapper"
)

// MaturityLevel represents the maturity level of a feature.
type MaturityLevelSpec struct {
	MaturityLevel   MaturityLevel
	Default         bool
	LockedToDefault bool
}

type MaturityLevel string

const (
	// Alpha indicates that the feature is in an alpha state.
	// Consumer needs to explicitly enable this feature via
	Alpha MaturityLevel = "Alpha"
	// Beta indicates that the feature is in a beta state.
	// It is more stable than alpha but may still have some issues.
	// It is recommended for testing and evaluation.
	Beta MaturityLevel = "Beta"
	// GA (General Availability) indicates that the feature is stable and ready for production use.
	// It has been tested and is expected to work reliably.
	GA MaturityLevel = "GA"
)

var (
	maturityLevelSpecAlpha = MaturityLevelSpec{
		MaturityLevel:   Alpha,
		Default:         false,
		LockedToDefault: false,
	}
	maturityLevelSpecBeta = MaturityLevelSpec{
		MaturityLevel:   Beta,
		Default:         true,
		LockedToDefault: false,
	}
	maturityLevelSpecGA = MaturityLevelSpec{
		MaturityLevel:   GA,
		Default:         true,
		LockedToDefault: true,
	}
)

type FeatureGate struct {
}

func (f *FeatureGate) IsEnabled(feature FeatureName) bool {

}

func (f *FeatureGate) SetFromMap(featureMap map[FeatureName]bool) error {
}

type FeatureSpec struct {
	Name              string
	MaturityLevelSpec MaturityLevelSpec
}
