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
	defaultCustodianWorkers            = 3
	defaultCustodianSyncPeriod         = 30 * time.Second
	defaultEtcdMemberNotReadyThreshold = 5 * time.Minute
	defaultEtcdMemberUnknownThreshold  = 1 * time.Minute
)

// CustodianControllerConfig contains configuration for the etcd custodian controller.
type CustodianControllerConfig struct {
	// Workers denotes the number of worker threads for the custodian controller.
	Workers int
	// SyncPeriod is the duration after which re-enqueuing happens.
	SyncPeriod time.Duration
	// EtcdMember holds configuration related to etcd members.
	EtcdMember EtcdMemberConfig
}

// EtcdMemberConfig holds configuration related to etcd members.
type EtcdMemberConfig struct {
	// EtcdMemberNotReadyThreshold is the duration after which an etcd member's state is considered `NotReady`.
	EtcdMemberNotReadyThreshold time.Duration
	// EtcdMemberUnknownThreshold is the duration after which an etcd member's state is considered `Unknown`.
	EtcdMemberUnknownThreshold time.Duration
}

// NewCustodianControllerConfig returns custodian controller configuration with default values set.
func NewCustodianControllerConfig() CustodianControllerConfig {
	return CustodianControllerConfig{
		Workers:    defaultCustodianWorkers,
		SyncPeriod: defaultCustodianSyncPeriod,
		EtcdMember: EtcdMemberConfig{
			EtcdMemberNotReadyThreshold: defaultEtcdMemberNotReadyThreshold,
			EtcdMemberUnknownThreshold:  defaultEtcdMemberUnknownThreshold,
		},
	}
}
