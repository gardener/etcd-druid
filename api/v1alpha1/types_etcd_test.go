// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	. "github.com/gardener/etcd-druid/api/v1alpha1"
)

// These tests are written in BDD-style using Ginkgo framework. Refer to
// http://onsi.github.io/ginkgo to learn more.

var _ = Describe("Etcd", func() {
	var (
		etcd *Etcd
	)

	BeforeEach(func() {
		etcd = getEtcd("foo", "default")
	})

	Context("GetPeerServiceName", func() {
		It("should return the correct peer service name", func() {
			Expect(etcd.GetPeerServiceName()).To(Equal("foo-peer"))
		})
	})

	Context("GetClientServiceName", func() {
		It("should return the correct client service name", func() {
			Expect(etcd.GetClientServiceName()).To(Equal("foo-client"))
		})
	})

	Context("GetServiceAccountName", func() {
		It("should return the correct service account name", func() {
			Expect(etcd.GetServiceAccountName()).To(Equal("foo"))
		})
	})

	Context("GetConfigmapName", func() {
		It("should return the correct configmap name", func() {
			Expect(etcd.GetConfigMapName()).To(Equal("etcd-bootstrap-123456"))
		})
	})

	Context("GetCompactionJobName", func() {
		It("should return the correct compaction job name", func() {
			Expect(etcd.GetCompactionJobName()).To(Equal("foo-compactor"))
		})
	})

	Context("GetOrdinalPodName", func() {
		It("should return the correct ordinal pod name", func() {
			Expect(etcd.GetOrdinalPodName(0)).To(Equal("foo-0"))
		})
	})

	Context("GetDeltaSnapshotLeaseName", func() {
		It("should return the correct delta snapshot lease name", func() {
			Expect(etcd.GetDeltaSnapshotLeaseName()).To(Equal("foo-delta-snap"))
		})
	})

	Context("GetFullSnapshotLeaseName", func() {
		It("should return the correct full snapshot lease name", func() {
			Expect(etcd.GetFullSnapshotLeaseName()).To(Equal("foo-full-snap"))
		})
	})

	Context("GetDefaultLabels", func() {
		It("should return the default labels for etcd", func() {
			expected := map[string]string{
				LabelManagedByKey: LabelManagedByValue,
				LabelPartOfKey:    "foo",
			}
			Expect(etcd.GetDefaultLabels()).To(Equal(expected))
		})
	})

	Context("GetAsOwnerReference", func() {
		It("should return an OwnerReference object that represents the current Etcd instance", func() {
			expected := metav1.OwnerReference{
				APIVersion:         GroupVersion.String(),
				Kind:               "Etcd",
				Name:               "foo",
				UID:                "123456",
				Controller:         pointer.Bool(true),
				BlockOwnerDeletion: pointer.Bool(true),
			}
			Expect(etcd.GetAsOwnerReference()).To(Equal(expected))
		})
	})

	Context("GetRoleName", func() {
		It("should return the role name for the Etcd", func() {
			Expect(etcd.GetRoleName()).To(Equal(GroupVersion.Group + ":etcd:foo"))
		})
	})

	Context("GetRoleBindingName", func() {
		It("should return the rolebinding name for the Etcd", func() {
			Expect(etcd.GetRoleName()).To(Equal(GroupVersion.Group + ":etcd:foo"))
		})
	})

	Context("IsBackupStoreEnabled", func() {
		Context("when backup is enabled", func() {
			It("should return true", func() {
				Expect(etcd.IsBackupStoreEnabled()).To(Equal(true))
			})
		})
		Context("when backup is not enabled", func() {
			It("should return false", func() {
				etcd.Spec.Backup = BackupSpec{}
				Expect(etcd.IsBackupStoreEnabled()).To(Equal(false))
			})
		})
	})

	Context("IsMarkedForDeletion", func() {
		Context("when deletion timestamp is not set", func() {
			It("should return false", func() {
				Expect(etcd.IsMarkedForDeletion()).To(Equal(false))
			})
		})
		Context("when deletion timestamp is set", func() {
			It("should return true", func() {
				etcd.DeletionTimestamp = &metav1.Time{Time: time.Now()}
				Expect(etcd.IsMarkedForDeletion()).To(Equal(true))
			})
		})
	})

	Context("GetSuspendEtcdSpecReconcileAnnotationKey", func() {
		ignoreReconciliationAnnotation := IgnoreReconciliationAnnotation
		suspendEtcdSpecReconcileAnnotation := SuspendEtcdSpecReconcileAnnotation
		Context("when etcd has only ignore-reconcile annotation set", func() {
			It("should return druid.gardener.cloud/ignore-reconciliation", func() {
				etcd.Annotations = map[string]string{
					IgnoreReconciliationAnnotation: "true",
				}
				Expect(etcd.GetSuspendEtcdSpecReconcileAnnotationKey()).To(Equal(&ignoreReconciliationAnnotation))
			})
		})
		Context("when etcd has only suspend-etcd-spec-reconcile annotation set", func() {
			It("should return suspend-etcd-spec-reconcile annotation", func() {
				etcd.Annotations = map[string]string{
					SuspendEtcdSpecReconcileAnnotation: "",
				}
				Expect(etcd.GetSuspendEtcdSpecReconcileAnnotationKey()).To(Equal(&suspendEtcdSpecReconcileAnnotation))
			})
		})
		Context("when etcd has both annotations druid.gardener.cloud/suspend-etcd-spec-reconcile and druid.gardener.cloud/ignore-reconciliation set", func() {
			It("should return suspend-etcd-spec-reconcile annotation", func() {
				etcd.Annotations = map[string]string{
					SuspendEtcdSpecReconcileAnnotation: "",
					IgnoreReconciliationAnnotation:     "",
				}
				Expect(etcd.GetSuspendEtcdSpecReconcileAnnotationKey()).To(Equal(&suspendEtcdSpecReconcileAnnotation))
			})
		})
		Context("when etcd does not have suspend-etcd-spec-reconcile or ignore-reconcile annotation set", func() {
			It("should return nil string pointer", func() {
				var nilString *string
				Expect(etcd.GetSuspendEtcdSpecReconcileAnnotationKey()).To(Equal(nilString))
			})
		})
	})

	Context("AreManagedResourcesProtected", func() {
		Context("when etcd has annotation druid.gardener.cloud/disable-resource-protection", func() {
			It("should return false", func() {
				etcd.Annotations = map[string]string{
					DisableResourceProtectionAnnotation: "",
				}
				Expect(etcd.AreManagedResourcesProtected()).To(Equal(false))
			})
		})
		Context("when etcd does not have annotation druid.gardener.cloud/disable-resource-protection set", func() {
			It("should return true", func() {
				Expect(etcd.AreManagedResourcesProtected()).To(Equal(true))
			})
		})
	})

	Context("IsReconciliationInProgress", func() {
		Context("when etcd status has lastOperation and its state is Processing", func() {
			It("should return true", func() {
				etcd.Status.LastOperation = &LastOperation{
					State: LastOperationStateProcessing,
				}
				Expect(etcd.IsReconciliationInProgress()).To(Equal(true))
			})
		})
		Context("when etcd status has lastOperation and its state is Error", func() {
			It("should return true", func() {
				etcd.Status.LastOperation = &LastOperation{
					State: LastOperationStateError,
				}
				Expect(etcd.IsReconciliationInProgress()).To(Equal(true))
			})
		})
		Context("when etcd status has lastOperation and its state is neither Processing or Error", func() {
			It("should return false", func() {
				etcd.Status.LastOperation = &LastOperation{
					State: LastOperationStateSucceeded,
				}
				Expect(etcd.IsReconciliationInProgress()).To(Equal(false))
			})
		})
		Context("when etcd status does not have lastOperation populated", func() {
			It("should return false", func() {
				Expect(etcd.IsReconciliationInProgress()).To(Equal(false))
			})
		})
	})
})

func getEtcd(name, namespace string) *Etcd {
	var (
		clientPort  int32 = 2379
		serverPort  int32 = 2380
		backupPort  int32 = 8080
		metricLevel       = Basic
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
				TLS:                      clientTlsConfig,
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
				ClientPort:   &clientPort,
				ServerPort:   &serverPort,
				ClientUrlTLS: clientTlsConfig,
				PeerUrlTLS:   peerTlsConfig,
			},
		},
	}
	return instance
}
