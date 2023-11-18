package etcd

import (
	"errors"
	"time"

	"github.com/gardener/etcd-druid/internal/controller/utils"
	"github.com/gardener/etcd-druid/internal/features"
	flag "github.com/spf13/pflag"
	"k8s.io/component-base/featuregate"
)

// Flag names
const (
	workersFlagName                            = "etcd-workers"
	ignoreOperationAnnotationFlagName          = "ignore-operation-annotation"
	enableEtcdSpecAutoReconcileFlagName        = "enable-etcd-spec-auto-reconcile"
	disableEtcdServiceAccountAutomountFlagName = "disable-etcd-serviceaccount-automount"
	etcdStatusSyncPeriodFlagName               = "etcd-status-sync-period"
)

const (
	defaultWorkers                            = 3
	defaultIgnoreOperationAnnotation          = false
	defaultDisableEtcdServiceAccountAutomount = false
	defaultEnableEtcdSpecAutoReconcile        = false
	defaultEtcdStatusSyncPeriod               = 15 * time.Second
)

// featureList holds the feature gate names that are relevant for the Etcd Controller.
var featureList = []featuregate.Feature{
	features.UseEtcdWrapper,
}

// Config defines the configuration for the Etcd Controller.
type Config struct {
	// Workers is the number of workers concurrently processing reconciliation requests.
	Workers int
	// IgnoreOperationAnnotation specifies whether to ignore or honour the operation annotation on resources to be reconciled.
	// Deprecated: Use EnableEtcdSpecAutoReconcile instead.
	IgnoreOperationAnnotation bool
	// EnableEtcdSpecAutoReconcile controls how the Etcd Spec is reconciled. If set to true, then any change in Etcd spec
	// will automatically trigger a reconciliation of the Etcd resource. If set to false, then an operator needs to
	// explicitly set gardener.cloud/operation=reconcile annotation on the Etcd resource to trigger reconciliation
	// of the Etcd spec.
	// NOTE: Decision to enable it should be carefully taken as spec updates could potentially result in rolling update
	// of the StatefulSet which will cause a minor downtime for a single node etcd cluster and can potentially cause a
	// downtime for a multi-node etcd cluster.
	EnableEtcdSpecAutoReconcile bool
	// DisableEtcdServiceAccountAutomount controls the auto-mounting of service account token for ETCD StatefulSets.
	DisableEtcdServiceAccountAutomount bool
	// EtcdStatusSyncPeriod is the duration after which an event will be re-queued ensuring ETCD status synchronization.
	EtcdStatusSyncPeriod time.Duration
	// FeatureGates contains the feature gates to be used by Etcd Controller.
	FeatureGates map[featuregate.Feature]bool
}

// InitFromFlags initializes the config from the provided CLI flag set.
func InitFromFlags(fs *flag.FlagSet, cfg *Config) {
	fs.IntVar(&cfg.Workers, workersFlagName, defaultWorkers,
		"Number of workers spawned for concurrent reconciles of etcd spec and status changes. If not specified then default of 3 is assumed.")
	flag.BoolVar(&cfg.IgnoreOperationAnnotation, ignoreOperationAnnotationFlagName, defaultIgnoreOperationAnnotation,
		"Specifies whether to ignore or honour the operation annotation on resources to be reconciled.")
	flag.BoolVar(&cfg.EnableEtcdSpecAutoReconcile, enableEtcdSpecAutoReconcileFlagName, defaultEnableEtcdSpecAutoReconcile,
		"If true then automatically reconciles Etcd Spec. If false waits for explicit annotation to be placed on the Etcd resource to trigger reconcile.")
	fs.BoolVar(&cfg.DisableEtcdServiceAccountAutomount, disableEtcdServiceAccountAutomountFlagName, defaultDisableEtcdServiceAccountAutomount,
		"If true then .automountServiceAccountToken will be set to false for the ServiceAccount created for etcd StatefulSets.")
	fs.DurationVar(&cfg.EtcdStatusSyncPeriod, etcdStatusSyncPeriodFlagName, defaultEtcdStatusSyncPeriod,
		"Period after which an etcd status sync will be attempted.")
}

// Validate validates the config.
func (cfg *Config) Validate() error {
	var errs []error
	if err := utils.MustBeGreaterThan(workersFlagName, 0, cfg.Workers); err != nil {
		errs = append(errs, err)
	}
	if err := utils.MustBeGreaterThan(etcdStatusSyncPeriodFlagName, 0, cfg.EtcdStatusSyncPeriod); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// CaptureFeatureActivations captures all feature gates required by the controller into controller config
func (cfg *Config) CaptureFeatureActivations(fg featuregate.FeatureGate) {
	if cfg.FeatureGates == nil {
		cfg.FeatureGates = make(map[featuregate.Feature]bool)
	}
	for _, feature := range featureList {
		cfg.FeatureGates[feature] = fg.Enabled(feature)
	}
}
