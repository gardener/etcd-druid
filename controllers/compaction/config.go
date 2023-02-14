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

package compaction

import (
	"flag"
	"time"
)

const (
	enableBackupCompactionFlagName = "enable-backup-compaction"
	compactionWorkersFlagName      = "compaction-workers"
	eventsThresholdFlagName        = "etcd-events-threshold"
	activeDeadlineDurationFlagName = "active-deadline-duration"

	defaultEnableBackupCompaction = false
	defaultCompactionWorkers      = 3
	defaultEventsThreshold        = 1000000
	defaultActiveDeadlineDuration = 3 * time.Hour
)

// Config contains configuration for the compaction controller.
type Config struct {
	// EnableBackupCompaction specifies whether backup compaction should be enabled.
	EnableBackupCompaction bool
	// Workers denotes the number of worker threads for the compaction controller.
	Workers int
	// EventsThreshold denotes total number of etcd events to be reached upon which a backup compaction job is triggered.
	EventsThreshold int64
	// ActiveDeadlineDuration is the duration after which a running compaction job will be killed.
	ActiveDeadlineDuration time.Duration
}

func InitFromFlags(fs *flag.FlagSet, cfg *Config) {
	fs.BoolVar(&cfg.EnableBackupCompaction, enableBackupCompactionFlagName, defaultEnableBackupCompaction,
		"Enable automatic compaction of etcd backups.")
	fs.IntVar(&cfg.Workers, compactionWorkersFlagName, defaultCompactionWorkers,
		"Number of worker threads of the CompactionJob controller. The controller creates a backup compaction job if a certain etcd event threshold is reached. Setting this flag to 0 disables the controller.")
	fs.Int64Var(&cfg.EventsThreshold, eventsThresholdFlagName, defaultEventsThreshold,
		"Total number of etcd events that can be allowed before a backup compaction job is triggered.")
	fs.DurationVar(&cfg.ActiveDeadlineDuration, activeDeadlineDurationFlagName, defaultActiveDeadlineDuration,
		"Duration after which a running backup compaction job will be killed (Ex: \"300ms\", \"20s\", \"-1.5h\" or \"2h45m\").\").")
}
