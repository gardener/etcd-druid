// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package compaction

import (
	"time"

	"github.com/gardener/etcd-druid/internal/controller/utils"

	flag "github.com/spf13/pflag"
	"k8s.io/component-base/featuregate"
)

// featureList holds the feature gate names that are relevant for the Compaction Controller.
var featureList []featuregate.Feature

const (
	enableBackupCompactionFlagName    = "enable-backup-compaction"
	workersFlagName                   = "compaction-workers"
	eventsThresholdFlagName           = "etcd-events-threshold"
	activeDeadlineDurationFlagName    = "active-deadline-duration"
	metricsScrapeWaitDurationFlagName = "metrics-scrape-wait-duration"

	defaultEnableBackupCompaction    = false
	defaultCompactionWorkers         = 3
	defaultEventsThreshold           = 1000000
	defaultActiveDeadlineDuration    = 3 * time.Hour
	defaultMetricsScrapeWaitDuration = 0
	defaultMinWorkers                = 1
)

// Config contains configuration for the Compaction Controller.
type Config struct {
	// EnableBackupCompaction specifies whether backup compaction should be enabled.
	EnableBackupCompaction bool
	// Workers denotes the number of worker threads for the compaction controller.
	Workers int
	// EventsThreshold denotes total number of etcd events to be reached upon which a backup compaction job is triggered.
	EventsThreshold int64
	// ActiveDeadlineDuration is the duration after which a running compaction job will be killed.
	ActiveDeadlineDuration time.Duration
	// MetricsScrapeWaitDuration is the duration to wait for after compaction job is completed, to allow Prometheus metrics to be scraped
	MetricsScrapeWaitDuration time.Duration
	// FeatureGates contains the feature gates to be used by Compaction Controller.
	FeatureGates map[featuregate.Feature]bool
}

// InitFromFlags initializes the compaction controller config from the provided CLI flag set.
func (cfg *Config) InitFromFlags(fs *flag.FlagSet) {
	fs.BoolVar(&cfg.EnableBackupCompaction, enableBackupCompactionFlagName, defaultEnableBackupCompaction,
		"Enable automatic compaction of etcd backups.")
	fs.IntVar(&cfg.Workers, workersFlagName, defaultCompactionWorkers,
		"Number of worker threads of the CompactionJob controller. The controller creates a backup compaction job if a certain etcd event threshold is reached. If compaction is enabled, the value for this flag must be greater than zero.")
	fs.Int64Var(&cfg.EventsThreshold, eventsThresholdFlagName, defaultEventsThreshold,
		"Total number of etcd events that can be allowed before a backup compaction job is triggered.")
	fs.DurationVar(&cfg.ActiveDeadlineDuration, activeDeadlineDurationFlagName, defaultActiveDeadlineDuration,
		"Duration after which a running backup compaction job will be terminated.")
	fs.DurationVar(&cfg.MetricsScrapeWaitDuration, metricsScrapeWaitDurationFlagName, defaultMetricsScrapeWaitDuration,
		"Duration to wait for after compaction job is completed, to allow Prometheus metrics to be scraped.")
}

// Validate validates the config.
func (cfg *Config) Validate() error {
	if cfg.EnableBackupCompaction {
		if err := utils.MustBeGreaterThanOrEqualTo(workersFlagName, defaultMinWorkers, cfg.Workers); err != nil {
			return err
		}
		if err := utils.MustBeGreaterThan(eventsThresholdFlagName, 0, cfg.EventsThreshold); err != nil {
			return err
		}
		if err := utils.MustBeGreaterThan(activeDeadlineDurationFlagName, 0, cfg.ActiveDeadlineDuration.Seconds()); err != nil {
			return err
		}
	}
	return nil
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
