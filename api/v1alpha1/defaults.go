// Copyright (c) 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package v1alpha1

const (
	defaultEtcdServerPort int32 = 2380
	defaultEtcdClientPort int32 = 2379
	defaultBackupPort     int32 = 8080
)

// TODO: Use apimachinery DefaulterFunc using default-gen generator

// GetCompletedEtcdNodes returns the completed value of number of etcd nodes.
func (etcd *Etcd) GetCompletedEtcdNodes() int32 {
	if etcd.Spec.Replicas != nil {
		return *etcd.Spec.Replicas
	}
	return 1
}

// GetCompletedEtcdServerPort returns the completed value of etcd server port.
func (etcd *Etcd) GetCompletedEtcdServerPort() int32 {
	if etcd.Spec.Etcd.ServerPort != nil {
		return *etcd.Spec.Etcd.ServerPort
	}
	return defaultEtcdServerPort
}

// GetCompletedEtcdClientPort returns the completed value of etcd client port.
func (etcd *Etcd) GetCompletedEtcdClientPort() int32 {
	if etcd.Spec.Etcd.ClientPort != nil {
		return *etcd.Spec.Etcd.ClientPort
	}
	return defaultEtcdClientPort
}

// GetCompletedBackupServerPort returns the completed value of backup server port.
func (etcd *Etcd) GetCompletedBackupServerPort() int32 {
	if etcd.Spec.Backup.Port != nil {
		return *etcd.Spec.Backup.Port
	}
	return defaultBackupPort
}
