// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1_test

import (
	"context"
	"time"

	testutils "github.com/gardener/etcd-druid/test/utils"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"

	. "github.com/gardener/etcd-druid/api/v1alpha1"
)

// These tests are written in BDD-style using Ginkgo framework. Refer to
// http://onsi.github.io/ginkgo to learn more.

var _ = Describe("Etcd", func() {
	var (
		key              types.NamespacedName
		created, fetched *Etcd
	)

	BeforeEach(func() {
		created = getEtcd("foo", "default")
	})

	Context("Create API", func() {

		It("should create an object successfully", func() {

			key = types.NamespacedName{
				Name:      "foo",
				Namespace: "default",
			}

			By("creating an API obj")
			Expect(k8sClient.Create(context.Background(), created)).To(Succeed())

			fetched = &Etcd{}
			Expect(k8sClient.Get(context.Background(), key, fetched)).To(Succeed())
			Expect(fetched).To(Equal(created))

			By("deleting the created object")
			Expect(k8sClient.Delete(context.Background(), created)).To(Succeed())
			Expect(k8sClient.Get(context.Background(), key, created)).ToNot(Succeed())
		})

	})

	Context("GetPeerServiceName", func() {
		It("should return the correct peer service name", func() {
			Expect(created.GetPeerServiceName()).To(Equal("foo-peer"))
		})
	})

	Context("GetClientServiceName", func() {
		It("should return the correct client service name", func() {
			Expect(created.GetClientServiceName()).To(Equal("foo-client"))
		})
	})

	Context("GetServiceAccountName", func() {
		It("should return the correct service account name", func() {
			Expect(created.GetServiceAccountName()).To(Equal("foo"))
		})
	})

	Context("GetConfigmapName", func() {
		It("should return the correct configmap name", func() {
			Expect(created.GetConfigmapName()).To(Equal("etcd-bootstrap-123456"))
		})
	})

	Context("GetCompactionJobName", func() {
		It("should return the correct compaction job name", func() {
			Expect(created.GetCompactionJobName()).To(Equal("foo-compactor"))
		})
	})

	Context("GetOrdinalPodName", func() {
		It("should return the correct ordinal pod name", func() {
			Expect(created.GetOrdinalPodName(0)).To(Equal("foo-0"))
		})
	})

	Context("GetDeltaSnapshotLeaseName", func() {
		It("should return the correct delta snapshot lease name", func() {
			Expect(created.GetDeltaSnapshotLeaseName()).To(Equal("foo-delta-snap"))
		})
	})

	Context("GetFullSnapshotLeaseName", func() {
		It("should return the correct full snapshot lease name", func() {
			Expect(created.GetFullSnapshotLeaseName()).To(Equal("foo-full-snap"))
		})
	})

	Context("GetDefaultLabels", func() {
		It("should return the default labels for etcd", func() {
			expected := map[string]string{
				"name":     "etcd",
				"instance": "foo",
			}
			Expect(created.GetDefaultLabels()).To(Equal(expected))
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
			Expect(created.GetAsOwnerReference()).To(Equal(expected))
		})
	})

	Context("GetRoleName", func() {
		It("should return the role name for the Etcd", func() {
			Expect(created.GetRoleName()).To(Equal(GroupVersion.Group + ":etcd:foo"))
		})
	})

	Context("GetRoleBindingName", func() {
		It("should return the rolebinding name for the Etcd", func() {
			Expect(created.GetRoleName()).To(Equal(GroupVersion.Group + ":etcd:foo"))
		})
	})
})

func getEtcd(name, namespace string) *Etcd {
	var (
		clientPort  int32        = 2379
		serverPort  int32        = 2380
		backupPort  int32        = 8080
		metricLevel MetricsLevel = Basic
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
						"cpu":    testutils.ParseQuantity("500m"),
						"memory": testutils.ParseQuantity("2Gi"),
					},
					Requests: corev1.ResourceList{
						"cpu":    testutils.ParseQuantity("23m"),
						"memory": testutils.ParseQuantity("128Mi"),
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
						"cpu":    testutils.ParseQuantity("2500m"),
						"memory": testutils.ParseQuantity("4Gi"),
					},
					Requests: corev1.ResourceList{
						"cpu":    testutils.ParseQuantity("500m"),
						"memory": testutils.ParseQuantity("1000Mi"),
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
