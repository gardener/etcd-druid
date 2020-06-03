// Copyright (c) 2018 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package controllers

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/gardener/etcd-druid/pkg/common"
	"github.com/gardener/gardener/pkg/utils/imagevector"
	"github.com/ghodss/yaml"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/pkg/utils"
	"github.com/gardener/gardener/pkg/utils/kubernetes/health"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type StatefulSetInitializer int

const (
	WithoutOwner        StatefulSetInitializer = 0
	WithOwnerReference  StatefulSetInitializer = 1
	WithOwnerAnnotation StatefulSetInitializer = 2
)

const (
	aws       = "aws"
	azure     = "azure"
	gcp       = "gcp"
	alicloud  = "alicloud"
	openstack = "openstack"
)

const (
	s3    = "S3"
	abs   = "ABS"
	gcs   = "GCS"
	oss   = "OSS"
	swift = "Swift"
	local = "Local"
)

const (
	timeout         = time.Minute * 2
	pollingInterval = time.Second * 2
	etcdConfig      = "etcd.conf.yaml"
	quotaKey        = "quota-backend-bytes"
	backupRestore   = "backup-restore"
	metricsKey      = "metrics"
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
	imageEtcd                     = "quay.io/coreos/etcd:v3.3.13"
	imageBR                       = "eu.gcr.io/gardener-project/gardener/etcdbrctl:0.8.0"
	snapshotSchedule              = "0 */24 * * *"
	defragSchedule                = "0 */24 * * *"
	container                     = "default.bkp"
	storageCapacity               = resource.MustParse("5Gi")
	defaultStorageCapacity        = resource.MustParse("16Gi")
	storageClass                  = "gardener.fast"
	priorityClassName             = "class_priority"
	deltaSnapShotMemLimit         = resource.MustParse("100Mi")
	quota                         = resource.MustParse("8Gi")
	provider                      = druidv1alpha1.StorageProvider("Local")
	prefix                        = "/tmp"
	volumeClaimTemplateName       = "etcd-main"
	garbageCollectionPolicy       = druidv1alpha1.GarbageCollectionPolicy(druidv1alpha1.GarbageCollectionPolicyExponential)
	metricsBasic                  = druidv1alpha1.Basic
	maxBackups                    = 7
	imageNames                    = []string{
		common.Etcd,
		common.BackupRestore,
	}
)

func ownerRefIterator(element interface{}) string {
	return (element.(metav1.OwnerReference)).Name
}

func servicePortIterator(element interface{}) string {
	return (element.(corev1.ServicePort)).Name
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
	return string(element.(string))
}

var _ = Describe("Druid", func() {
	//Reconciliation of new etcd resource deployment without any existing statefulsets.
	Context("when adding etcd resources", func() {
		var err error
		var instance *druidv1alpha1.Etcd
		var c client.Client

		BeforeEach(func() {
			instance = getEtcd("foo1", "default", false)
			c = mgr.GetClient()
			ns := corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: instance.Namespace,
				},
			}
			c.Create(context.TODO(), &ns)
			storeSecret := instance.Spec.Backup.Store.SecretRef.Name
			errors := createSecrets(c, instance.Namespace, storeSecret)
			Expect(len(errors)).Should(BeZero())
		})
		It("should create and adopt statefulset", func() {
			defer WithWd("..")()
			err = c.Create(context.TODO(), instance)
			Expect(err).NotTo(HaveOccurred())
			ss := appsv1.StatefulSet{}
			Eventually(func() error { return statefulsetIsCorrectlyReconciled(c, instance, &ss) }, timeout, pollingInterval).Should(BeNil())
			setStatefulSetReady(&ss)
			err = c.Status().Update(context.TODO(), &ss)
			Expect(err).NotTo(HaveOccurred())
		})
		AfterEach(func() {
			c.Delete(context.TODO(), instance)
		})
	})

	DescribeTable("when adding etcd resources with statefulset already present",
		func(name string, setupStatefulSet StatefulSetInitializer) {
			var ss *appsv1.StatefulSet
			instance := getEtcd(name, "default", false)
			c := mgr.GetClient()
			switch setupStatefulSet {
			case WithoutOwner:
				ss = createStatefulset(name, "default", instance.Spec.Labels)
				Expect(ss.OwnerReferences).Should(BeNil())
			case WithOwnerReference:
				ss = createStatefulsetWithOwnerReference(instance)
				Expect(len(ss.OwnerReferences)).ShouldNot(BeZero())
			case WithOwnerAnnotation:
				ss = createStatefulsetWithOwnerAnnotations(instance)
			default:
				Fail("StatefulSetInitializer invalid")
			}

			storeSecret := instance.Spec.Backup.Store.SecretRef.Name
			errors := createSecrets(c, instance.Namespace, storeSecret)
			Expect(len(errors)).Should(BeZero())
			c.Create(context.TODO(), ss)
			defer WithWd("..")()
			err := c.Create(context.TODO(), instance)
			Expect(err).NotTo(HaveOccurred())
			s := &appsv1.StatefulSet{}
			Eventually(func() error { return statefulsetIsCorrectlyReconciled(c, instance, s) }, timeout, pollingInterval).Should(BeNil())
			setStatefulSetReady(s)
			err = c.Status().Update(context.TODO(), s)
			Expect(err).NotTo(HaveOccurred())
			err = c.Delete(context.TODO(), instance)
			Expect(err).NotTo(HaveOccurred())
		},
		Entry("when statefulset not owned by etcd, druid should adopt the statefulset", "foo20", WithoutOwner),
		Entry("when statefulset owned by etcd with owner reference set, druid should remove ownerref and add annotations", "foo21", WithOwnerReference),
		Entry("when statefulset owned by etcd with owner annotations set, druid should persist the annotations", "foo22", WithOwnerAnnotation),
		Entry("when etcd has the spec changed, druid should reconcile statefulset", "foo3", WithoutOwner),
	)

	Context("when adding etcd resources with statefulset already present", func() {
		Context("when statefulset is in crashloopbackoff", func() {
			var err error
			var instance *druidv1alpha1.Etcd
			var c client.Client
			var p *corev1.Pod
			var ss *appsv1.StatefulSet
			BeforeEach(func() {
				instance = getEtcd("foo4", "default", false)
				Expect(err).NotTo(HaveOccurred())
				c = mgr.GetClient()
				p = createPod(fmt.Sprintf("%s-0", instance.Name), "default", instance.Spec.Labels)
				ss = createStatefulset(instance.Name, instance.Namespace, instance.Spec.Labels)
				err = c.Create(context.TODO(), p)
				Expect(err).NotTo(HaveOccurred())
				err = c.Create(context.TODO(), ss)
				Expect(err).NotTo(HaveOccurred())
				p.Status.ContainerStatuses = []corev1.ContainerStatus{
					{
						Name: "Container-0",
						State: corev1.ContainerState{
							Waiting: &corev1.ContainerStateWaiting{
								Reason:  "CrashLoopBackOff",
								Message: "Container is in CrashLoopBackOff.",
							},
						},
					},
				}
				err = c.Status().Update(context.TODO(), p)
				Expect(err).NotTo(HaveOccurred())
				storeSecret := instance.Spec.Backup.Store.SecretRef.Name
				errors := createSecrets(c, instance.Namespace, storeSecret)
				Expect(len(errors)).Should(BeZero())

			})
			It("should restart pod", func() {
				defer WithWd("..")()
				err = c.Create(context.TODO(), instance)
				Expect(err).NotTo(HaveOccurred())
				Eventually(func() error { return podDeleted(c, instance) }, timeout, pollingInterval).Should(BeNil())
			})
			AfterEach(func() {
				s := &appsv1.StatefulSet{}
				Eventually(func() error { return statefulsetIsCorrectlyReconciled(c, instance, s) }, timeout, pollingInterval).Should(BeNil())
				setStatefulSetReady(s)
				err = c.Status().Update(context.TODO(), s)
				Expect(err).NotTo(HaveOccurred())
				c.Delete(context.TODO(), instance)
			})
		})
	})

	DescribeTable("when deleting etcd resources",
		func(name string, setupStatefulSet StatefulSetInitializer) {
			var ss *appsv1.StatefulSet
			var sts appsv1.StatefulSet
			instance := getEtcd(name, "default", false)
			c := mgr.GetClient()
			switch setupStatefulSet {
			case WithoutOwner:
				ss = createStatefulset(name, "default", instance.Spec.Labels)
				Expect(ss.OwnerReferences).Should(BeNil())
			case WithOwnerReference:
				ss = createStatefulsetWithOwnerReference(instance)
				Expect(len(ss.OwnerReferences)).ShouldNot(BeZero())
			case WithOwnerAnnotation:
				ss = createStatefulsetWithOwnerAnnotations(instance)
			default:
				Fail("StatefulSetInitializer invalid")
			}
			stopCh := make(chan struct{})
			storeSecret := instance.Spec.Backup.Store.SecretRef.Name
			errors := createSecrets(c, instance.Namespace, storeSecret)
			Expect(len(errors)).Should(BeZero())
			c.Create(context.TODO(), ss)
			defer WithWd("..")()
			err := c.Create(context.TODO(), instance)
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() error { return statefulsetIsCorrectlyReconciled(c, instance, &sts) }, timeout, pollingInterval).Should(BeNil())
			// This go-routine is to set that statefulset is ready manually as statefulset controller is absent for tests.
			go func() {
				for {
					select {
					case <-time.After(time.Second * 2):
						setAndCheckStatefulSetReady(c, instance)
					case <-stopCh:
						return
					}
				}
			}()
			Eventually(func() error { return isEtcdReady(c, instance) }, timeout, pollingInterval).Should(BeNil())
			close(stopCh)
			err = c.Delete(context.TODO(), instance)
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() error { return statefulSetRemoved(c, &sts) }, timeout, pollingInterval).Should(BeNil())
			Eventually(func() error { return etcdRemoved(c, instance) }, timeout, pollingInterval).Should(BeNil())
		},
		Entry("when  statefulset with ownerReference and without owner annotations, druid should adopt and delete statefulset", "foo61", WithOwnerReference),
		Entry("when statefulset without ownerReference and without owner annotations, druid should adopt and delete statefulset", "foo62", WithoutOwner),
		Entry("when  statefulset without ownerReference and with owner annotations, druid should adopt and delete statefulset", "foo63", WithOwnerAnnotation),
	)

	DescribeTable("when etcd resource is created",
		func(name string, generateEtcd func(string, string) *druidv1alpha1.Etcd, validate func(*appsv1.StatefulSet, *corev1.ConfigMap, *corev1.Service, *druidv1alpha1.Etcd)) {
			var err error
			var instance *druidv1alpha1.Etcd
			var c client.Client
			var s *appsv1.StatefulSet
			var cm *corev1.ConfigMap
			var svc *corev1.Service

			defer WithWd("..")()
			instance = generateEtcd(name, "default")
			c = mgr.GetClient()
			ns := corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: instance.Namespace,
				},
			}
			c.Create(context.TODO(), &ns)
			if instance.Spec.Backup.Store != nil && instance.Spec.Backup.Store.SecretRef != nil {
				storeSecret := instance.Spec.Backup.Store.SecretRef.Name
				errors := createSecrets(c, instance.Namespace, storeSecret)
				Expect(len(errors)).Should(BeZero())
			}
			err = c.Create(context.TODO(), instance)
			Expect(err).NotTo(HaveOccurred())
			s = &appsv1.StatefulSet{}
			Eventually(func() error { return statefulsetIsCorrectlyReconciled(c, instance, s) }, timeout, pollingInterval).Should(BeNil())
			cm = &corev1.ConfigMap{}
			Eventually(func() error { return configMapIsCorrectlyReconciled(c, instance, cm) }, timeout, pollingInterval).Should(BeNil())
			svc = &corev1.Service{}
			Eventually(func() error { return serviceIsCorrectlyReconciled(c, instance, svc) }, timeout, pollingInterval).Should(BeNil())

			validate(s, cm, svc, instance)

			setStatefulSetReady(s)
			err = c.Status().Update(context.TODO(), s)
			Expect(err).NotTo(HaveOccurred())
		},
		Entry("if fields are not set in etcd.Spec, the statefulset should reflect the spec changes", "foo51", getEtcdWithDefault, validateEtcdWithDefaults),
		Entry("if fields are set in etcd.Spec and TLS enabled, the resources should reflect the spec changes", "foo52", getEtcdWithTLS, validateEtcd),
		Entry("if the store is GCS, the statefulset should reflect the spec changes", "foo53", getEtcdWithGCS, validateStoreGCP),
		Entry("if the store is S3, the statefulset should reflect the spec changes", "foo54", getEtcdWithS3, validateStoreAWS),
		Entry("if the store is ABS, the statefulset should reflect the spec changes", "foo55", getEtcdWithABS, validateStoreAzure),
		Entry("if the store is Swift, the statefulset should reflect the spec changes", "foo56", getEtcdWithSwift, validateStoreOpenstack),
		Entry("if the store is OSS, the statefulset should reflect the spec changes", "foo57", getEtcdWithOSS, validateStoreAlicloud),
	)
})

func podDeleted(c client.Client, etcd *druidv1alpha1.Etcd) error {
	ctx, cancel := context.WithTimeout(context.TODO(), timeout)
	defer cancel()
	pod := &corev1.Pod{}
	req := types.NamespacedName{
		Name:      fmt.Sprintf("%s-0", etcd.Name),
		Namespace: etcd.Namespace,
	}
	if err := c.Get(ctx, req, pod); err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers
			return nil
		}
		return err
	}
	return fmt.Errorf("pod not deleted")

}

func validateEtcdWithDefaults(s *appsv1.StatefulSet, cm *corev1.ConfigMap, svc *corev1.Service, instance *druidv1alpha1.Etcd) {
	// Validate Quota
	configYML := cm.Data[etcdConfig]
	config := map[string]string{}
	err := yaml.Unmarshal([]byte(configYML), &config)
	Expect(err).NotTo(HaveOccurred())
	Expect(instance.Spec.Etcd.Quota).To(BeNil())
	Expect(config).To(HaveKeyWithValue(quotaKey, fmt.Sprintf("%d", int64(quota.Value()))))

	// Validate Metrics MetricsLevel
	Expect(instance.Spec.Etcd.Metrics).To(BeNil())
	Expect(config).To(HaveKeyWithValue(metricsKey, fmt.Sprintf("%s", druidv1alpha1.Basic)))

	// Validate DefragmentationSchedule *string
	Expect(instance.Spec.Etcd.DefragmentationSchedule).To(BeNil())

	// Validate ServerPort and ClientPort
	Expect(instance.Spec.Etcd.ServerPort).To(BeNil())
	Expect(instance.Spec.Etcd.ClientPort).To(BeNil())

	Expect(instance.Spec.Etcd.Image).To(BeNil())
	imageVector, err := imagevector.ReadGlobalImageVectorWithEnvOverride(common.DefaultImageVector)
	Expect(err).NotTo(HaveOccurred())
	images, err := imagevector.FindImages(imageVector, imageNames)
	Expect(err).NotTo(HaveOccurred())

	// Validate Resources
	// resources:
	//	  limits:
	//		cpu: 100m
	//		memory: 512Gi
	//	  requests:
	//		cpu: 50m
	//		memory: 128Mi
	Expect(instance.Spec.Etcd.Resources).To(BeNil())

	// Validate TLS. Ensure that enableTLS flag is not triggered in the go-template
	Expect(instance.Spec.Etcd.TLS).To(BeNil())

	Expect(*cm).To(MatchFields(IgnoreExtras, Fields{
		"Data": MatchKeys(IgnoreExtras, Keys{
			"bootstrap.sh": Equal(fmt.Sprintf(bootstrapScriptWithoutTLS, instance.Name, backupPort)),
		}),
	}))

	Expect(config).To(MatchKeys(IgnoreExtras, Keys{
		"name":                      Equal(fmt.Sprintf("etcd-%s", instance.UID[:6])),
		"data-dir":                  Equal("/var/etcd/data/new.etcd"),
		"metrics":                   Equal(string(druidv1alpha1.Basic)),
		"snapshot-count":            Equal("75000"),
		"enable-v2":                 Equal("false"),
		"quota-backend-bytes":       Equal("8589934592"),
		"listen-client-urls":        Equal(fmt.Sprintf("http://0.0.0.0:%d", clientPort)),
		"advertise-client-urls":     Equal(fmt.Sprintf("http://0.0.0.0:%d", clientPort)),
		"initial-cluster-token":     Equal("initial"),
		"initial-cluster-state":     Equal("new"),
		"auto-compaction-mode":      Equal("periodic"),
		"auto-compaction-retention": Equal("24"),
	}))

	Expect(*svc).To(MatchFields(IgnoreExtras, Fields{
		"ObjectMeta": MatchFields(IgnoreExtras, Fields{
			"Name":      Equal(fmt.Sprintf("%s-client", instance.Name)),
			"Namespace": Equal(instance.Namespace),
			"Labels": MatchAllKeys(Keys{
				"name":     Equal("etcd"),
				"instance": Equal(instance.Name),
			}),
			"OwnerReferences": MatchElements(ownerRefIterator, IgnoreExtras, Elements{
				instance.Name: MatchFields(IgnoreExtras, Fields{
					"APIVersion":         Equal("druid.gardener.cloud/v1alpha1"),
					"Kind":               Equal("Etcd"),
					"Name":               Equal(instance.Name),
					"UID":                Equal(instance.UID),
					"Controller":         PointTo(Equal(true)),
					"BlockOwnerDeletion": PointTo(Equal(true)),
				}),
			}),
		}),
		"Spec": MatchFields(IgnoreExtras, Fields{
			"Type":            Equal(corev1.ServiceTypeClusterIP),
			"SessionAffinity": Equal(corev1.ServiceAffinityNone),
			"Selector": MatchKeys(IgnoreExtras, Keys{
				"instance": Equal(instance.Name),
				"name":     Equal("etcd"),
			}),
			"Ports": MatchElements(servicePortIterator, IgnoreExtras, Elements{
				"client": MatchFields(IgnoreExtras, Fields{
					"Name":     Equal("client"),
					"Protocol": Equal(corev1.ProtocolTCP),
					"Port":     Equal(clientPort),
					"TargetPort": MatchFields(IgnoreExtras, Fields{
						"IntVal": Equal(clientPort),
					}),
				}),
				"server": MatchFields(IgnoreExtras, Fields{
					"Name":     Equal("server"),
					"Protocol": Equal(corev1.ProtocolTCP),
					"Port":     Equal(serverPort),
					"TargetPort": MatchFields(IgnoreExtras, Fields{
						"IntVal": Equal(serverPort),
					}),
				}),
				"backuprestore": MatchFields(IgnoreExtras, Fields{
					"Name":     Equal("backuprestore"),
					"Protocol": Equal(corev1.ProtocolTCP),
					"Port":     Equal(backupPort),
					"TargetPort": MatchFields(IgnoreExtras, Fields{
						"IntVal": Equal(backupPort),
					}),
				}),
			}),
		}),
	}))

	Expect(*s).To(MatchFields(IgnoreExtras, Fields{
		"ObjectMeta": MatchFields(IgnoreExtras, Fields{
			"Name":      Equal(instance.Name),
			"Namespace": Equal(instance.Namespace),
			"Annotations": MatchAllKeys(Keys{
				"gardener.cloud/owned-by":   Equal(fmt.Sprintf("%s/%s", instance.Namespace, instance.Name)),
				"gardener.cloud/owner-type": Equal("etcd"),
				"app":                       Equal("etcd-statefulset"),
				"role":                      Equal("test"),
				"instance":                  Equal(instance.Name),
			}),
			"Labels": MatchAllKeys(Keys{
				"name":     Equal("etcd"),
				"instance": Equal(instance.Name),
			}),
		}),
		"Spec": MatchFields(IgnoreExtras, Fields{
			"UpdateStrategy": MatchFields(IgnoreExtras, Fields{
				"Type": Equal(appsv1.RollingUpdateStatefulSetStrategyType),
			}),
			"ServiceName": Equal(fmt.Sprintf("%s-client", instance.Name)),
			"Replicas":    PointTo(Equal(instance.Spec.Replicas)),
			"Selector": PointTo(MatchFields(IgnoreExtras, Fields{
				"MatchLabels": MatchAllKeys(Keys{
					"name":     Equal("etcd"),
					"instance": Equal(instance.Name),
				}),
			})),
			"Template": MatchFields(IgnoreExtras, Fields{
				"ObjectMeta": MatchFields(IgnoreExtras, Fields{
					"Annotations": MatchKeys(IgnoreExtras, Keys{
						"app":      Equal("etcd-statefulset"),
						"role":     Equal("test"),
						"instance": Equal(instance.Name),
					}),
					"Labels": MatchAllKeys(Keys{
						"name":     Equal("etcd"),
						"instance": Equal(instance.Name),
					}),
				}),
				"Spec": MatchFields(IgnoreExtras, Fields{
					"HostAliases": MatchAllElements(hostAliasIterator, Elements{
						"127.0.0.1": MatchFields(IgnoreExtras, Fields{
							"IP": Equal("127.0.0.1"),
							"Hostnames": MatchAllElements(cmdIterator, Elements{
								fmt.Sprintf("%s-local", instance.Name): Equal(fmt.Sprintf("%s-local", instance.Name)),
							}),
						}),
					}),
					"PriorityClassName": Equal(""),
					"Containers": MatchAllElements(containerIterator, Elements{
						common.Etcd: MatchFields(IgnoreExtras, Fields{
							"Ports": ConsistOf([]corev1.ContainerPort{
								corev1.ContainerPort{
									Name:          "server",
									Protocol:      corev1.ProtocolTCP,
									HostPort:      0,
									ContainerPort: serverPort,
								},
								corev1.ContainerPort{
									Name:          "client",
									Protocol:      corev1.ProtocolTCP,
									HostPort:      0,
									ContainerPort: clientPort,
								},
							}),
							"Command": MatchAllElements(cmdIterator, Elements{
								"/var/etcd/bin/bootstrap.sh": Equal("/var/etcd/bin/bootstrap.sh"),
							}),
							"ImagePullPolicy": Equal(corev1.PullIfNotPresent),
							"Image":           Equal(fmt.Sprintf("%s:%s", images[common.Etcd].Repository, *images[common.Etcd].Tag)),
							"Resources": MatchFields(IgnoreExtras, Fields{
								"Requests": MatchKeys(IgnoreExtras, Keys{
									corev1.ResourceCPU:    Equal(resource.MustParse("50m")),
									corev1.ResourceMemory: Equal(resource.MustParse("128Mi")),
								}),
								"Limits": MatchKeys(IgnoreExtras, Keys{
									corev1.ResourceCPU:    Equal(resource.MustParse("100m")),
									corev1.ResourceMemory: Equal(resource.MustParse("512Gi")),
								}),
							}),
							"ReadinessProbe": PointTo(MatchFields(IgnoreExtras, Fields{
								"Handler": MatchFields(IgnoreExtras, Fields{
									"HTTPGet": PointTo(MatchFields(IgnoreExtras, Fields{
										"Path": Equal("/healthz"),
										"Port": MatchFields(IgnoreExtras, Fields{
											"IntVal": Equal(int32(8080)),
										}),
										"Scheme": Equal(corev1.URISchemeHTTP),
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
											fmt.Sprintf("--endpoints=http://%s-local:%d", instance.Name, clientPort): Equal(fmt.Sprintf("--endpoints=http://%s-local:%d", instance.Name, clientPort)),
											"get": Equal("get"),
											"foo": Equal("foo"),
										}),
									})),
								}),
								"InitialDelaySeconds": Equal(int32(15)),
								"PeriodSeconds":       Equal(int32(5)),
							})),
							"VolumeMounts": MatchAllElements(volumeMountIterator, Elements{
								instance.Name: MatchFields(IgnoreExtras, Fields{
									"Name":      Equal(instance.Name),
									"MountPath": Equal("/var/etcd/data/"),
								}),
								"etcd-bootstrap-sh": MatchFields(IgnoreExtras, Fields{
									"Name":      Equal("etcd-bootstrap-sh"),
									"MountPath": Equal("/var/etcd/bin/"),
								}),
								"etcd-config-file": MatchFields(IgnoreExtras, Fields{
									"Name":      Equal("etcd-config-file"),
									"MountPath": Equal("/var/etcd/config/"),
								}),
							}),
						}),

						backupRestore: MatchFields(IgnoreExtras, Fields{
							"Command": MatchAllElements(cmdIterator, Elements{
								"etcdbrctl":                                      Equal("etcdbrctl"),
								"server":                                         Equal("server"),
								"--data-dir=/var/etcd/data/new.etcd":             Equal("--data-dir=/var/etcd/data/new.etcd"),
								"--insecure-transport=true":                      Equal("--insecure-transport=true"),
								"--insecure-skip-tls-verify=true":                Equal("--insecure-skip-tls-verify=true"),
								"--etcd-connection-timeout=5m":                   Equal("--etcd-connection-timeout=5m"),
								"--snapstore-temp-directory=/var/etcd/data/temp": Equal("--snapstore-temp-directory=/var/etcd/data/temp"),
								fmt.Sprintf("--delta-snapshot-memory-limit=%d", deltaSnapShotMemLimit.Value()):                 Equal(fmt.Sprintf("--delta-snapshot-memory-limit=%d", deltaSnapShotMemLimit.Value())),
								fmt.Sprintf("--garbage-collection-policy=%s", druidv1alpha1.GarbageCollectionPolicyLimitBased): Equal(fmt.Sprintf("--garbage-collection-policy=%s", druidv1alpha1.GarbageCollectionPolicyLimitBased)),
								fmt.Sprintf("--endpoints=http://%s-local:%d", instance.Name, clientPort):                       Equal(fmt.Sprintf("--endpoints=http://%s-local:%d", instance.Name, clientPort)),
								fmt.Sprintf("--embedded-etcd-quota-bytes=%d", int64(quota.Value())):                            Equal(fmt.Sprintf("--embedded-etcd-quota-bytes=%d", int64(quota.Value()))),
								fmt.Sprintf("--max-backups=%d", maxBackups):                                                    Equal(fmt.Sprintf("--max-backups=%d", maxBackups)),
							}),
							"Ports": ConsistOf([]corev1.ContainerPort{
								corev1.ContainerPort{
									Name:          "server",
									Protocol:      corev1.ProtocolTCP,
									HostPort:      0,
									ContainerPort: backupPort,
								},
							}),
							"Image":           Equal(fmt.Sprintf("%s:%s", images[common.BackupRestore].Repository, *images[common.BackupRestore].Tag)),
							"ImagePullPolicy": Equal(corev1.PullIfNotPresent),
							"VolumeMounts": MatchAllElements(volumeMountIterator, Elements{
								instance.Name: MatchFields(IgnoreExtras, Fields{
									"Name":      Equal(instance.Name),
									"MountPath": Equal("/var/etcd/data"),
								}),
								"etcd-config-file": MatchFields(IgnoreExtras, Fields{
									"Name":      Equal("etcd-config-file"),
									"MountPath": Equal("/var/etcd/config/"),
								}),
							}),
							"Env": MatchAllElements(envIterator, Elements{
								"STORAGE_CONTAINER": MatchFields(IgnoreExtras, Fields{
									"Name":  Equal("STORAGE_CONTAINER"),
									"Value": Equal(""),
								}),
							}),
						}),
					}),
					"Volumes": MatchAllElements(volumeIterator, Elements{
						"etcd-bootstrap-sh": MatchFields(IgnoreExtras, Fields{
							"Name": Equal("etcd-bootstrap-sh"),
							"VolumeSource": MatchFields(IgnoreExtras, Fields{
								"ConfigMap": PointTo(MatchFields(IgnoreExtras, Fields{
									"LocalObjectReference": MatchFields(IgnoreExtras, Fields{
										"Name": Equal(fmt.Sprintf("etcd-bootstrap-%s", string(instance.UID[:6]))),
									}),
									"DefaultMode": PointTo(Equal(int32(356))),
									"Items": MatchAllElements(keyIterator, Elements{
										"bootstrap.sh": MatchFields(IgnoreExtras, Fields{
											"Key":  Equal("bootstrap.sh"),
											"Path": Equal("bootstrap.sh"),
										}),
									}),
								})),
							}),
						}),
						"etcd-config-file": MatchFields(IgnoreExtras, Fields{
							"Name": Equal("etcd-config-file"),
							"VolumeSource": MatchFields(IgnoreExtras, Fields{
								"ConfigMap": PointTo(MatchFields(IgnoreExtras, Fields{
									"LocalObjectReference": MatchFields(IgnoreExtras, Fields{
										"Name": Equal(fmt.Sprintf("etcd-bootstrap-%s", string(instance.UID[:6]))),
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
					}),
				}),
			}),
			"VolumeClaimTemplates": MatchAllElements(pvcIterator, Elements{
				instance.Name: MatchFields(IgnoreExtras, Fields{
					"ObjectMeta": MatchFields(IgnoreExtras, Fields{
						"Name": Equal(instance.Name),
					}),
					"Spec": MatchFields(IgnoreExtras, Fields{
						"AccessModes": MatchAllElements(accessModeIterator, Elements{
							"ReadWriteOnce": Equal(corev1.ReadWriteOnce),
						}),
						"Resources": MatchFields(IgnoreExtras, Fields{
							"Requests": MatchKeys(IgnoreExtras, Keys{
								corev1.ResourceStorage: Equal(defaultStorageCapacity),
							}),
						}),
					}),
				}),
			}),
		}),
	}))

}

func validateEtcd(s *appsv1.StatefulSet, cm *corev1.ConfigMap, svc *corev1.Service, instance *druidv1alpha1.Etcd) {

	// Validate Quota
	configYML := cm.Data[etcdConfig]
	config := map[string]interface{}{}
	err := yaml.Unmarshal([]byte(configYML), &config)
	Expect(err).NotTo(HaveOccurred())
	Expect(instance.Spec.Etcd.Quota).NotTo(BeNil())
	Expect(config).To(HaveKeyWithValue(quotaKey, float64(instance.Spec.Etcd.Quota.Value())))

	// Validate Metrics MetricsLevel
	Expect(instance.Spec.Etcd.Metrics).NotTo(BeNil())
	Expect(config).To(HaveKeyWithValue(metricsKey, fmt.Sprintf("%s", *instance.Spec.Etcd.Metrics)))

	// Validate DefragmentationSchedule *string
	Expect(instance.Spec.Etcd.DefragmentationSchedule).NotTo(BeNil())

	// Validate Image
	Expect(instance.Spec.Etcd.Image).NotTo(BeNil())

	// Validate Resources
	Expect(instance.Spec.Etcd.Resources).NotTo(BeNil())

	store, err := utils.StorageProviderFromInfraProvider(instance.Spec.Backup.Store.Provider)
	Expect(err).NotTo(HaveOccurred())

	Expect(*cm).To(MatchFields(IgnoreExtras, Fields{
		"ObjectMeta": MatchFields(IgnoreExtras, Fields{
			"Name":      Equal(fmt.Sprintf("etcd-bootstrap-%s", string(instance.UID[:6]))),
			"Namespace": Equal(instance.Namespace),
			"Labels": MatchAllKeys(Keys{
				"name":     Equal("etcd"),
				"instance": Equal(instance.Name),
			}),
			"OwnerReferences": MatchElements(ownerRefIterator, IgnoreExtras, Elements{
				instance.Name: MatchFields(IgnoreExtras, Fields{
					"APIVersion":         Equal("druid.gardener.cloud/v1alpha1"),
					"Kind":               Equal("Etcd"),
					"Name":               Equal(instance.Name),
					"UID":                Equal(instance.UID),
					"Controller":         PointTo(Equal(true)),
					"BlockOwnerDeletion": PointTo(Equal(true)),
				}),
			}),
		}),
		"Data": MatchKeys(IgnoreExtras, Keys{
			"bootstrap.sh": Equal(fmt.Sprintf(bootstrapScriptWithTLS, instance.Name, backupPort)),
		}),
	}))

	Expect(config).To(MatchKeys(IgnoreExtras, Keys{
		"name":                      Equal(fmt.Sprintf("etcd-%s", instance.UID[:6])),
		"data-dir":                  Equal("/var/etcd/data/new.etcd"),
		"metrics":                   Equal(fmt.Sprintf("%s", *instance.Spec.Etcd.Metrics)),
		"snapshot-count":            Equal(float64(75000)),
		"enable-v2":                 Equal(false),
		"quota-backend-bytes":       Equal(float64(instance.Spec.Etcd.Quota.Value())),
		"listen-client-urls":        Equal(fmt.Sprintf("https://0.0.0.0:%d", *instance.Spec.Etcd.ClientPort)),
		"advertise-client-urls":     Equal(fmt.Sprintf("https://0.0.0.0:%d", *instance.Spec.Etcd.ClientPort)),
		"initial-cluster-token":     Equal("initial"),
		"initial-cluster-state":     Equal("new"),
		"auto-compaction-mode":      Equal("periodic"),
		"auto-compaction-retention": Equal("24"),

		"client-transport-security": MatchKeys(IgnoreExtras, Keys{
			"cert-file":        Equal("/var/etcd/ssl/server/tls.crt"),
			"key-file":         Equal("/var/etcd/ssl/server/tls.key"),
			"client-cert-auth": Equal(true),
			"trusted-ca-file":  Equal("/var/etcd/ssl/ca/ca.crt"),
			"auto-tls":         Equal(false),
		}),
	}))

	Expect(*svc).To(MatchFields(IgnoreExtras, Fields{
		"ObjectMeta": MatchFields(IgnoreExtras, Fields{
			"Name":      Equal(fmt.Sprintf("%s-client", instance.Name)),
			"Namespace": Equal(instance.Namespace),
			"Labels": MatchAllKeys(Keys{
				"name":     Equal("etcd"),
				"instance": Equal(instance.Name),
			}),
			"OwnerReferences": MatchElements(ownerRefIterator, IgnoreExtras, Elements{
				instance.Name: MatchFields(IgnoreExtras, Fields{
					"APIVersion":         Equal("druid.gardener.cloud/v1alpha1"),
					"Kind":               Equal("Etcd"),
					"Name":               Equal(instance.Name),
					"UID":                Equal(instance.UID),
					"Controller":         PointTo(Equal(true)),
					"BlockOwnerDeletion": PointTo(Equal(true)),
				}),
			}),
		}),
		"Spec": MatchFields(IgnoreExtras, Fields{
			"Type":            Equal(corev1.ServiceTypeClusterIP),
			"SessionAffinity": Equal(corev1.ServiceAffinityNone),
			"Selector": MatchKeys(IgnoreExtras, Keys{
				"instance": Equal(instance.Name),
				"name":     Equal("etcd"),
			}),
			"Ports": MatchElements(servicePortIterator, IgnoreExtras, Elements{
				"client": MatchFields(IgnoreExtras, Fields{
					"Name":     Equal("client"),
					"Protocol": Equal(corev1.ProtocolTCP),
					"Port":     Equal(*instance.Spec.Etcd.ClientPort),
					"TargetPort": MatchFields(IgnoreExtras, Fields{
						"IntVal": Equal(*instance.Spec.Etcd.ClientPort),
					}),
				}),
				"server": MatchFields(IgnoreExtras, Fields{
					"Name":     Equal("server"),
					"Protocol": Equal(corev1.ProtocolTCP),
					"Port":     Equal(*instance.Spec.Etcd.ServerPort),
					"TargetPort": MatchFields(IgnoreExtras, Fields{
						"IntVal": Equal(*instance.Spec.Etcd.ServerPort),
					}),
				}),
				"backuprestore": MatchFields(IgnoreExtras, Fields{
					"Name":     Equal("backuprestore"),
					"Protocol": Equal(corev1.ProtocolTCP),
					"Port":     Equal(*instance.Spec.Backup.Port),
					"TargetPort": MatchFields(IgnoreExtras, Fields{
						"IntVal": Equal(*instance.Spec.Backup.Port),
					}),
				}),
			}),
		}),
	}))

	Expect(*s).To(MatchFields(IgnoreExtras, Fields{
		"ObjectMeta": MatchFields(IgnoreExtras, Fields{
			"Name":      Equal(instance.Name),
			"Namespace": Equal(instance.Namespace),
			"Annotations": MatchAllKeys(Keys{
				"gardener.cloud/owned-by":   Equal(fmt.Sprintf("%s/%s", instance.Namespace, instance.Name)),
				"gardener.cloud/owner-type": Equal("etcd"),
				"app":                       Equal("etcd-statefulset"),
				"role":                      Equal("test"),
				"instance":                  Equal(instance.Name),
			}),
			"Labels": MatchAllKeys(Keys{
				"name":     Equal("etcd"),
				"instance": Equal(instance.Name),
			}),
		}),

		"Spec": MatchFields(IgnoreExtras, Fields{
			"UpdateStrategy": MatchFields(IgnoreExtras, Fields{
				"Type": Equal(appsv1.RollingUpdateStatefulSetStrategyType),
			}),
			"ServiceName": Equal(fmt.Sprintf("%s-client", instance.Name)),
			"Replicas":    PointTo(Equal(instance.Spec.Replicas)),
			"Selector": PointTo(MatchFields(IgnoreExtras, Fields{
				"MatchLabels": MatchAllKeys(Keys{
					"name":     Equal("etcd"),
					"instance": Equal(instance.Name),
				}),
			})),
			"Template": MatchFields(IgnoreExtras, Fields{
				"ObjectMeta": MatchFields(IgnoreExtras, Fields{
					"Annotations": MatchKeys(IgnoreExtras, Keys{
						"app":      Equal("etcd-statefulset"),
						"role":     Equal("test"),
						"instance": Equal(instance.Name),
					}),
					"Labels": MatchAllKeys(Keys{
						"name":     Equal("etcd"),
						"instance": Equal(instance.Name),
					}),
				}),
				//s.Spec.Template.Spec.HostAliases
				"Spec": MatchFields(IgnoreExtras, Fields{
					"HostAliases": MatchAllElements(hostAliasIterator, Elements{
						"127.0.0.1": MatchFields(IgnoreExtras, Fields{
							"IP": Equal("127.0.0.1"),
							"Hostnames": MatchAllElements(cmdIterator, Elements{
								fmt.Sprintf("%s-local", instance.Name): Equal(fmt.Sprintf("%s-local", instance.Name)),
							}),
						}),
					}),
					"PriorityClassName": Equal(*instance.Spec.PriorityClassName),
					"Containers": MatchAllElements(containerIterator, Elements{
						common.Etcd: MatchFields(IgnoreExtras, Fields{
							"Ports": ConsistOf([]corev1.ContainerPort{
								corev1.ContainerPort{
									Name:          "server",
									Protocol:      corev1.ProtocolTCP,
									HostPort:      0,
									ContainerPort: *instance.Spec.Etcd.ServerPort,
								},
								corev1.ContainerPort{
									Name:          "client",
									Protocol:      corev1.ProtocolTCP,
									HostPort:      0,
									ContainerPort: *instance.Spec.Etcd.ClientPort,
								},
							}),
							"Command": MatchAllElements(cmdIterator, Elements{
								"/var/etcd/bin/bootstrap.sh": Equal("/var/etcd/bin/bootstrap.sh"),
							}),
							"ImagePullPolicy": Equal(corev1.PullIfNotPresent),
							"Image":           Equal(*instance.Spec.Etcd.Image),
							"Resources": MatchFields(IgnoreExtras, Fields{
								"Requests": MatchKeys(IgnoreExtras, Keys{
									corev1.ResourceCPU:    Equal(instance.Spec.Etcd.Resources.Requests[corev1.ResourceCPU]),
									corev1.ResourceMemory: Equal(instance.Spec.Etcd.Resources.Requests[corev1.ResourceMemory]),
								}),
								"Limits": MatchKeys(IgnoreExtras, Keys{
									corev1.ResourceCPU:    Equal(instance.Spec.Etcd.Resources.Limits[corev1.ResourceCPU]),
									corev1.ResourceMemory: Equal(instance.Spec.Etcd.Resources.Limits[corev1.ResourceMemory]),
								}),
							}),
							"ReadinessProbe": PointTo(MatchFields(IgnoreExtras, Fields{
								"Handler": MatchFields(IgnoreExtras, Fields{
									"HTTPGet": PointTo(MatchFields(IgnoreExtras, Fields{
										"Path": Equal("/healthz"),
										"Port": MatchFields(IgnoreExtras, Fields{
											"IntVal": Equal(int32(8080)),
										}),
										"Scheme": Equal(corev1.URISchemeHTTPS),
									})),
								}),
								"InitialDelaySeconds": Equal(int32(15)),
								"PeriodSeconds":       Equal(int32(5)),
							})),
							"LivenessProbe": PointTo(MatchFields(IgnoreExtras, Fields{
								"Handler": MatchFields(IgnoreExtras, Fields{
									"Exec": PointTo(MatchFields(IgnoreExtras, Fields{
										"Command": MatchAllElements(cmdIterator, Elements{
											"/bin/sh":                             Equal("/bin/sh"),
											"-ec":                                 Equal("-ec"),
											"ETCDCTL_API=3":                       Equal("ETCDCTL_API=3"),
											"etcdctl":                             Equal("etcdctl"),
											"--cert=/var/etcd/ssl/client/tls.crt": Equal("--cert=/var/etcd/ssl/client/tls.crt"),
											"--key=/var/etcd/ssl/client/tls.key":  Equal("--key=/var/etcd/ssl/client/tls.key"),
											"--cacert=/var/etcd/ssl/ca/ca.crt":    Equal("--cacert=/var/etcd/ssl/ca/ca.crt"),
											fmt.Sprintf("--endpoints=https://%s-local:%d", instance.Name, clientPort): Equal(fmt.Sprintf("--endpoints=https://%s-local:%d", instance.Name, clientPort)),
											"get": Equal("get"),
											"foo": Equal("foo"),
										}),
									})),
								}),
								"InitialDelaySeconds": Equal(int32(15)),
								"PeriodSeconds":       Equal(int32(5)),
							})),
							"VolumeMounts": MatchAllElements(volumeMountIterator, Elements{
								*instance.Spec.VolumeClaimTemplate: MatchFields(IgnoreExtras, Fields{
									"Name":      Equal(*instance.Spec.VolumeClaimTemplate),
									"MountPath": Equal("/var/etcd/data/"),
								}),
								"etcd-bootstrap-sh": MatchFields(IgnoreExtras, Fields{
									"Name":      Equal("etcd-bootstrap-sh"),
									"MountPath": Equal("/var/etcd/bin/"),
								}),
								"etcd-config-file": MatchFields(IgnoreExtras, Fields{
									"Name":      Equal("etcd-config-file"),
									"MountPath": Equal("/var/etcd/config/"),
								}),
								"ca-etcd": MatchFields(IgnoreExtras, Fields{
									"Name":      Equal("ca-etcd"),
									"MountPath": Equal("/var/etcd/ssl/ca"),
								}),
								"etcd-server-tls": MatchFields(IgnoreExtras, Fields{
									"Name":      Equal("etcd-server-tls"),
									"MountPath": Equal("/var/etcd/ssl/server"),
								}),
								"etcd-client-tls": MatchFields(IgnoreExtras, Fields{
									"Name":      Equal("etcd-client-tls"),
									"MountPath": Equal("/var/etcd/ssl/client"),
								}),
							}),
						}),

						backupRestore: MatchFields(IgnoreExtras, Fields{
							"Command": MatchAllElements(cmdIterator, Elements{
								"etcdbrctl":                                      Equal("etcdbrctl"),
								"server":                                         Equal("server"),
								"--cert=/var/etcd/ssl/client/tls.crt":            Equal("--cert=/var/etcd/ssl/client/tls.crt"),
								"--key=/var/etcd/ssl/client/tls.key":             Equal("--key=/var/etcd/ssl/client/tls.key"),
								"--cacert=/var/etcd/ssl/ca/ca.crt":               Equal("--cacert=/var/etcd/ssl/ca/ca.crt"),
								"--server-cert=/var/etcd/ssl/server/tls.crt":     Equal("--server-cert=/var/etcd/ssl/server/tls.crt"),
								"--server-key=/var/etcd/ssl/server/tls.key":      Equal("--server-key=/var/etcd/ssl/server/tls.key"),
								"--data-dir=/var/etcd/data/new.etcd":             Equal("--data-dir=/var/etcd/data/new.etcd"),
								"--insecure-transport=false":                     Equal("--insecure-transport=false"),
								"--insecure-skip-tls-verify=false":               Equal("--insecure-skip-tls-verify=false"),
								"--snapstore-temp-directory=/var/etcd/data/temp": Equal("--snapstore-temp-directory=/var/etcd/data/temp"),
								"--etcd-connection-timeout=5m":                   Equal("--etcd-connection-timeout=5m"),
								fmt.Sprintf("--defragmentation-schedule=%s", *instance.Spec.Etcd.DefragmentationSchedule):                           Equal(fmt.Sprintf("--defragmentation-schedule=%s", *instance.Spec.Etcd.DefragmentationSchedule)),
								fmt.Sprintf("--schedule=%s", *instance.Spec.Backup.FullSnapshotSchedule):                                            Equal(fmt.Sprintf("--schedule=%s", *instance.Spec.Backup.FullSnapshotSchedule)),
								fmt.Sprintf("%s=%s", "--garbage-collection-policy", *instance.Spec.Backup.GarbageCollectionPolicy):                  Equal(fmt.Sprintf("%s=%s", "--garbage-collection-policy", *instance.Spec.Backup.GarbageCollectionPolicy)),
								fmt.Sprintf("%s=%s", "--storage-provider", store):                                                                   Equal(fmt.Sprintf("%s=%s", "--storage-provider", fmt.Sprintf("%s", store))),
								fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix):                                           Equal(fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix)),
								fmt.Sprintf("--delta-snapshot-memory-limit=%d", instance.Spec.Backup.DeltaSnapshotMemoryLimit.Value()):              Equal(fmt.Sprintf("--delta-snapshot-memory-limit=%d", instance.Spec.Backup.DeltaSnapshotMemoryLimit.Value())),
								fmt.Sprintf("--garbage-collection-policy=%s", *instance.Spec.Backup.GarbageCollectionPolicy):                        Equal(fmt.Sprintf("--garbage-collection-policy=%s", *instance.Spec.Backup.GarbageCollectionPolicy)),
								fmt.Sprintf("--endpoints=https://%s-local:%d", instance.Name, clientPort):                                           Equal(fmt.Sprintf("--endpoints=https://%s-local:%d", instance.Name, clientPort)),
								fmt.Sprintf("--embedded-etcd-quota-bytes=%d", int64(instance.Spec.Etcd.Quota.Value())):                              Equal(fmt.Sprintf("--embedded-etcd-quota-bytes=%d", int64(instance.Spec.Etcd.Quota.Value()))),
								fmt.Sprintf("%s=%s", "--delta-snapshot-period", instance.Spec.Backup.DeltaSnapshotPeriod.Duration.String()):         Equal(fmt.Sprintf("%s=%s", "--delta-snapshot-period", instance.Spec.Backup.DeltaSnapshotPeriod.Duration.String())),
								fmt.Sprintf("%s=%s", "--garbage-collection-period", instance.Spec.Backup.GarbageCollectionPeriod.Duration.String()): Equal(fmt.Sprintf("%s=%s", "--garbage-collection-period", instance.Spec.Backup.GarbageCollectionPeriod.Duration.String())),
							}),
							"Ports": ConsistOf([]corev1.ContainerPort{
								corev1.ContainerPort{
									Name:          "server",
									Protocol:      corev1.ProtocolTCP,
									HostPort:      0,
									ContainerPort: backupPort,
								},
							}),
							"Image":           Equal(fmt.Sprintf("%s", *instance.Spec.Backup.Image)),
							"ImagePullPolicy": Equal(corev1.PullIfNotPresent),
							"VolumeMounts": MatchElements(volumeMountIterator, IgnoreExtras, Elements{
								*instance.Spec.VolumeClaimTemplate: MatchFields(IgnoreExtras, Fields{
									"Name":      Equal(*instance.Spec.VolumeClaimTemplate),
									"MountPath": Equal("/var/etcd/data"),
								}),
								"etcd-config-file": MatchFields(IgnoreExtras, Fields{
									"Name":      Equal("etcd-config-file"),
									"MountPath": Equal("/var/etcd/config/"),
								}),
								"ca-etcd": MatchFields(IgnoreExtras, Fields{
									"Name":      Equal("ca-etcd"),
									"MountPath": Equal("/var/etcd/ssl/ca"),
								}),
								"etcd-server-tls": MatchFields(IgnoreExtras, Fields{
									"Name":      Equal("etcd-server-tls"),
									"MountPath": Equal("/var/etcd/ssl/server"),
								}),
								"etcd-client-tls": MatchFields(IgnoreExtras, Fields{
									"Name":      Equal("etcd-client-tls"),
									"MountPath": Equal("/var/etcd/ssl/client"),
								}),
							}),
							"Env": MatchElements(envIterator, IgnoreExtras, Elements{
								"STORAGE_CONTAINER": MatchFields(IgnoreExtras, Fields{
									"Name":  Equal("STORAGE_CONTAINER"),
									"Value": Equal(*instance.Spec.Backup.Store.Container),
								}),
							}),
						}),
					}),
					"Volumes": MatchAllElements(volumeIterator, Elements{
						"etcd-bootstrap-sh": MatchFields(IgnoreExtras, Fields{
							"Name": Equal("etcd-bootstrap-sh"),
							"VolumeSource": MatchFields(IgnoreExtras, Fields{
								"ConfigMap": PointTo(MatchFields(IgnoreExtras, Fields{
									"LocalObjectReference": MatchFields(IgnoreExtras, Fields{
										"Name": Equal(fmt.Sprintf("etcd-bootstrap-%s", string(instance.UID[:6]))),
									}),
									"DefaultMode": PointTo(Equal(int32(356))),
									"Items": MatchAllElements(keyIterator, Elements{
										"bootstrap.sh": MatchFields(IgnoreExtras, Fields{
											"Key":  Equal("bootstrap.sh"),
											"Path": Equal("bootstrap.sh"),
										}),
									}),
								})),
							}),
						}),
						"etcd-config-file": MatchFields(IgnoreExtras, Fields{
							"Name": Equal("etcd-config-file"),
							"VolumeSource": MatchFields(IgnoreExtras, Fields{
								"ConfigMap": PointTo(MatchFields(IgnoreExtras, Fields{
									"LocalObjectReference": MatchFields(IgnoreExtras, Fields{
										"Name": Equal(fmt.Sprintf("etcd-bootstrap-%s", string(instance.UID[:6]))),
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
						"etcd-server-tls": MatchFields(IgnoreExtras, Fields{
							"Name": Equal("etcd-server-tls"),
							"VolumeSource": MatchFields(IgnoreExtras, Fields{
								"Secret": PointTo(MatchFields(IgnoreExtras, Fields{
									"SecretName": Equal(instance.Spec.Etcd.TLS.ServerTLSSecretRef.Name),
								})),
							}),
						}),
						"etcd-client-tls": MatchFields(IgnoreExtras, Fields{
							"Name": Equal("etcd-client-tls"),
							"VolumeSource": MatchFields(IgnoreExtras, Fields{
								"Secret": PointTo(MatchFields(IgnoreExtras, Fields{
									"SecretName": Equal(instance.Spec.Etcd.TLS.ClientTLSSecretRef.Name),
								})),
							}),
						}),
						"ca-etcd": MatchFields(IgnoreExtras, Fields{
							"Name": Equal("ca-etcd"),
							"VolumeSource": MatchFields(IgnoreExtras, Fields{
								"Secret": PointTo(MatchFields(IgnoreExtras, Fields{
									"SecretName": Equal(instance.Spec.Etcd.TLS.TLSCASecretRef.Name),
								})),
							}),
						}),
					}),
				}),
			}),
			"VolumeClaimTemplates": MatchAllElements(pvcIterator, Elements{
				*instance.Spec.VolumeClaimTemplate: MatchFields(IgnoreExtras, Fields{
					"ObjectMeta": MatchFields(IgnoreExtras, Fields{
						"Name": Equal(*instance.Spec.VolumeClaimTemplate),
					}),
					"Spec": MatchFields(IgnoreExtras, Fields{
						"StorageClassName": PointTo(Equal(*instance.Spec.StorageClass)),
						"AccessModes": MatchAllElements(accessModeIterator, Elements{
							"ReadWriteOnce": Equal(corev1.ReadWriteOnce),
						}),
						"Resources": MatchFields(IgnoreExtras, Fields{
							"Requests": MatchKeys(IgnoreExtras, Keys{
								corev1.ResourceStorage: Equal(*instance.Spec.StorageCapacity),
							}),
						}),
					}),
				}),
			}),
		}),
	}))
}

func validateStoreGCP(s *appsv1.StatefulSet, cm *corev1.ConfigMap, svc *corev1.Service, instance *druidv1alpha1.Etcd) {

	Expect(*s).To(MatchFields(IgnoreExtras, Fields{
		"Spec": MatchFields(IgnoreExtras, Fields{
			"Template": MatchFields(IgnoreExtras, Fields{
				//s.Spec.Template.Spec.HostAliases
				"Spec": MatchFields(IgnoreExtras, Fields{
					"Containers": MatchElements(containerIterator, IgnoreExtras, Elements{
						backupRestore: MatchFields(IgnoreExtras, Fields{
							"Command": MatchElements(cmdIterator, IgnoreExtras, Elements{
								"--storage-provider=GCS": Equal("--storage-provider=GCS"),
								fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix): Equal(fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix)),
							}),
							"VolumeMounts": MatchElements(volumeMountIterator, IgnoreExtras, Elements{
								"etcd-backup": MatchFields(IgnoreExtras, Fields{
									"Name":      Equal("etcd-backup"),
									"MountPath": Equal("/root/.gcp/"),
								}),
							}),
							"Env": MatchAllElements(envIterator, Elements{
								"STORAGE_CONTAINER": MatchFields(IgnoreExtras, Fields{
									"Name":  Equal("STORAGE_CONTAINER"),
									"Value": Equal(*instance.Spec.Backup.Store.Container),
								}),
								"GOOGLE_APPLICATION_CREDENTIALS": MatchFields(IgnoreExtras, Fields{
									"Name":  Equal("GOOGLE_APPLICATION_CREDENTIALS"),
									"Value": Equal("/root/.gcp/serviceaccount.json"),
								}),
							}),
						}),
					}),
					"Volumes": MatchElements(volumeIterator, IgnoreExtras, Elements{
						"etcd-backup": MatchFields(IgnoreExtras, Fields{
							"Name": Equal("etcd-backup"),
							"VolumeSource": MatchFields(IgnoreExtras, Fields{
								"Secret": PointTo(MatchFields(IgnoreExtras, Fields{
									"SecretName": Equal(instance.Spec.Backup.Store.SecretRef.Name),
								})),
							}),
						}),
					}),
				}),
			}),
		}),
	}))
}

func validateStoreAzure(s *appsv1.StatefulSet, cm *corev1.ConfigMap, svc *corev1.Service, instance *druidv1alpha1.Etcd) {
	Expect(*s).To(MatchFields(IgnoreExtras, Fields{
		"Spec": MatchFields(IgnoreExtras, Fields{
			"Template": MatchFields(IgnoreExtras, Fields{
				//s.Spec.Template.Spec.HostAliases
				"Spec": MatchFields(IgnoreExtras, Fields{
					"Containers": MatchElements(containerIterator, IgnoreExtras, Elements{
						backupRestore: MatchFields(IgnoreExtras, Fields{
							"Command": MatchElements(cmdIterator, IgnoreExtras, Elements{
								"--storage-provider=ABS": Equal("--storage-provider=ABS"),
								fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix): Equal(fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix)),
							}),
							"Env": MatchAllElements(envIterator, Elements{
								"STORAGE_CONTAINER": MatchFields(IgnoreExtras, Fields{
									"Name":  Equal("STORAGE_CONTAINER"),
									"Value": Equal(*instance.Spec.Backup.Store.Container),
								}),
								"STORAGE_ACCOUNT": MatchFields(IgnoreExtras, Fields{
									"Name": Equal("STORAGE_ACCOUNT"),
									"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
										"SecretKeyRef": PointTo(MatchFields(IgnoreExtras, Fields{
											"LocalObjectReference": MatchFields(IgnoreExtras, Fields{
												"Name": Equal(instance.Spec.Backup.Store.SecretRef.Name),
											}),
											"Key": Equal("storageAccount"),
										})),
									})),
								}),
								"STORAGE_KEY": MatchFields(IgnoreExtras, Fields{
									"Name": Equal("STORAGE_KEY"),
									"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
										"SecretKeyRef": PointTo(MatchFields(IgnoreExtras, Fields{
											"LocalObjectReference": MatchFields(IgnoreExtras, Fields{
												"Name": Equal(instance.Spec.Backup.Store.SecretRef.Name),
											}),
											"Key": Equal("storageKey"),
										})),
									})),
								}),
							}),
						}),
					}),
				}),
			}),
		}),
	}))
}

func validateStoreOpenstack(s *appsv1.StatefulSet, cm *corev1.ConfigMap, svc *corev1.Service, instance *druidv1alpha1.Etcd) {
	Expect(*s).To(MatchFields(IgnoreExtras, Fields{
		"Spec": MatchFields(IgnoreExtras, Fields{
			"Template": MatchFields(IgnoreExtras, Fields{
				//s.Spec.Template.Spec.HostAliases
				"Spec": MatchFields(IgnoreExtras, Fields{
					"Containers": MatchElements(containerIterator, IgnoreExtras, Elements{
						backupRestore: MatchFields(IgnoreExtras, Fields{
							"Command": MatchElements(cmdIterator, IgnoreExtras, Elements{
								"--storage-provider=Swift": Equal("--storage-provider=Swift"),
								fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix): Equal(fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix)),
							}),
							"Env": MatchAllElements(envIterator, Elements{
								"STORAGE_CONTAINER": MatchFields(IgnoreExtras, Fields{
									"Name":  Equal("STORAGE_CONTAINER"),
									"Value": Equal(*instance.Spec.Backup.Store.Container),
								}),
								"OS_AUTH_URL": MatchFields(IgnoreExtras, Fields{
									"Name": Equal("OS_AUTH_URL"),
									"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
										"SecretKeyRef": PointTo(MatchFields(IgnoreExtras, Fields{
											"LocalObjectReference": MatchFields(IgnoreExtras, Fields{
												"Name": Equal(instance.Spec.Backup.Store.SecretRef.Name),
											}),
											"Key": Equal("authURL"),
										})),
									})),
								}),
								"OS_USERNAME": MatchFields(IgnoreExtras, Fields{
									"Name": Equal("OS_USERNAME"),
									"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
										"SecretKeyRef": PointTo(MatchFields(IgnoreExtras, Fields{
											"LocalObjectReference": MatchFields(IgnoreExtras, Fields{
												"Name": Equal(instance.Spec.Backup.Store.SecretRef.Name),
											}),
											"Key": Equal("username"),
										})),
									})),
								}),
								"OS_TENANT_NAME": MatchFields(IgnoreExtras, Fields{
									"Name": Equal("OS_TENANT_NAME"),
									"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
										"SecretKeyRef": PointTo(MatchFields(IgnoreExtras, Fields{
											"LocalObjectReference": MatchFields(IgnoreExtras, Fields{
												"Name": Equal(instance.Spec.Backup.Store.SecretRef.Name),
											}),
											"Key": Equal("tenantName"),
										})),
									})),
								}),
								"OS_PASSWORD": MatchFields(IgnoreExtras, Fields{
									"Name": Equal("OS_PASSWORD"),
									"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
										"SecretKeyRef": PointTo(MatchFields(IgnoreExtras, Fields{
											"LocalObjectReference": MatchFields(IgnoreExtras, Fields{
												"Name": Equal(instance.Spec.Backup.Store.SecretRef.Name),
											}),
											"Key": Equal("password"),
										})),
									})),
								}),
								"OS_DOMAIN_NAME": MatchFields(IgnoreExtras, Fields{
									"Name": Equal("OS_DOMAIN_NAME"),
									"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
										"SecretKeyRef": PointTo(MatchFields(IgnoreExtras, Fields{
											"LocalObjectReference": MatchFields(IgnoreExtras, Fields{
												"Name": Equal(instance.Spec.Backup.Store.SecretRef.Name),
											}),
											"Key": Equal("domainName"),
										})),
									})),
								}),
							}),
						}),
					}),
				}),
			}),
		}),
	}))
}

func validateStoreAlicloud(s *appsv1.StatefulSet, cm *corev1.ConfigMap, svc *corev1.Service, instance *druidv1alpha1.Etcd) {
	Expect(*s).To(MatchFields(IgnoreExtras, Fields{
		"Spec": MatchFields(IgnoreExtras, Fields{
			"Template": MatchFields(IgnoreExtras, Fields{
				//s.Spec.Template.Spec.HostAliases
				"Spec": MatchFields(IgnoreExtras, Fields{
					"Containers": MatchElements(containerIterator, IgnoreExtras, Elements{

						backupRestore: MatchFields(IgnoreExtras, Fields{
							"Command": MatchElements(cmdIterator, IgnoreExtras, Elements{
								"--storage-provider=OSS": Equal("--storage-provider=OSS"),
								fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix): Equal(fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix)),
							}),
							"ImagePullPolicy": Equal(corev1.PullIfNotPresent),
							"Env": MatchAllElements(envIterator, Elements{
								"STORAGE_CONTAINER": MatchFields(IgnoreExtras, Fields{
									"Name":  Equal("STORAGE_CONTAINER"),
									"Value": Equal(*instance.Spec.Backup.Store.Container),
								}),
								"ALICLOUD_ENDPOINT": MatchFields(IgnoreExtras, Fields{
									"Name": Equal("ALICLOUD_ENDPOINT"),
									"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
										"SecretKeyRef": PointTo(MatchFields(IgnoreExtras, Fields{
											"LocalObjectReference": MatchFields(IgnoreExtras, Fields{
												"Name": Equal(instance.Spec.Backup.Store.SecretRef.Name),
											}),
											"Key": Equal("storageEndpoint"),
										})),
									})),
								}),
								"ALICLOUD_ACCESS_KEY_SECRET": MatchFields(IgnoreExtras, Fields{
									"Name": Equal("ALICLOUD_ACCESS_KEY_SECRET"),
									"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
										"SecretKeyRef": PointTo(MatchFields(IgnoreExtras, Fields{
											"LocalObjectReference": MatchFields(IgnoreExtras, Fields{
												"Name": Equal(instance.Spec.Backup.Store.SecretRef.Name),
											}),
											"Key": Equal("accessKeySecret"),
										})),
									})),
								}),
								"ALICLOUD_ACCESS_KEY_ID": MatchFields(IgnoreExtras, Fields{
									"Name": Equal("ALICLOUD_ACCESS_KEY_ID"),
									"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
										"SecretKeyRef": PointTo(MatchFields(IgnoreExtras, Fields{
											"LocalObjectReference": MatchFields(IgnoreExtras, Fields{
												"Name": Equal(instance.Spec.Backup.Store.SecretRef.Name),
											}),
											"Key": Equal("accessKeyID"),
										})),
									})),
								}),
							}),
						}),
					}),
				}),
			}),
		}),
	}))
}

func validateStoreAWS(s *appsv1.StatefulSet, cm *corev1.ConfigMap, svc *corev1.Service, instance *druidv1alpha1.Etcd) {
	Expect(*s).To(MatchFields(IgnoreExtras, Fields{
		"Spec": MatchFields(IgnoreExtras, Fields{
			"Template": MatchFields(IgnoreExtras, Fields{
				//s.Spec.Template.Spec.HostAliases
				"Spec": MatchFields(IgnoreExtras, Fields{
					"Containers": MatchElements(containerIterator, IgnoreExtras, Elements{

						backupRestore: MatchFields(IgnoreExtras, Fields{
							"Command": MatchElements(cmdIterator, IgnoreExtras, Elements{
								"--storage-provider=S3": Equal("--storage-provider=S3"),
								fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix): Equal(fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix)),
							}),
							"ImagePullPolicy": Equal(corev1.PullIfNotPresent),
							"Env": MatchAllElements(envIterator, Elements{
								"STORAGE_CONTAINER": MatchFields(IgnoreExtras, Fields{
									"Name":  Equal("STORAGE_CONTAINER"),
									"Value": Equal(*instance.Spec.Backup.Store.Container),
								}),
								"AWS_REGION": MatchFields(IgnoreExtras, Fields{
									"Name": Equal("AWS_REGION"),
									"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
										"SecretKeyRef": PointTo(MatchFields(IgnoreExtras, Fields{
											"LocalObjectReference": MatchFields(IgnoreExtras, Fields{
												"Name": Equal(instance.Spec.Backup.Store.SecretRef.Name),
											}),
											"Key": Equal("region"),
										})),
									})),
								}),
								"AWS_SECRET_ACCESS_KEY": MatchFields(IgnoreExtras, Fields{
									"Name": Equal("AWS_SECRET_ACCESS_KEY"),
									"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
										"SecretKeyRef": PointTo(MatchFields(IgnoreExtras, Fields{
											"LocalObjectReference": MatchFields(IgnoreExtras, Fields{
												"Name": Equal(instance.Spec.Backup.Store.SecretRef.Name),
											}),
											"Key": Equal("secretAccessKey"),
										})),
									})),
								}),
								"AWS_ACCESS_KEY_ID": MatchFields(IgnoreExtras, Fields{
									"Name": Equal("AWS_ACCESS_KEY_ID"),
									"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
										"SecretKeyRef": PointTo(MatchFields(IgnoreExtras, Fields{
											"LocalObjectReference": MatchFields(IgnoreExtras, Fields{
												"Name": Equal(instance.Spec.Backup.Store.SecretRef.Name),
											}),
											"Key": Equal("accessKeyID"),
										})),
									})),
								}),
							}),
						}),
					}),
				}),
			}),
		}),
	}))
}

func etcdRemoved(c client.Client, etcd *druidv1alpha1.Etcd) error {
	ctx, cancel := context.WithTimeout(context.TODO(), timeout)
	defer cancel()
	e := &druidv1alpha1.Etcd{}
	req := types.NamespacedName{
		Name:      etcd.Name,
		Namespace: etcd.Namespace,
	}
	if err := c.Get(ctx, req, e); err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers
			return nil
		}
		return err
	}
	return fmt.Errorf("etcd not deleted")
}

func isEtcdReady(c client.Client, etcd *druidv1alpha1.Etcd) error {
	ctx, cancel := context.WithTimeout(context.TODO(), timeout)
	defer cancel()
	e := &druidv1alpha1.Etcd{}
	req := types.NamespacedName{
		Name:      etcd.Name,
		Namespace: etcd.Namespace,
	}
	if err := c.Get(ctx, req, e); err != nil {
		return err
	}
	if e.Status.Ready == nil || !*e.Status.Ready {
		return fmt.Errorf("etcd not ready")
	}
	return nil
}

func setAndCheckStatefulSetReady(c client.Client, etcd *druidv1alpha1.Etcd) error {
	ctx, cancel := context.WithTimeout(context.TODO(), timeout)
	defer cancel()
	ss := &appsv1.StatefulSet{}
	req := types.NamespacedName{
		Name:      etcd.Name,
		Namespace: etcd.Namespace,
	}
	if err := c.Get(ctx, req, ss); err != nil {
		return err
	}
	setStatefulSetReady(ss)
	if err := c.Status().Update(context.TODO(), ss); err != nil {
		return err
	}
	if err := health.CheckStatefulSet(ss); err != nil {
		return err
	}
	return nil
}

func statefulSetRemoved(c client.Client, ss *appsv1.StatefulSet) error {
	ctx, cancel := context.WithTimeout(context.TODO(), timeout)
	defer cancel()
	sts := &appsv1.StatefulSet{}
	req := types.NamespacedName{
		Name:      ss.Name,
		Namespace: ss.Namespace,
	}
	if err := c.Get(ctx, req, sts); err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers
			return nil
		}
		return err
	}
	return fmt.Errorf("statefulset not removed")
}

func statefulsetIsCorrectlyReconciled(c client.Client, instance *druidv1alpha1.Etcd, ss *appsv1.StatefulSet) error {
	ctx, cancel := context.WithTimeout(context.TODO(), timeout)
	defer cancel()
	req := types.NamespacedName{
		Name:      fmt.Sprintf("%s", instance.Name),
		Namespace: instance.Namespace,
	}

	if err := c.Get(ctx, req, ss); err != nil {
		return err
	}
	if !checkEtcdAnnotations(ss.GetAnnotations(), instance) {
		return fmt.Errorf("no annotations")
	}
	if checkEtcdOwnerReference(ss.GetOwnerReferences(), instance) {
		return fmt.Errorf("ownerReference exists")
	}
	return nil
}

func configMapIsCorrectlyReconciled(c client.Client, instance *druidv1alpha1.Etcd, cm *corev1.ConfigMap) error {
	ctx, cancel := context.WithTimeout(context.TODO(), timeout)
	defer cancel()
	req := types.NamespacedName{
		Name:      fmt.Sprintf("etcd-bootstrap-%s", string(instance.UID[:6])),
		Namespace: instance.Namespace,
	}

	if err := c.Get(ctx, req, cm); err != nil {
		return err
	}

	if !checkEtcdOwnerReference(cm.GetOwnerReferences(), instance) {
		return fmt.Errorf("ownerReference does not exists")
	}
	return nil
}

func serviceIsCorrectlyReconciled(c client.Client, instance *druidv1alpha1.Etcd, svc *corev1.Service) error {
	ctx, cancel := context.WithTimeout(context.TODO(), timeout)
	defer cancel()
	req := types.NamespacedName{
		Name:      fmt.Sprintf("%s-client", instance.Name),
		Namespace: instance.Namespace,
	}

	if err := c.Get(ctx, req, svc); err != nil {
		return err
	}

	if !checkEtcdOwnerReference(svc.GetOwnerReferences(), instance) {
		return fmt.Errorf("ownerReference does not exists")
	}
	return nil
}

func createStatefulset(name, namespace string, labels map[string]string) *appsv1.StatefulSet {
	var replicas int32 = 0
	ss := appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-0", name),
					Namespace: namespace,
					Labels:    labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "etcd",
							Image: "quay.io/coreos/etcd:v3.3.13",
						},
						{
							Name:  "backup-restore",
							Image: "quay.io/coreos/etcd:v3.3.17",
						},
					},
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{},
			ServiceName:          "etcd-client",
			UpdateStrategy:       appsv1.StatefulSetUpdateStrategy{},
		},
	}
	return &ss
}

func createStatefulsetWithOwnerReference(etcd *druidv1alpha1.Etcd) *appsv1.StatefulSet {
	ss := createStatefulset(etcd.Name, etcd.Namespace, etcd.Spec.Labels)
	ss.SetOwnerReferences([]metav1.OwnerReference{
		{
			APIVersion:         etcd.TypeMeta.APIVersion,
			Kind:               etcd.TypeMeta.Kind,
			Name:               etcd.Name,
			UID:                etcd.UID,
			Controller:         pointer.BoolPtr(true),
			BlockOwnerDeletion: pointer.BoolPtr(true),
		},
	})
	return ss
}

func createStatefulsetWithOwnerAnnotations(etcd *druidv1alpha1.Etcd) *appsv1.StatefulSet {
	ss := createStatefulset(etcd.Name, etcd.Namespace, etcd.Spec.Labels)
	ss.SetAnnotations(map[string]string{
		"gardener.cloud/owned-by":   fmt.Sprintf("%s/%s", etcd.Namespace, etcd.Name),
		"gardener.cloud/owner-type": "etcd",
	})
	return ss
}

func createPod(name, namespace string, labels map[string]string) *corev1.Pod {
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "etcd",
					Image: "quay.io/coreos/etcd:v3.3.17",
				},
				{
					Name:  "backup-restore",
					Image: "quay.io/coreos/etcd:v3.3.17",
				},
			},
		},
	}
	return &pod
}

func getEtcdWithGCS(name, namespace string) *druidv1alpha1.Etcd {
	provider := druidv1alpha1.StorageProvider("gcp")
	etcd := getEtcdWithTLS(name, namespace)
	etcd.Spec.Backup.Store = &druidv1alpha1.StoreSpec{
		Container: &container,
		Prefix:    name,
		Provider:  &provider,
		SecretRef: &corev1.SecretReference{
			Name: "etcd-backup",
		},
	}
	return etcd
}

func getEtcdWithABS(name, namespace string) *druidv1alpha1.Etcd {
	provider := druidv1alpha1.StorageProvider("azure")
	etcd := getEtcdWithTLS(name, namespace)
	etcd.Spec.Backup.Store = &druidv1alpha1.StoreSpec{
		Container: &container,
		Prefix:    name,
		Provider:  &provider,
		SecretRef: &corev1.SecretReference{
			Name: "etcd-backup",
		},
	}
	return etcd
}

func getEtcdWithS3(name, namespace string) *druidv1alpha1.Etcd {
	provider := druidv1alpha1.StorageProvider("aws")
	etcd := getEtcdWithTLS(name, namespace)
	etcd.Spec.Backup.Store = &druidv1alpha1.StoreSpec{
		Container: &container,
		Prefix:    name,
		Provider:  &provider,
		SecretRef: &corev1.SecretReference{
			Name: "etcd-backup",
		},
	}
	return etcd
}

func getEtcdWithSwift(name, namespace string) *druidv1alpha1.Etcd {
	provider := druidv1alpha1.StorageProvider("openstack")
	etcd := getEtcdWithTLS(name, namespace)
	etcd.Spec.Backup.Store = &druidv1alpha1.StoreSpec{
		Container: &container,
		Prefix:    name,
		Provider:  &provider,
		SecretRef: &corev1.SecretReference{
			Name: "etcd-backup",
		},
	}
	return etcd
}

func getEtcdWithOSS(name, namespace string) *druidv1alpha1.Etcd {
	container := fmt.Sprintf("%s-container", name)
	provider := druidv1alpha1.StorageProvider("alicloud")
	etcd := getEtcdWithTLS(name, namespace)
	etcd.Spec.Backup.Store = &druidv1alpha1.StoreSpec{
		Container: &container,
		Prefix:    name,
		Provider:  &provider,
		SecretRef: &corev1.SecretReference{
			Name: "etcd-backup",
		},
	}
	return etcd
}

func getEtcdWithTLS(name, namespace string) *druidv1alpha1.Etcd {
	return getEtcd(name, namespace, true)
}

func getEtcdWithoutTLS(name, namespace string) *druidv1alpha1.Etcd {
	return getEtcd(name, namespace, false)
}

func getEtcdWithDefault(name, namespace string) *druidv1alpha1.Etcd {
	var replicas uint32 = 1
	instance := &druidv1alpha1.Etcd{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
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
			Replicas: &replicas,
			Backup:   druidv1alpha1.BackupSpec{},
			Etcd:     druidv1alpha1.EtcdConfig{},
		},
	}
	return instance
}

func getEtcd(name, namespace string, tlsEnabled bool) *druidv1alpha1.Etcd {
	var replicas uint32 = 1
	instance := &druidv1alpha1.Etcd{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
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
			Replicas:            &replicas,
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
					Provider:  &provider,
					Prefix:    prefix,
				},
			},
			Etcd: druidv1alpha1.EtcdConfig{
				Quota:                   &quota,
				Metrics:                 &metricsBasic,
				Image:                   &imageEtcd,
				DefragmentationSchedule: &defragSchedule,
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
		},
	}

	if tlsEnabled {
		tlsConfig := &druidv1alpha1.TLSConfig{
			ClientTLSSecretRef: corev1.SecretReference{
				Name: "etcd-client-tls",
			},
			ServerTLSSecretRef: corev1.SecretReference{
				Name: "etcd-server-tls",
			},
			TLSCASecretRef: corev1.SecretReference{
				Name: "ca-etcd",
			},
		}
		instance.Spec.Etcd.TLS = tlsConfig
	}
	return instance
}

func parseQuantity(q string) resource.Quantity {
	val, _ := resource.ParseQuantity(q)
	return val
}

func createSecrets(c client.Client, namespace string, secrets ...string) []error {
	var errors []error
	for _, name := range secrets {
		secret := corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Data: map[string][]byte{
				"test": []byte("test"),
			},
		}
		err := c.Create(context.TODO(), &secret)
		if apierrors.IsAlreadyExists(err) {
			continue
		}
		if err != nil {
			errors = append(errors, err)
		}
	}
	return errors
}

// WithWd sets the working directory and returns a function to revert to the previous one.
func WithWd(path string) func() {
	oldPath, err := os.Getwd()
	if err != nil {
		Expect(err).NotTo(HaveOccurred())
	}

	if err := os.Chdir(path); err != nil {
		Expect(err).NotTo(HaveOccurred())
	}

	return func() {
		if err := os.Chdir(oldPath); err != nil {
			Expect(err).NotTo(HaveOccurred())
		}
	}
}

func setStatefulSetReady(s *appsv1.StatefulSet) {
	logger.Infof("Setting statefulset ready explicitly")
	s.Status.ObservedGeneration = s.Generation

	replicas := int32(1)
	if s.Spec.Replicas != nil {
		replicas = *s.Spec.Replicas
	}
	s.Status.Replicas = replicas
	s.Status.ReadyReplicas = replicas
}
