// Copyright (c) 2023 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package utils

import (
	"fmt"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
)

var (
	deltaSnapshotPeriod = metav1.Duration{
		Duration: 300 * time.Second,
	}
	garbageCollectionPeriod = metav1.Duration{
		Duration: 43200 * time.Second,
	}
	clientPort              int32 = 2379
	serverPort              int32 = 2380
	backupPort              int32 = 8080
	imageEtcd                     = "eu.gcr.io/gardener-project/gardener/etcd:v3.4.13-bootstrap-8"
	imageBR                       = "eu.gcr.io/gardener-project/gardener/etcdbrctl:v0.21.0"
	snapshotSchedule              = "0 */24 * * *"
	defragSchedule                = "0 */24 * * *"
	container                     = "default.bkp"
	storageCapacity               = resource.MustParse("5Gi")
	storageClass                  = "default"
	priorityClassName             = "class_priority"
	deltaSnapShotMemLimit         = resource.MustParse("100Mi")
	autoCompactionMode            = druidv1alpha1.Periodic
	autoCompactionRetention       = "2m"
	quota                         = resource.MustParse("8Gi")
	localProvider                 = druidv1alpha1.StorageProvider("Local")
	prefix                        = "/tmp"
	uid                           = "a9b8c7d6e5f4"
	volumeClaimTemplateName       = "etcd-main"
	garbageCollectionPolicy       = druidv1alpha1.GarbageCollectionPolicy(druidv1alpha1.GarbageCollectionPolicyExponential)
	metricsBasic                  = druidv1alpha1.Basic
	etcdSnapshotTimeout           = metav1.Duration{
		Duration: 10 * time.Minute,
	}
	etcdDefragTimeout = metav1.Duration{
		Duration: 10 * time.Minute,
	}
)

type EtcdBuilder struct {
	etcd *druidv1alpha1.Etcd
}

func NewEtcdBuilder() *EtcdBuilder {
	return &EtcdBuilder{}
}

func (eb *EtcdBuilder) WithDefaults(name, namespace string) *EtcdBuilder {
	if eb == nil {
		return nil
	}
	eb.etcd = getDefaultEtcd(name, namespace)
	return eb
}

func (eb *EtcdBuilder) WithReplicas(replicas int32) *EtcdBuilder {
	if eb == nil || eb.etcd == nil {
		return nil
	}
	eb.etcd.Spec.Replicas = replicas
	return eb
}

func (eb *EtcdBuilder) WithTLS() *EtcdBuilder {
	if eb == nil || eb.etcd == nil {
		return nil
	}
	clientTLSConfig := &druidv1alpha1.TLSConfig{
		TLSCASecretRef: druidv1alpha1.SecretReference{
			SecretReference: corev1.SecretReference{
				Name: "client-url-ca-etcd",
			},
			DataKey: pointer.String("ca.crt"),
		},
		ClientTLSSecretRef: corev1.SecretReference{
			Name: "client-url-etcd-client-tls",
		},
		ServerTLSSecretRef: corev1.SecretReference{
			Name: "client-url-etcd-server-tls",
		},
	}

	peerTLSConfig := &druidv1alpha1.TLSConfig{
		TLSCASecretRef: druidv1alpha1.SecretReference{
			SecretReference: corev1.SecretReference{
				Name: "peer-url-ca-etcd",
			},
			DataKey: pointer.String("ca.crt"),
		},
		ServerTLSSecretRef: corev1.SecretReference{
			Name: "peer-url-etcd-server-tls",
		},
	}

	eb.etcd.Spec.Etcd.ClientUrlTLS = clientTLSConfig
	eb.etcd.Spec.Etcd.PeerUrlTLS = peerTLSConfig
	eb.etcd.Spec.Backup.TLS = clientTLSConfig

	return eb
}

func (eb *EtcdBuilder) WithProviderS3() *EtcdBuilder {
	if eb == nil || eb.etcd == nil {
		return nil
	}
	eb.etcd.Spec.Backup.Store = getBackupStore(
		eb.etcd.Name,
		druidv1alpha1.StorageProvider("aws"),
	)
	return eb
}

func (eb *EtcdBuilder) WithProviderABS() *EtcdBuilder {
	if eb == nil || eb.etcd == nil {
		return nil
	}
	eb.etcd.Spec.Backup.Store = getBackupStore(
		eb.etcd.Name,
		druidv1alpha1.StorageProvider("azure"),
	)
	return eb
}

func (eb *EtcdBuilder) WithProviderGCS() *EtcdBuilder {
	if eb == nil || eb.etcd == nil {
		return nil
	}
	eb.etcd.Spec.Backup.Store = getBackupStore(
		eb.etcd.Name,
		druidv1alpha1.StorageProvider("gcp"),
	)
	return eb
}

func (eb *EtcdBuilder) WithProviderSwift() *EtcdBuilder {
	if eb == nil || eb.etcd == nil {
		return nil
	}
	eb.etcd.Spec.Backup.Store = getBackupStore(
		eb.etcd.Name,
		druidv1alpha1.StorageProvider("openstack"),
	)
	return eb
}

func (eb *EtcdBuilder) WithProviderOSS() *EtcdBuilder {
	if eb == nil || eb.etcd == nil {
		return nil
	}
	eb.etcd.Spec.Backup.Store = getBackupStore(
		eb.etcd.Name,
		druidv1alpha1.StorageProvider("alicloud"),
	)
	return eb
}

func (eb *EtcdBuilder) Build() (*druidv1alpha1.Etcd, error) {
	if eb == nil {
		return nil, fmt.Errorf("EtcdBuilder is nil")
	}
	if eb.etcd == nil {
		return nil, fmt.Errorf("EtcdBuilder etcd is nil")
	}
	return eb.etcd, nil
}

func getDefaultEtcd(name, namespace string) *druidv1alpha1.Etcd {
	return &druidv1alpha1.Etcd{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       types.UID(uid),
		},
		Spec: druidv1alpha1.EtcdSpec{
			Annotations: map[string]string{
				"app":      "etcd-statefulset",
				"role":     "test",
				"instance": name,
			},
			Labels: map[string]string{
				"name":     "etcd",
				"instance": name,
			},
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"name":     "etcd",
					"instance": name,
				},
			},
			Replicas:            1,
			StorageCapacity:     &storageCapacity,
			StorageClass:        &storageClass,
			PriorityClassName:   &priorityClassName,
			VolumeClaimTemplate: &volumeClaimTemplateName,
			Backup: druidv1alpha1.BackupSpec{
				Image:                    &imageBR,
				Port:                     &backupPort,
				FullSnapshotSchedule:     &snapshotSchedule,
				GarbageCollectionPolicy:  &garbageCollectionPolicy,
				GarbageCollectionPeriod:  &garbageCollectionPeriod,
				DeltaSnapshotPeriod:      &deltaSnapshotPeriod,
				DeltaSnapshotMemoryLimit: &deltaSnapShotMemLimit,
				EtcdSnapshotTimeout:      &etcdSnapshotTimeout,

				Resources: &corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						"cpu":    parseQuantity("500m"),
						"memory": parseQuantity("2Gi"),
					},
					Requests: corev1.ResourceList{
						"cpu":    parseQuantity("23m"),
						"memory": parseQuantity("128Mi"),
					},
				},
				Store: &druidv1alpha1.StoreSpec{
					SecretRef: &corev1.SecretReference{
						Name: "etcd-backup",
					},
					Container: &container,
					Provider:  &localProvider,
					Prefix:    prefix,
				},
			},
			Etcd: druidv1alpha1.EtcdConfig{
				Quota:                   &quota,
				Metrics:                 &metricsBasic,
				Image:                   &imageEtcd,
				DefragmentationSchedule: &defragSchedule,
				EtcdDefragTimeout:       &etcdDefragTimeout,
				Resources: &corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						"cpu":    parseQuantity("2500m"),
						"memory": parseQuantity("4Gi"),
					},
					Requests: corev1.ResourceList{
						"cpu":    parseQuantity("500m"),
						"memory": parseQuantity("1000Mi"),
					},
				},
				ClientPort: &clientPort,
				ServerPort: &serverPort,
			},
			Common: druidv1alpha1.SharedConfig{
				AutoCompactionMode:      &autoCompactionMode,
				AutoCompactionRetention: &autoCompactionRetention,
			},
		},
	}
}

func parseQuantity(q string) resource.Quantity {
	val, _ := resource.ParseQuantity(q)
	return val
}

func getBackupStore(name string, provider druidv1alpha1.StorageProvider) *druidv1alpha1.StoreSpec {
	return &druidv1alpha1.StoreSpec{
		Container: &container,
		Prefix:    name,
		Provider:  &provider,
		SecretRef: &corev1.SecretReference{
			Name: "etcd-backup",
		},
	}
}
