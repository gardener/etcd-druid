// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"k8s.io/utils/pointer"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
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
		ClientServiceName:        etcd.GetClientServiceName(),
		ClientServiceAnnotations: clientServiceAnnotations,
		ClientServiceLabels:      clientServiceLabels,
		SelectorLabels:           etcd.GetDefaultLabels(),
		Labels:                   etcd.GetDefaultLabels(),
		PeerServiceName:          etcd.GetPeerServiceName(),
		PeerPort:                 pointer.Int32Deref(etcd.Spec.Etcd.ServerPort, defaultServerPort),
		OwnerReference:           etcd.GetAsOwnerReference(),
	}
}
