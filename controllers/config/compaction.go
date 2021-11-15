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

// CompactionLeaseConfig contains configuration for the compaction controller.
type CompactionLeaseConfig struct {
	// ActiveDeadlineDuration is the duration after which a running compaction job will be killed (Ex: "300ms", "20s", "-1.5h" or "2h45m")
	ActiveDeadlineDuration time.Duration
	// EventsThreshold is total number of etcd events that can be allowed before a backup compaction job is triggered
	EventsThreshold int64
}
