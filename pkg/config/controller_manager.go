// Copyright (c) 2023 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file.
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

package config

import (
	"k8s.io/client-go/tools/leaderelection/resourcelock"
)

// ControllerManagerConfig defaults
const (
	defaultMetricsAddr                        = ":8080"
	defaultEnableLeaderElection               = false
	defaultLeaderElectionID                   = "druid-leader-election"
	defaultLeaderElectionResourceLock         = resourcelock.LeasesResourceLock
	defaultIgnoreOperationAnnotation          = true
	defaultDisableEtcdServiceAccountAutomount = false
	defaultDisableLeaseCache                  = false
)

// ControllerManagerConfig contains configuration for the controller manager.
type ControllerManagerConfig struct {
	// MetricsAddr is the address the metric endpoint binds to.
	MetricsAddr string
	// EnableLeaderElection specifies whether to enable leader election for controller manager.
	EnableLeaderElection bool
	// LeaderElectionID is the name of the resource that leader election will use for holding the leader lock.
	LeaderElectionID string
	// LeaderElectionResourceLock specifies which resource type to use for leader election.
	LeaderElectionResourceLock string
	// IgnoreOperationAnnotation specifies whether to ignore or honour the operation annotation on resources to be reconciled.
	IgnoreOperationAnnotation bool
	// DisableEtcdServiceAccountAutomount specifies whether .automountServiceAccountToken should be set to false for the ServiceAccount created for etcd statefulsets.
	DisableEtcdServiceAccountAutomount bool
	// DisableLeaseCache specifies whether to disable cache for lease.coordination.k8s.io resources
	DisableLeaseCache bool
}

// NewControllerManagerConfig returns controller manager configuration with default values set.
func NewControllerManagerConfig() ControllerManagerConfig {
	return ControllerManagerConfig{
		MetricsAddr:                        defaultMetricsAddr,
		EnableLeaderElection:               defaultEnableLeaderElection,
		LeaderElectionID:                   defaultLeaderElectionID,
		LeaderElectionResourceLock:         defaultLeaderElectionResourceLock,
		IgnoreOperationAnnotation:          defaultIgnoreOperationAnnotation,
		DisableEtcdServiceAccountAutomount: defaultDisableEtcdServiceAccountAutomount,
		DisableLeaseCache:                  defaultDisableLeaseCache,
	}
}
