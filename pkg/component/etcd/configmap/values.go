// Copyright (c) 2022 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package configmap

import (
	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const (
	defaultClientPort = 2379
	defaultServerPort = 2380
)

// Values contains the values to create a configmap
type Values struct {
	// Name is the name of the configmap that holds the ETCD config
	Name string
	// EtcdUID is the UID of the etcd resource.
	EtcdUID types.UID
	// Metrics defines the level of detail for exported metrics of etcd, specify 'extensive' to include histogram metrics.
	Metrics *druidv1alpha1.MetricsLevel
	// Quota defines the etcd DB quota.
	Quota *resource.Quantity
	// InitialCluster is the initial cluster value to bootstrap ETCD.
	InitialCluster string
	// ClientUrlTLS hold the TLS configuration details for Client Communication
	ClientUrlTLS *druidv1alpha1.TLSConfig
	// PeerUrlTLS hold the TLS configuration details for Peer Communication
	PeerUrlTLS *druidv1alpha1.TLSConfig
	//ClientServiceName is name of the etcd client service
	ClientServiceName string
	// ClientPort holds the client port
	ClientPort *int32
	//PeerServiceName is name of the etcd peer service
	PeerServiceName string
	// ServerPort holds the peer port
	ServerPort *int32
	// AutoCompactionMode defines the auto-compaction-mode: 'periodic' or 'revision'.
	AutoCompactionMode *druidv1alpha1.CompactionMode
	//AutoCompactionRetention defines the auto-compaction-retention length for etcd as well as for embedded-Etcd of backup-restore sidecar.
	AutoCompactionRetention *string
	// ConfigMapChecksum is the checksum of deployed configmap
	ConfigMapChecksum string
	// Labels are the labels of deployed configmap
	Labels map[string]string
	// OwnerReference are the OwnerReference of the Configmap.
	OwnerReference metav1.OwnerReference
}
