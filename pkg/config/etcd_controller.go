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

const (
	defaultEtcdWorkers = 3
)

// EtcdControllerConfig contains configuration for the etcd controller.
type EtcdControllerConfig struct {
	// Workers denotes the number of worker threads for the etcd controller.
	Workers int
}

// NewEtcdControllerConfig returns etcd controller configuration with default values set.
func NewEtcdControllerConfig() EtcdControllerConfig {
	return EtcdControllerConfig{
		Workers: defaultEtcdWorkers,
	}
}
