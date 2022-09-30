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

import "k8s.io/apimachinery/pkg/types"

type Values struct {
	// BackupPort is the port exposed by the etcd-backup-restore side-car.
	BackupPort int32
	// ClientPort is the port exposed by etcd for client communication.
	ClientPort int32
	// ClientServiceName is the name of the service responsible for client traffic.
	ClientServiceName string
	// ClientAnnotations are the annotations to be added to the client service
	ClientServiceAnnotations map[string]string
	// EtcdName is the name of the etcd resource.
	EtcdName string
	// EtcdName is the UID of the etcd resource.
	EtcdUID types.UID
	// Labels are the service labels.
	Labels map[string]string
	// PeerServiceName is the name of the service responsible for peer traffic.
	PeerServiceName string
	// ServerPort is the port used for etcd peer communication.
	ServerPort int32
}
