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

package service

import (
	"k8s.io/utils/pointer"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/pkg/utils"
)

const (
	defaultBackupPort = 8080
	defaultClientPort = 2379
	defaultServerPort = 2380
)

// GenerateValues generates `service.Values` for the service component with the given `etcd` object.
func GenerateValues(etcd *druidv1alpha1.Etcd) Values {
	var clientServiceAnnotations map[string]string
	if etcd.Spec.Etcd.ClientService != nil {
		clientServiceAnnotations = etcd.Spec.Etcd.ClientService.Annotations
	}
	var clientServiceLabels map[string]string
	if etcd.Spec.Etcd.ClientService != nil {
		clientServiceLabels = etcd.Spec.Etcd.ClientService.Labels
	}

	return Values{
		BackupPort:               pointer.Int32Deref(etcd.Spec.Backup.Port, defaultBackupPort),
		ClientPort:               pointer.Int32Deref(etcd.Spec.Etcd.ClientPort, defaultClientPort),
		ClientServiceName:        utils.GetClientServiceName(etcd),
		ClientServiceAnnotations: clientServiceAnnotations,
		ClientServiceLabels:      clientServiceLabels,
		EtcdName:                 etcd.Name,
		EtcdUID:                  etcd.UID,
		Labels:                   etcd.Spec.Labels,
		PeerServiceName:          utils.GetPeerServiceName(etcd),
		ServerPort:               pointer.Int32Deref(etcd.Spec.Etcd.ServerPort, defaultServerPort),
	}
}
