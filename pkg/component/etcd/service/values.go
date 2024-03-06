// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package service

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Values contains the values necessary for creating ETCD services.
type Values struct {
	// BackupPort is the port exposed by the etcd-backup-restore side-car.
	BackupPort int32
	// ClientPort is the port exposed by etcd for client communication.
	ClientPort int32
	// ClientServiceName is the name of the service responsible for client traffic.
	ClientServiceName string
	// ClientAnnotations are the annotations to be added to the client service
	ClientServiceAnnotations map[string]string
	// ClientServiceLabels are the labels to be added to the client service
	ClientServiceLabels map[string]string
	// Labels are the service labels.
	Labels map[string]string
	// PeerServiceName is the name of the service responsible for peer traffic.
	PeerServiceName string
	// PeerPort is the port used for etcd peer communication.
	PeerPort int32
	// OwnerReference is the OwnerReference for the ETCD services.
	OwnerReference metav1.OwnerReference
	// SelectorLabels are the labels to be used in the Service.spec selector
	SelectorLabels map[string]string
}
