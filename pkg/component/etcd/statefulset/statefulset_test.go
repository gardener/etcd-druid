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

package statefulset_test

import (
	"context"
	"fmt"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/pkg/client/kubernetes"
	"github.com/gardener/etcd-druid/pkg/common"
	. "github.com/gardener/etcd-druid/pkg/component/etcd/statefulset"
	druidutils "github.com/gardener/etcd-druid/pkg/utils"

	"github.com/gardener/gardener/pkg/operation/botanist/component"
	kutil "github.com/gardener/gardener/pkg/utils/kubernetes"
	. "github.com/gardener/gardener/pkg/utils/test/matchers"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var (
	backupRestore       = "backup-restore"
	deltaSnapshotPeriod = metav1.Duration{
		Duration: 300 * time.Second,
	}
	garbageCollectionPeriod = metav1.Duration{
		Duration: 43200 * time.Second,
	}
	clientPort              int32 = 2379
	serverPort              int32 = 2380
	backupPort              int32 = 8080
	uid                           = "a9b8c7d6e5f4"
	imageEtcd                     = "eu.gcr.io/gardener-project/gardener/etcd:v3.4.13-bootstrap"
	imageBR                       = "eu.gcr.io/gardener-project/gardener/etcdbrctl:v0.12.0"
	snapshotSchedule              = "0 */24 * * *"
	defragSchedule                = "0 */24 * * *"
	container                     = "default.bkp"
	storageCapacity               = resource.MustParse("5Gi")
	storageClass                  = "gardener.fast"
	priorityClassName             = "class_priority"
	deltaSnapShotMemLimit         = resource.MustParse("100Mi")
	autoCompactionMode            = druidv1alpha1.Periodic
	autoCompactionRetention       = "2m"
	quota                         = resource.MustParse("8Gi")
	prefix                        = "/tmp"
	volumeClaimTemplateName       = "etcd-main"
	garbageCollectionPolicy       = druidv1alpha1.GarbageCollectionPolicy(druidv1alpha1.GarbageCollectionPolicyExponential)
	metricsBasic                  = druidv1alpha1.Basic
	etcdSnapshotTimeout           = metav1.Duration{
		Duration: 10 * time.Minute,
	}
	etcdDefragTimeout = metav1.Duration{
		Duration: 10 * time.Minute,
	}
	etcdConnectionTimeout = metav1.Duration{
		Duration: 5 * time.Minute,
	}
	ownerName          = "owner.foo.example.com"
	ownerID            = "bar"
	ownerCheckInterval = metav1.Duration{
		Duration: 30 * time.Second,
	}
	ownerCheckTimeout = metav1.Duration{
		Duration: 2 * time.Minute,
	}
	ownerCheckDNSCacheTTL = metav1.Duration{
		Duration: 1 * time.Minute,
	}
	heartbeatDuration = metav1.Duration{
		Duration: 10 * time.Second,
	}
)

var _ = Describe("Statefulset", func() {
	var (
		ctx context.Context
		cl  client.Client

		etcd      *druidv1alpha1.Etcd
		namespace string
		name      string

		replicas *int32
		sts      *appsv1.StatefulSet

		values      Values
		stsDeployer component.Deployer
	)

	JustBeforeEach(func() {
		etcd = getEtcd(name, namespace, true, *replicas)
		values = GenerateValues(etcd, pointer.Int32Ptr(clientPort), pointer.Int32Ptr(serverPort), pointer.Int32Ptr(backupPort), imageEtcd, imageBR)
		stsDeployer = New(cl, logr.Discard(), values)

		sts = &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{

				Name:      values.Name,
				Namespace: values.Namespace,
			},
		}
	})

	BeforeEach(func() {
		ctx = context.Background()
		cl = fakeclient.NewClientBuilder().WithScheme(kubernetes.Scheme).Build()

		name = "statefulset"
		namespace = "default"
		quota = resource.MustParse("8Gi")

		if replicas == nil {
			replicas = pointer.Int32Ptr(1)
		}
	})

	Describe("#Deploy", func() {
		Context("when statefulset does not exist", func() {
			It("should create the statefulset successfully", func() {
				Expect(stsDeployer.Deploy(ctx)).To(Succeed())

				sts := &appsv1.StatefulSet{}

				Expect(cl.Get(ctx, kutil.Key(namespace, values.Name), sts)).To(Succeed())
				checkStatefulset(sts, values)
			})
		})

		Context("when statefulset exists", func() {
			It("should update the statefulset successfully", func() {
				// The generation is usually increased by the Kube-Apiserver but as we use a fake client here, we need to manually do it.
				sts.Generation = 1
				Expect(cl.Create(ctx, sts)).To(Succeed())

				Expect(stsDeployer.Deploy(ctx)).To(Succeed())

				sts := &appsv1.StatefulSet{}

				Expect(cl.Get(ctx, kutil.Key(namespace, values.Name), sts)).To(Succeed())
				checkStatefulset(sts, values)
			})

			Context("when multi-node cluster is configured", func() {
				BeforeEach(func() {
					replicas = pointer.Int32(3)
				})

				It("should re-create statefulset because serviceName is changed", func() {
					sts.Generation = 1
					sts.Spec.ServiceName = "foo"
					sts.Spec.Replicas = pointer.Int32Ptr(3)
					sts.Status.Replicas = 1
					Expect(cl.Create(ctx, sts)).To(Succeed())

					values.Replicas = 3
					Expect(stsDeployer.Deploy(ctx)).To(Succeed())

					sts := &appsv1.StatefulSet{}
					Expect(cl.Get(ctx, kutil.Key(namespace, values.Name), sts)).To(Succeed())
					checkStatefulset(sts, values)
				})
			})
		})
	})

	Describe("#Destroy", func() {
		Context("when statefulset does not exist", func() {
			It("should destroy successfully", func() {
				Expect(stsDeployer.Destroy(ctx)).To(Succeed())
				Expect(cl.Get(ctx, client.ObjectKeyFromObject(sts), &appsv1.StatefulSet{})).To(BeNotFoundError())
			})
		})

		Context("when statefulset exists", func() {
			It("should destroy successfully", func() {
				Expect(cl.Create(ctx, sts)).To(Succeed())

				Expect(stsDeployer.Destroy(ctx)).To(Succeed())

				Expect(cl.Get(ctx, kutil.Key(namespace, sts.Name), &appsv1.StatefulSet{})).To(BeNotFoundError())
			})
		})
	})
})

func checkStatefulset(sts *appsv1.StatefulSet, values Values) {
	checkStsMetadata(sts.ObjectMeta.OwnerReferences, values)

	readinessProbeUrl := fmt.Sprintf("https://%s-local:%d/health", values.Name, clientPort)
	if int(values.Replicas) == 1 {
		readinessProbeUrl = fmt.Sprintf("https://%s-local:%d/healthz", values.Name, backupPort)
	}

	store, err := druidutils.StorageProviderFromInfraProvider(values.BackupStore.Provider)
	Expect(err).NotTo(HaveOccurred())
	Expect(*sts).To(MatchFields(IgnoreExtras, Fields{
		"ObjectMeta": MatchFields(IgnoreExtras, Fields{
			"Name":      Equal(values.Name),
			"Namespace": Equal(values.Namespace),
			"Annotations": MatchAllKeys(Keys{
				"gardener.cloud/owned-by":   Equal(fmt.Sprintf("%s/%s", values.Namespace, values.Name)),
				"gardener.cloud/owner-type": Equal("etcd"),
				"app":                       Equal("etcd-statefulset"),
				"role":                      Equal("test"),
				"instance":                  Equal(values.Name),
			}),
			"Labels": MatchAllKeys(Keys{
				"name":     Equal("etcd"),
				"instance": Equal(values.Name),
				"foo":      Equal("bar"),
			}),
		}),

		"Spec": MatchFields(IgnoreExtras, Fields{
			"UpdateStrategy": MatchFields(IgnoreExtras, Fields{
				"Type": Equal(appsv1.RollingUpdateStatefulSetStrategyType),
			}),
			"Replicas": PointTo(Equal(values.Replicas)),
			"Selector": PointTo(MatchFields(IgnoreExtras, Fields{
				"MatchLabels": MatchAllKeys(Keys{
					"name":     Equal("etcd"),
					"instance": Equal(values.Name),
				}),
			})),
			"Template": MatchFields(IgnoreExtras, Fields{
				"ObjectMeta": MatchFields(IgnoreExtras, Fields{
					"Annotations": MatchKeys(IgnoreExtras, Keys{
						"app":      Equal("etcd-statefulset"),
						"role":     Equal("test"),
						"instance": Equal(values.Name),
					}),
					"Labels": MatchAllKeys(Keys{
						"name":     Equal("etcd"),
						"instance": Equal(values.Name),
						"foo":      Equal("bar"),
					}),
				}),
				//s.Spec.Template.Spec.HostAliases
				"Spec": MatchFields(IgnoreExtras, Fields{
					"HostAliases": MatchAllElements(hostAliasIterator, Elements{
						"127.0.0.1": MatchFields(IgnoreExtras, Fields{
							"IP": Equal("127.0.0.1"),
							"Hostnames": MatchAllElements(cmdIterator, Elements{
								fmt.Sprintf("%s-local", values.Name): Equal(fmt.Sprintf("%s-local", values.Name)),
							}),
						}),
					}),
					"Containers": MatchAllElements(containerIterator, Elements{
						common.Etcd: MatchFields(IgnoreExtras, Fields{
							"Ports": ConsistOf([]corev1.ContainerPort{
								{
									Name:          "server",
									Protocol:      corev1.ProtocolTCP,
									HostPort:      0,
									ContainerPort: *values.ServerPort,
								},
								{
									Name:          "client",
									Protocol:      corev1.ProtocolTCP,
									HostPort:      0,
									ContainerPort: *values.ClientPort,
								},
							}),
							"Command": MatchAllElements(cmdIterator, Elements{
								"/var/etcd/bin/bootstrap.sh": Equal("/var/etcd/bin/bootstrap.sh"),
							}),
							"ImagePullPolicy": Equal(corev1.PullIfNotPresent),
							"Image":           Equal(values.EtcdImage),
							"ReadinessProbe": PointTo(MatchFields(IgnoreExtras, Fields{
								"Handler": MatchFields(IgnoreExtras, Fields{
									"Exec": PointTo(MatchFields(IgnoreExtras, Fields{
										"Command": MatchAllElements(cmdIterator, Elements{
											"/usr/bin/curl":                       Equal("/usr/bin/curl"),
											"--cert":                              Equal("--cert"),
											"/var/etcd/ssl/client/client/tls.crt": Equal("/var/etcd/ssl/client/client/tls.crt"),
											"--key":                               Equal("--key"),
											"/var/etcd/ssl/client/client/tls.key": Equal("/var/etcd/ssl/client/client/tls.key"),
											"--cacert":                            Equal("--cacert"),
											"/var/etcd/ssl/client/ca/ca.crt":      Equal("/var/etcd/ssl/client/ca/ca.crt"),
											readinessProbeUrl:                     Equal(readinessProbeUrl),
										}),
									})),
								}),
								"InitialDelaySeconds": Equal(int32(15)),
								"PeriodSeconds":       Equal(int32(5)),
							})),
							"LivenessProbe": PointTo(MatchFields(IgnoreExtras, Fields{
								"Handler": MatchFields(IgnoreExtras, Fields{
									"Exec": PointTo(MatchFields(IgnoreExtras, Fields{
										"Command": MatchAllElements(cmdIterator, Elements{
											"/bin/sh":       Equal("/bin/sh"),
											"-ec":           Equal("-ec"),
											"ETCDCTL_API=3": Equal("ETCDCTL_API=3"),
											"etcdctl":       Equal("etcdctl"),
											"--cert=/var/etcd/ssl/client/client/tls.crt":                            Equal("--cert=/var/etcd/ssl/client/client/tls.crt"),
											"--key=/var/etcd/ssl/client/client/tls.key":                             Equal("--key=/var/etcd/ssl/client/client/tls.key"),
											"--cacert=/var/etcd/ssl/client/ca/ca.crt":                               Equal("--cacert=/var/etcd/ssl/client/ca/ca.crt"),
											fmt.Sprintf("--endpoints=https://%s-local:%d", values.Name, clientPort): Equal(fmt.Sprintf("--endpoints=https://%s-local:%d", values.Name, clientPort)),
											"get":             Equal("get"),
											"foo":             Equal("foo"),
											"--consistency=s": Equal("--consistency=s"),
										}),
									})),
								}),
								"InitialDelaySeconds": Equal(int32(15)),
								"PeriodSeconds":       Equal(int32(5)),
							})),
							"VolumeMounts": MatchAllElements(volumeMountIterator, Elements{
								values.VolumeClaimTemplateName: MatchFields(IgnoreExtras, Fields{
									"Name":      Equal(values.VolumeClaimTemplateName),
									"MountPath": Equal("/var/etcd/data/"),
								}),
								"client-url-ca-etcd": MatchFields(IgnoreExtras, Fields{
									"Name":      Equal("client-url-ca-etcd"),
									"MountPath": Equal("/var/etcd/ssl/client/ca"),
								}),
								"client-url-etcd-server-tls": MatchFields(IgnoreExtras, Fields{
									"Name":      Equal("client-url-etcd-server-tls"),
									"MountPath": Equal("/var/etcd/ssl/client/server"),
								}),
								"client-url-etcd-client-tls": MatchFields(IgnoreExtras, Fields{
									"Name":      Equal("client-url-etcd-client-tls"),
									"MountPath": Equal("/var/etcd/ssl/client/client"),
								}),
								"peer-url-ca-etcd": MatchFields(IgnoreExtras, Fields{
									"Name":      Equal("peer-url-ca-etcd"),
									"MountPath": Equal("/var/etcd/ssl/peer/ca"),
								}),
								"peer-url-etcd-server-tls": MatchFields(IgnoreExtras, Fields{
									"Name":      Equal("peer-url-etcd-server-tls"),
									"MountPath": Equal("/var/etcd/ssl/peer/server"),
								}),
							}),
						}),

						backupRestore: MatchFields(IgnoreExtras, Fields{
							"Command": MatchAllElements(cmdIterator, Elements{
								"etcdbrctl": Equal("etcdbrctl"),
								"server":    Equal("server"),
								"--cert=/var/etcd/ssl/client/client/tls.crt":                                                          Equal("--cert=/var/etcd/ssl/client/client/tls.crt"),
								"--key=/var/etcd/ssl/client/client/tls.key":                                                           Equal("--key=/var/etcd/ssl/client/client/tls.key"),
								"--cacert=/var/etcd/ssl/client/ca/ca.crt":                                                             Equal("--cacert=/var/etcd/ssl/client/ca/ca.crt"),
								"--server-cert=/var/etcd/ssl/client/server/tls.crt":                                                   Equal("--server-cert=/var/etcd/ssl/client/server/tls.crt"),
								"--server-key=/var/etcd/ssl/client/server/tls.key":                                                    Equal("--server-key=/var/etcd/ssl/client/server/tls.key"),
								"--data-dir=/var/etcd/data/new.etcd":                                                                  Equal("--data-dir=/var/etcd/data/new.etcd"),
								"--insecure-transport=false":                                                                          Equal("--insecure-transport=false"),
								"--insecure-skip-tls-verify=false":                                                                    Equal("--insecure-skip-tls-verify=false"),
								"--snapstore-temp-directory=/var/etcd/data/temp":                                                      Equal("--snapstore-temp-directory=/var/etcd/data/temp"),
								"--etcd-process-name=etcd":                                                                            Equal("--etcd-process-name=etcd"),
								fmt.Sprintf("%s=%s", "--etcd-connection-timeout", etcdConnectionTimeout.Duration.String()):            Equal(fmt.Sprintf("%s=%s", "--etcd-connection-timeout", values.LeaderElection.EtcdConnectionTimeout.Duration.String())),
								"--enable-snapshot-lease-renewal=true":                                                                Equal("--enable-snapshot-lease-renewal=true"),
								"--enable-member-lease-renewal=true":                                                                  Equal("--enable-member-lease-renewal=true"),
								"--k8s-heartbeat-duration=10s":                                                                        Equal("--k8s-heartbeat-duration=10s"),
								fmt.Sprintf("--defragmentation-schedule=%s", *values.DefragmentationSchedule):                         Equal(fmt.Sprintf("--defragmentation-schedule=%s", *values.DefragmentationSchedule)),
								fmt.Sprintf("--schedule=%s", *values.FullSnapshotSchedule):                                            Equal(fmt.Sprintf("--schedule=%s", *values.FullSnapshotSchedule)),
								fmt.Sprintf("%s=%s", "--garbage-collection-policy", *values.GarbageCollectionPolicy):                  Equal(fmt.Sprintf("%s=%s", "--garbage-collection-policy", *values.GarbageCollectionPolicy)),
								fmt.Sprintf("%s=%s", "--storage-provider", store):                                                     Equal(fmt.Sprintf("%s=%s", "--storage-provider", store)),
								fmt.Sprintf("%s=%s", "--store-prefix", values.BackupStore.Prefix):                                     Equal(fmt.Sprintf("%s=%s", "--store-prefix", values.BackupStore.Prefix)),
								fmt.Sprintf("--delta-snapshot-memory-limit=%d", values.DeltaSnapshotMemoryLimit.Value()):              Equal(fmt.Sprintf("--delta-snapshot-memory-limit=%d", values.DeltaSnapshotMemoryLimit.Value())),
								fmt.Sprintf("--garbage-collection-policy=%s", *values.GarbageCollectionPolicy):                        Equal(fmt.Sprintf("--garbage-collection-policy=%s", *values.GarbageCollectionPolicy)),
								fmt.Sprintf("--endpoints=https://%s-local:%d", values.Name, clientPort):                               Equal(fmt.Sprintf("--endpoints=https://%s-local:%d", values.Name, clientPort)),
								fmt.Sprintf("--embedded-etcd-quota-bytes=%d", int64(values.Quota.Value())):                            Equal(fmt.Sprintf("--embedded-etcd-quota-bytes=%d", int64(values.Quota.Value()))),
								fmt.Sprintf("%s=%s", "--delta-snapshot-period", values.DeltaSnapshotPeriod.Duration.String()):         Equal(fmt.Sprintf("%s=%s", "--delta-snapshot-period", values.DeltaSnapshotPeriod.Duration.String())),
								fmt.Sprintf("%s=%s", "--garbage-collection-period", values.GarbageCollectionPeriod.Duration.String()): Equal(fmt.Sprintf("%s=%s", "--garbage-collection-period", values.GarbageCollectionPeriod.Duration.String())),
								fmt.Sprintf("%s=%s", "--auto-compaction-mode", *values.AutoCompactionMode):                            Equal(fmt.Sprintf("%s=%s", "--auto-compaction-mode", *values.AutoCompactionMode)),
								fmt.Sprintf("%s=%s", "--auto-compaction-retention", *values.AutoCompactionRetention):                  Equal(fmt.Sprintf("%s=%s", "--auto-compaction-retention", *values.AutoCompactionRetention)),
								fmt.Sprintf("%s=%s", "--etcd-snapshot-timeout", values.EtcdSnapshotTimeout.Duration.String()):         Equal(fmt.Sprintf("%s=%s", "--etcd-snapshot-timeout", values.EtcdSnapshotTimeout.Duration.String())),
								fmt.Sprintf("%s=%s", "--etcd-defrag-timeout", values.EtcdDefragTimeout.Duration.String()):             Equal(fmt.Sprintf("%s=%s", "--etcd-defrag-timeout", values.EtcdDefragTimeout.Duration.String())),
								fmt.Sprintf("%s=%s", "--owner-name", values.OwnerCheck.Name):                                          Equal(fmt.Sprintf("%s=%s", "--owner-name", values.OwnerCheck.Name)),
								fmt.Sprintf("%s=%s", "--owner-id", values.OwnerCheck.ID):                                              Equal(fmt.Sprintf("%s=%s", "--owner-id", values.OwnerCheck.ID)),
								fmt.Sprintf("%s=%s", "--owner-check-interval", values.OwnerCheck.Interval.Duration.String()):          Equal(fmt.Sprintf("%s=%s", "--owner-check-interval", values.OwnerCheck.Interval.Duration.String())),
								fmt.Sprintf("%s=%s", "--owner-check-timeout", values.OwnerCheck.Timeout.Duration.String()):            Equal(fmt.Sprintf("%s=%s", "--owner-check-timeout", values.OwnerCheck.Timeout.Duration.String())),
								fmt.Sprintf("%s=%s", "--owner-check-dns-cache-ttl", values.OwnerCheck.DNSCacheTTL.Duration.String()):  Equal(fmt.Sprintf("%s=%s", "--owner-check-dns-cache-ttl", values.OwnerCheck.DNSCacheTTL.Duration.String())),
								fmt.Sprintf("%s=%s", "--delta-snapshot-lease-name", values.DeltaSnapLeaseName):                        Equal(fmt.Sprintf("%s=%s", "--delta-snapshot-lease-name", values.DeltaSnapLeaseName)),
								fmt.Sprintf("%s=%s", "--full-snapshot-lease-name", values.FullSnapLeaseName):                          Equal(fmt.Sprintf("%s=%s", "--full-snapshot-lease-name", values.FullSnapLeaseName)),
							}),
							"Ports": ConsistOf([]corev1.ContainerPort{
								{
									Name:          "server",
									Protocol:      corev1.ProtocolTCP,
									HostPort:      0,
									ContainerPort: *values.BackupPort,
								},
							}),
							"Image":           Equal(values.BackupImage),
							"ImagePullPolicy": Equal(corev1.PullIfNotPresent),
							"VolumeMounts": MatchElements(volumeMountIterator, IgnoreExtras, Elements{
								values.VolumeClaimTemplateName: MatchFields(IgnoreExtras, Fields{
									"Name":      Equal(values.VolumeClaimTemplateName),
									"MountPath": Equal("/var/etcd/data"),
								}),
								"etcd-config-file": MatchFields(IgnoreExtras, Fields{
									"Name":      Equal("etcd-config-file"),
									"MountPath": Equal("/var/etcd/config/"),
								}),
								"etcd-backup": MatchFields(IgnoreExtras, Fields{
									"Name":      Equal("etcd-backup"),
									"MountPath": Equal("/root/etcd-backup/"),
								}),
							}),
							"Env": MatchElements(envIterator, IgnoreExtras, Elements{
								"STORAGE_CONTAINER": MatchFields(IgnoreExtras, Fields{
									"Name":  Equal("STORAGE_CONTAINER"),
									"Value": Equal(*values.BackupStore.Container),
								}),
								"POD_NAME": MatchFields(IgnoreExtras, Fields{
									"Name": Equal("POD_NAME"),
									"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
										"FieldRef": PointTo(MatchFields(IgnoreExtras, Fields{
											"FieldPath": Equal("metadata.name"),
										})),
									})),
								}),
								"POD_NAMESPACE": MatchFields(IgnoreExtras, Fields{
									"Name": Equal("POD_NAMESPACE"),
									"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
										"FieldRef": PointTo(MatchFields(IgnoreExtras, Fields{
											"FieldPath": Equal("metadata.namespace"),
										})),
									})),
								}),
								"AZURE_APPLICATION_CREDENTIALS": MatchFields(IgnoreExtras, Fields{
									"Name":  Equal("AZURE_APPLICATION_CREDENTIALS"),
									"Value": Equal("/root/etcd-backup"),
								}),
							}),
							"SecurityContext": PointTo(MatchFields(IgnoreExtras, Fields{
								"Capabilities": PointTo(MatchFields(IgnoreExtras, Fields{
									"Add": ConsistOf([]corev1.Capability{
										"SYS_PTRACE",
									}),
								})),
							})),
						}),
					}),
					"ShareProcessNamespace": Equal(pointer.BoolPtr(true)),
					"Volumes": MatchAllElements(volumeIterator, Elements{
						"etcd-config-file": MatchFields(IgnoreExtras, Fields{
							"Name": Equal("etcd-config-file"),
							"VolumeSource": MatchFields(IgnoreExtras, Fields{
								"ConfigMap": PointTo(MatchFields(IgnoreExtras, Fields{
									"LocalObjectReference": MatchFields(IgnoreExtras, Fields{
										"Name": Equal(fmt.Sprintf("etcd-bootstrap-%s", string(values.EtcdUID[:6]))),
									}),
									"DefaultMode": PointTo(Equal(int32(0644))),
									"Items": MatchAllElements(keyIterator, Elements{
										"etcd.conf.yaml": MatchFields(IgnoreExtras, Fields{
											"Key":  Equal("etcd.conf.yaml"),
											"Path": Equal("etcd.conf.yaml"),
										}),
									}),
								})),
							}),
						}),
						"etcd-backup": MatchFields(IgnoreExtras, Fields{
							"Name": Equal("etcd-backup"),
							"VolumeSource": MatchFields(IgnoreExtras, Fields{
								"Secret": PointTo(MatchFields(IgnoreExtras, Fields{
									"SecretName": Equal(values.BackupStore.SecretRef.Name),
								})),
							}),
						}),
						"client-url-etcd-server-tls": MatchFields(IgnoreExtras, Fields{
							"Name": Equal("client-url-etcd-server-tls"),
							"VolumeSource": MatchFields(IgnoreExtras, Fields{
								"Secret": PointTo(MatchFields(IgnoreExtras, Fields{
									"SecretName": Equal(values.ClientUrlTLS.ServerTLSSecretRef.Name),
								})),
							}),
						}),
						"client-url-etcd-client-tls": MatchFields(IgnoreExtras, Fields{
							"Name": Equal("client-url-etcd-client-tls"),
							"VolumeSource": MatchFields(IgnoreExtras, Fields{
								"Secret": PointTo(MatchFields(IgnoreExtras, Fields{
									"SecretName": Equal(values.ClientUrlTLS.ClientTLSSecretRef.Name),
								})),
							}),
						}),
						"client-url-ca-etcd": MatchFields(IgnoreExtras, Fields{
							"Name": Equal("client-url-ca-etcd"),
							"VolumeSource": MatchFields(IgnoreExtras, Fields{
								"Secret": PointTo(MatchFields(IgnoreExtras, Fields{
									"SecretName": Equal(values.ClientUrlTLS.TLSCASecretRef.Name),
								})),
							}),
						}),
						"peer-url-etcd-server-tls": MatchFields(IgnoreExtras, Fields{
							"Name": Equal("peer-url-etcd-server-tls"),
							"VolumeSource": MatchFields(IgnoreExtras, Fields{
								"Secret": PointTo(MatchFields(IgnoreExtras, Fields{
									"SecretName": Equal(values.PeerUrlTLS.ServerTLSSecretRef.Name),
								})),
							}),
						}),
						"peer-url-ca-etcd": MatchFields(IgnoreExtras, Fields{
							"Name": Equal("peer-url-ca-etcd"),
							"VolumeSource": MatchFields(IgnoreExtras, Fields{
								"Secret": PointTo(MatchFields(IgnoreExtras, Fields{
									"SecretName": Equal(values.PeerUrlTLS.TLSCASecretRef.Name),
								})),
							}),
						}),
					}),
				}),
			}),
			"VolumeClaimTemplates": MatchAllElements(pvcIterator, Elements{
				values.VolumeClaimTemplateName: MatchFields(IgnoreExtras, Fields{
					"ObjectMeta": MatchFields(IgnoreExtras, Fields{
						"Name": Equal(values.VolumeClaimTemplateName),
					}),
					"Spec": MatchFields(IgnoreExtras, Fields{
						"StorageClassName": PointTo(Equal(*values.StorageClass)),
						"AccessModes": MatchAllElements(accessModeIterator, Elements{
							"ReadWriteOnce": Equal(corev1.ReadWriteOnce),
						}),
						"Resources": MatchFields(IgnoreExtras, Fields{
							"Requests": MatchKeys(IgnoreExtras, Keys{
								corev1.ResourceStorage: Equal(*values.StorageCapacity),
							}),
						}),
					}),
				}),
			}),
		}),
	}))
}

func checkStsMetadata(ors []metav1.OwnerReference, values Values) {
	Expect(ors).To(ConsistOf(Equal(metav1.OwnerReference{
		APIVersion:         druidv1alpha1.GroupVersion.String(),
		Kind:               "Etcd",
		Name:               values.Name,
		UID:                values.EtcdUID,
		Controller:         pointer.BoolPtr(true),
		BlockOwnerDeletion: pointer.BoolPtr(true),
	})))
}

func getEtcd(name, namespace string, tlsEnabled bool, replicas int32) *druidv1alpha1.Etcd {
	instance := &druidv1alpha1.Etcd{
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
				"foo": "bar",
			},
			Replicas:            replicas,
			StorageCapacity:     &storageCapacity,
			StorageClass:        &storageClass,
			PriorityClassName:   &priorityClassName,
			VolumeClaimTemplate: &volumeClaimTemplateName,
			Backup: druidv1alpha1.BackupSpec{
				Image:                    &imageBR,
				Port:                     pointer.Int32Ptr(backupPort),
				Store:                    getEtcdWithABS(),
				FullSnapshotSchedule:     &snapshotSchedule,
				GarbageCollectionPolicy:  &garbageCollectionPolicy,
				GarbageCollectionPeriod:  &garbageCollectionPeriod,
				DeltaSnapshotPeriod:      &deltaSnapshotPeriod,
				DeltaSnapshotMemoryLimit: &deltaSnapShotMemLimit,
				EtcdSnapshotTimeout:      &etcdSnapshotTimeout,
				LeaderElection: &druidv1alpha1.LeaderElectionSpec{
					EtcdConnectionTimeout: &etcdConnectionTimeout,
				},

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
				OwnerCheck: &druidv1alpha1.OwnerCheckSpec{
					Name:        ownerName,
					ID:          ownerID,
					Interval:    &ownerCheckInterval,
					Timeout:     &ownerCheckTimeout,
					DNSCacheTTL: &ownerCheckDNSCacheTTL,
				},
			},
			Etcd: druidv1alpha1.EtcdConfig{
				Quota:                   &quota,
				Metrics:                 &metricsBasic,
				Image:                   &imageEtcd,
				DefragmentationSchedule: &defragSchedule,
				EtcdDefragTimeout:       &etcdDefragTimeout,
				HeartbeatDuration:       &heartbeatDuration,
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
				ClientPort: pointer.Int32Ptr(clientPort),
				ServerPort: pointer.Int32Ptr(serverPort),
			},
			Common: druidv1alpha1.SharedConfig{
				AutoCompactionMode:      &autoCompactionMode,
				AutoCompactionRetention: &autoCompactionRetention,
			},
		},
	}

	if tlsEnabled {
		clientTlsConfig := &druidv1alpha1.TLSConfig{
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

		peerTlsConfig := &druidv1alpha1.TLSConfig{
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

		instance.Spec.Etcd.ClientUrlTLS = clientTlsConfig
		instance.Spec.Etcd.PeerUrlTLS = peerTlsConfig
		instance.Spec.Backup.TLS = clientTlsConfig
	}
	return instance
}

func parseQuantity(q string) resource.Quantity {
	val, _ := resource.ParseQuantity(q)
	return val
}

func getEtcdWithABS() *druidv1alpha1.StoreSpec {
	return &druidv1alpha1.StoreSpec{
		Container: &container,
		Prefix:    prefix,
		Provider:  (*druidv1alpha1.StorageProvider)(pointer.StringPtr("azure")),
		SecretRef: &corev1.SecretReference{
			Name: "etcd-backup",
		},
	}
}

func volumeMountIterator(element interface{}) string {
	return (element.(corev1.VolumeMount)).Name
}

func volumeIterator(element interface{}) string {
	return (element.(corev1.Volume)).Name
}

func keyIterator(element interface{}) string {
	return (element.(corev1.KeyToPath)).Key
}

func envIterator(element interface{}) string {
	return (element.(corev1.EnvVar)).Name
}

func containerIterator(element interface{}) string {
	return (element.(corev1.Container)).Name
}

func hostAliasIterator(element interface{}) string {
	return (element.(corev1.HostAlias)).IP
}

func pvcIterator(element interface{}) string {
	return (element.(corev1.PersistentVolumeClaim)).Name
}

func accessModeIterator(element interface{}) string {
	return string(element.(corev1.PersistentVolumeAccessMode))
}

func cmdIterator(element interface{}) string {
	return element.(string)
}
