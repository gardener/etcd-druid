// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1_test

import (
	"testing"
	"time"

	druidapicommon "github.com/gardener/etcd-druid/api/common"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/gardener/etcd-druid/api/core/v1alpha1"
	. "github.com/onsi/gomega"
)

// These tests are written in BDD-style using Ginkgo framework. Refer to
// http://onsi.github.io/ginkgo to learn more.

func TestIsBackupStoreEnabled(t *testing.T) {
	etcd := createEtcd("foo", "default")
	tests := []struct {
		name     string
		backup   BackupSpec
		expected bool
	}{
		{
			name:     "when backup is enabled",
			backup:   BackupSpec{Store: &StoreSpec{}},
			expected: true,
		},
		{
			name:     "when backup is not enabled",
			backup:   BackupSpec{},
			expected: false,
		},
	}
	g := NewWithT(t)
	t.Parallel()

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			etcd.Spec.Backup = test.backup
			g.Expect(etcd.IsBackupStoreEnabled()).To(Equal(test.expected))
		})
	}

}

func TestIsReconciliationInProgress(t *testing.T) {
	tests := []struct {
		name     string
		lastOp   *druidapicommon.LastOperation
		expected bool
	}{
		{
			name: "when etcd status has druidapicommon.lastOperation set and its state is Processing",
			lastOp: &druidapicommon.LastOperation{
				Type:  LastOperationTypeReconcile,
				State: LastOperationStateProcessing,
			},
			expected: true,
		},
		{
			name: "when etcd status has druidapicommon.lastOperation set and its state is Error",
			lastOp: &druidapicommon.LastOperation{
				Type:  LastOperationTypeReconcile,
				State: LastOperationStateError,
			},
			expected: true,
		},
		{
			name: "when etcd status has druidapicommon.lastOperation set and its state is Succeeded",
			lastOp: &druidapicommon.LastOperation{
				Type:  LastOperationTypeReconcile,
				State: LastOperationStateSucceeded,
			},
			expected: false,
		},
		{
			name: "when etcd status has druidapicommon.lastOperation set and its type is delete",
			lastOp: &druidapicommon.LastOperation{
				Type:  LastOperationTypeDelete,
				State: LastOperationStateError,
			},
			expected: false,
		},
		{
			name: "when etcd status has druidapicommon.lastOperation set and its type is create",
			lastOp: &druidapicommon.LastOperation{
				Type:  LastOperationTypeCreate,
				State: LastOperationStateProcessing,
			},
			expected: false,
		},
		{
			name:     "when etcd status does not have druidapicommon.lastOperation set",
			lastOp:   nil,
			expected: false,
		},
	}

	g := NewWithT(t)
	t.Parallel()

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			etcd := createEtcd("foo", "default")
			etcd.Status.LastOperation = test.lastOp
			g.Expect(etcd.IsReconciliationInProgress()).To(Equal(test.expected))
		})
	}
}

func createEtcd(name, namespace string) *Etcd {
	var (
		clientPort    int32 = 2379
		serverPort    int32 = 2380
		backupPort    int32 = 8080
		wrapperPort   int32 = 9095
		metricLevel         = Basic
		snapshotCount int64 = 10000
	)

	garbageCollectionPeriod := metav1.Duration{
		Duration: 43200 * time.Second,
	}
	deltaSnapshotPeriod := metav1.Duration{
		Duration: 300 * time.Second,
	}
	imageEtcd := "europe-docker.pkg.dev/gardener-project/public/gardener/etcd-wrapper:v0.1.0"
	imageBR := "europe-docker.pkg.dev/gardener-project/public/gardener/etcdbrctl:v0.25.0"
	snapshotSchedule := "0 */24 * * *"
	defragSchedule := "0 */24 * * *"
	container := "my-object-storage-container-name"
	storageCapacity := resource.MustParse("20Gi")
	deltaSnapShotMemLimit := resource.MustParse("100Mi")
	quota := resource.MustParse("8Gi")
	storageClass := "gardener.cloud-fast"
	provider := StorageProvider("aws")
	prefix := "etcd-test"
	garbageCollectionPolicy := GarbageCollectionPolicy(GarbageCollectionPolicyExponential)

	clientTlsConfig := &TLSConfig{
		TLSCASecretRef: SecretReference{
			SecretReference: corev1.SecretReference{
				Name: "client-url-ca-etcd",
			},
		},
		ClientTLSSecretRef: corev1.SecretReference{
			Name: "client-url-etcd-client-tls",
		},
		ServerTLSSecretRef: corev1.SecretReference{
			Name: "client-url-etcd-server-tls",
		},
	}

	peerTlsConfig := &TLSConfig{
		TLSCASecretRef: SecretReference{
			SecretReference: corev1.SecretReference{
				Name: "peer-url-ca-etcd",
			},
		},
		ServerTLSSecretRef: corev1.SecretReference{
			Name: "peer-url-etcd-server-tls",
		},
	}

	etcdbrTLSConfig := &TLSConfig{
		TLSCASecretRef: SecretReference{
			SecretReference: corev1.SecretReference{
				Name: "ca-etcdbr",
			},
		},
		ServerTLSSecretRef: corev1.SecretReference{
			Name: "etcdbr-server-tls",
		},
		ClientTLSSecretRef: corev1.SecretReference{
			Name: "etcdbr-client-tls",
		},
	}

	instance := &Etcd{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       "123456",
		},
		Spec: EtcdSpec{
			Annotations: map[string]string{
				"app":  "etcd-statefulset",
				"role": "test",
			},
			Labels: map[string]string{
				"app":  "etcd-statefulset",
				"role": "test",
				"name": name,
			},
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app":  "etcd-statefulset",
					"name": name,
				},
			},
			Replicas:        1,
			StorageClass:    &storageClass,
			StorageCapacity: &storageCapacity,

			Backup: BackupSpec{
				Image:                    &imageBR,
				Port:                     &backupPort,
				TLS:                      etcdbrTLSConfig,
				FullSnapshotSchedule:     &snapshotSchedule,
				GarbageCollectionPolicy:  &garbageCollectionPolicy,
				GarbageCollectionPeriod:  &garbageCollectionPeriod,
				DeltaSnapshotPeriod:      &deltaSnapshotPeriod,
				DeltaSnapshotMemoryLimit: &deltaSnapShotMemLimit,

				Resources: &corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						"cpu":    resource.MustParse("500m"),
						"memory": resource.MustParse("2Gi"),
					},
					Requests: corev1.ResourceList{
						"cpu":    resource.MustParse("23m"),
						"memory": resource.MustParse("128Mi"),
					},
				},
				Store: &StoreSpec{
					SecretRef: &corev1.SecretReference{
						Name: "etcd-backup",
					},
					Container: &container,
					Provider:  &provider,
					Prefix:    prefix,
				},
			},
			Etcd: EtcdConfig{
				Quota:                   &quota,
				Metrics:                 &metricLevel,
				Image:                   &imageEtcd,
				DefragmentationSchedule: &defragSchedule,
				Resources: &corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						"cpu":    resource.MustParse("2500m"),
						"memory": resource.MustParse("4Gi"),
					},
					Requests: corev1.ResourceList{
						"cpu":    resource.MustParse("500m"),
						"memory": resource.MustParse("1000Mi"),
					},
				},
				ClientPort:    &clientPort,
				ServerPort:    &serverPort,
				WrapperPort:   &wrapperPort,
				SnapshotCount: &snapshotCount,
				ClientUrlTLS:  clientTlsConfig,
				PeerUrlTLS:    peerTlsConfig,
			},
		},
	}
	return instance
}
