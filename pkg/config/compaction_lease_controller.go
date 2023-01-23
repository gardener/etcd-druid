// Copyright (c) 2021 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package config

import "time"

const (
	defaultEnableBackupCompaction = false
	defaultCompactionWorkers      = 3
	defaultEventsThreshold        = 1000000
	defaultActiveDeadlineDuration = 3 * time.Hour
)

// CompactionLeaseControllerConfig contains configuration for the compaction lease controller.
type CompactionLeaseControllerConfig struct {
	// EnableBackupCompaction specifies whether backup compaction should be enabled.
	EnableBackupCompaction bool
	// Workers denotes the number of worker threads for the compaction lease controller.
	Workers int
	// EventsThreshold denotes total number of etcd events to be reached upon which a backup compaction job is triggered.
	EventsThreshold int64
	// ActiveDeadlineDuration is the duration after which a running compaction job will be killed.
	ActiveDeadlineDuration time.Duration
}

// NewCompactionLeaseControllerConfig returns compaction lease controller configuration with default values set.
func NewCompactionLeaseControllerConfig() CompactionLeaseControllerConfig {
	return CompactionLeaseControllerConfig{
		EnableBackupCompaction: defaultEnableBackupCompaction,
		Workers:                defaultCompactionWorkers,
		EventsThreshold:        defaultEventsThreshold,
		ActiveDeadlineDuration: defaultActiveDeadlineDuration,
	}
}