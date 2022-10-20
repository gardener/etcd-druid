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
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/pkg/common"
	"github.com/gardener/etcd-druid/pkg/utils"

	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	gardenerUtils "github.com/gardener/gardener/pkg/utils"
	"github.com/gardener/gardener/pkg/utils/imagevector"
	"github.com/gardener/gardener/pkg/utils/test/matchers"
	"github.com/ghodss/yaml"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const (
	timeout         = time.Minute * 2
	pollingInterval = time.Second * 2
	etcdConfig      = "etcd.conf.yaml"
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
	imageEtcd                     = "eu.gcr.io/gardener-project/gardener/etcd:v3.4.13-bootstrap"
	imageBR                       = "eu.gcr.io/gardener-project/gardener/etcdbrctl:v0.12.0"
	snapshotSchedule              = "0 */24 * * *"
	defragSchedule                = "0 */24 * * *"
	container                     = "default.bkp"
	storageCapacity               = resource.MustParse("5Gi")
	defaultStorageCapacity        = resource.MustParse("16Gi")
	storageClass                  = "gardener.fast"
	priorityClassName             = "class_priority"
	deltaSnapShotMemLimit         = resource.MustParse("100Mi")
	autoCompactionMode            = druidv1alpha1.Periodic
	autoCompactionRetention       = "2m"
	quota                         = resource.MustParse("8Gi")
	provider                      = druidv1alpha1.StorageProvider("Local")
	prefix                        = "/tmp"
	uid                           = "a9b8c7d6e5f4"
	volumeClaimTemplateName       = "etcd-main"
	garbageCollectionPolicy       = druidv1alpha1.GarbageCollectionPolicy(druidv1alpha1.GarbageCollectionPolicyExponential)
	metricsBasic                  = druidv1alpha1.Basic
	maxBackups                    = 7
	imageNames                    = []string{
		common.Etcd,
		common.BackupRestore,
	}
	etcdSnapshotTimeout = metav1.Duration{
		Duration: 10 * time.Minute,
	}
	etcdDefragTimeout = metav1.Duration{
		Duration: 10 * time.Minute,
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
	return element.(string)
}

func ruleIterator(element interface{}) string {
	return element.(rbac.PolicyRule).APIGroups[0]
}

func stringArrayIterator(element interface{}) string {
	return element.(string)
}

var _ = Describe("Druid", func() {
	//Reconciliation of new etcd resource deployment without any existing statefulsets.
	Context("when adding etcd resources", func() {
		var (
			err      error
			instance *druidv1alpha1.Etcd
			sts      *appsv1.StatefulSet
			svc      *corev1.Service
			c        client.Client
		)

		BeforeEach(func() {
			instance = getEtcd("foo1", "default", false)
			c = mgr.GetClient()
			ns := corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: instance.Namespace,
				},
			}
			_, err = controllerutil.CreateOrUpdate(context.TODO(), c, &ns, func() error { return nil })
			Expect(err).To(Not(HaveOccurred()))

			storeSecret := instance.Spec.Backup.Store.SecretRef.Name
			errors := createSecrets(c, instance.Namespace, storeSecret)
			Expect(len(errors)).Should(BeZero())
			Expect(c.Create(context.TODO(), instance)).To(Succeed())

			sts = &appsv1.StatefulSet{}
			// Wait until StatefulSet has been created by controller
			Eventually(func() error {
				return c.Get(context.TODO(), types.NamespacedName{
					Name:      instance.Name,
					Namespace: instance.Namespace,
				}, sts)
			}, timeout, pollingInterval).Should(BeNil())

			svc = &corev1.Service{}
			// Wait until Service has been created by controller
			Eventually(func() error {
				return c.Get(context.TODO(), types.NamespacedName{
					Name:      utils.GetClientServiceName(instance),
					Namespace: instance.Namespace,
				}, svc)
			}, timeout, pollingInterval).Should(BeNil())

		})
		It("should create and adopt statefulset", func() {
			setStatefulSetReady(sts)
			err = c.Status().Update(context.TODO(), sts)
			Eventually(func() error { return statefulsetIsCorrectlyReconciled(c, instance, sts) }, timeout, pollingInterval).Should(BeNil())
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() (*int32, error) {
				if err := c.Get(context.TODO(), client.ObjectKeyFromObject(instance), instance); err != nil {
					return nil, err
				}
				return instance.Status.ClusterSize, nil
			}, timeout, pollingInterval).Should(Equal(pointer.Int32Ptr(instance.Spec.Replicas)))
		})
		It("should create and adopt statefulset and printing events", func() {
			// Check StatefulSet requirements
			Expect(len(sts.Spec.VolumeClaimTemplates)).To(Equal(1))
			Expect(sts.Spec.Replicas).To(PointTo(Equal(int32(1))))

			// Create PVC
			pvc := &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-%s-%d", sts.Spec.VolumeClaimTemplates[0].Name, sts.Name, 0),
					Namespace: sts.Namespace,
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse("1Gi"),
						},
					},
				},
			}
			Expect(c.Create(context.TODO(), pvc)).To(Succeed())

			// Create PVC warning Event
			pvcMessage := "Failed to provision volume"
			Expect(c.Create(context.TODO(), &corev1.Event{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pvc-event-1",
					Namespace: pvc.Namespace,
				},
				InvolvedObject: corev1.ObjectReference{
					APIVersion: "v1",
					Kind:       "PersistentVolumeClaim",
					Name:       pvc.Name,
					Namespace:  pvc.Namespace,
				},
				Type:    corev1.EventTypeWarning,
				Message: pvcMessage,
			})).To(Succeed())

			// Eventually, warning message should be reflected in `etcd` object status.
			Eventually(func() string {
				if err := c.Get(context.TODO(), client.ObjectKeyFromObject(instance), instance); err != nil {
					return ""
				}
				if instance.Status.LastError == nil {
					return ""
				}
				return *instance.Status.LastError
			}, timeout, pollingInterval).Should(ContainSubstring(pvcMessage))
		})
		AfterEach(func() {
			// Delete `etcd` instance
			Expect(c.Delete(context.TODO(), instance)).To(Succeed())
			Eventually(func() error {
				return c.Get(context.TODO(), client.ObjectKeyFromObject(instance), &druidv1alpha1.Etcd{})
			}, timeout, pollingInterval).Should(matchers.BeNotFoundError())
			// Delete service manually because garbage collection is not available in `envtest`
			Expect(c.Delete(context.TODO(), svc)).To(Succeed())
			Eventually(func() error {
				return c.Get(context.TODO(), client.ObjectKeyFromObject(svc), &corev1.Service{})
			}, timeout, pollingInterval).Should(matchers.BeNotFoundError())

		})
	})

	Describe("Druid custodian controller", func() {
		Context("when statefulset status is updated", func() {
			var (
				instance *druidv1alpha1.Etcd
				sts      *appsv1.StatefulSet
				c        client.Client
			)

			BeforeEach(func() {
				ctx, cancel := context.WithTimeout(context.TODO(), timeout)
				defer cancel()

				instance = getEtcd("foo19", "default", false)
				c = mgr.GetClient()

				ns := corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: instance.Namespace,
					},
				}
				_, err := controllerutil.CreateOrUpdate(ctx, c, &ns, func() error { return nil })
				Expect(err).To(Not(HaveOccurred()))

				storeSecret := instance.Spec.Backup.Store.SecretRef.Name
				errors := createSecrets(c, instance.Namespace, storeSecret)
				Expect(len(errors)).Should(BeZero())
				Expect(c.Create(ctx, instance)).To(Succeed())

				sts = &appsv1.StatefulSet{}
				// Wait until StatefulSet has been created by controller
				Eventually(func() error {
					return c.Get(ctx, types.NamespacedName{
						Name:      instance.Name,
						Namespace: instance.Namespace,
					}, sts)
				}, timeout, pollingInterval).Should(BeNil())

				sts.Status.Replicas = 1
				sts.Status.ReadyReplicas = 1
				sts.Status.ObservedGeneration = 2
				Expect(c.Status().Update(ctx, sts)).To(Succeed())

				Eventually(func() error {
					if err := c.Get(ctx, client.ObjectKeyFromObject(instance), sts); err != nil {
						return err
					}
					if sts.Status.ReadyReplicas != 1 {
						return fmt.Errorf("ReadyReplicas != 1")
					}
					return nil
				}, timeout, pollingInterval).Should(Succeed())

				Eventually(func() error { return statefulsetIsCorrectlyReconciled(c, instance, sts) }, timeout, pollingInterval).Should(BeNil())

				// Check if ETCD has ready replicas more than zero
				Eventually(func() error {
					if err := c.Get(ctx, client.ObjectKeyFromObject(instance), instance); err != nil {
						return err
					}

					if int(instance.Status.ReadyReplicas) < 1 {
						return fmt.Errorf("ETCD ready replicas should be more than zero")
					}
					return nil
				}, timeout, pollingInterval).Should(BeNil())
			})
			It("mark statefulset status not ready when no readyreplicas in statefulset", func() {
				ctx, cancel := context.WithTimeout(context.TODO(), timeout)
				defer cancel()

				err := c.Get(ctx, client.ObjectKeyFromObject(instance), sts)
				Expect(err).NotTo(HaveOccurred())

				// Forcefully change readyreplicas in statefulset as zero which may cause due to facts like crashloopbackoff
				sts.Status.ReadyReplicas = 0
				Expect(c.Status().Update(ctx, sts)).To(Succeed())

				Eventually(func() error {
					err := c.Get(ctx, client.ObjectKeyFromObject(instance), sts)
					if err != nil {
						return err
					}

					if sts.Status.ReadyReplicas > 0 {
						return fmt.Errorf("No readyreplicas of statefulset should exist at this point")
					}

					err = c.Get(ctx, client.ObjectKeyFromObject(instance), instance)
					if err != nil {
						return err
					}

					if instance.Status.ReadyReplicas > 0 {
						return fmt.Errorf("ReadyReplicas should be zero in ETCD instance")
					}

					return nil
				}, timeout, pollingInterval).Should(BeNil())
			})
			AfterEach(func() {
				ctx, cancel := context.WithTimeout(context.TODO(), timeout)
				defer cancel()

				// Delete `etcd` instance
				Expect(c.Delete(ctx, instance)).To(Succeed())
				Eventually(func() error {
					err := c.Get(ctx, client.ObjectKeyFromObject(instance), &druidv1alpha1.Etcd{})
					if err != nil {
						return err
					}

					return c.Get(ctx, client.ObjectKeyFromObject(instance), sts)
				}, timeout, pollingInterval).Should(matchers.BeNotFoundError())
			})
		})
	})

	DescribeTable("when etcd resource is created",
		func(name string, generateEtcd func(string, string) *druidv1alpha1.Etcd, validate func(*druidv1alpha1.Etcd, *appsv1.StatefulSet, *corev1.ConfigMap, *corev1.Service, *corev1.Service)) {
			var err error
			var instance *druidv1alpha1.Etcd
			var c client.Client
			var s *appsv1.StatefulSet
			var cm *corev1.ConfigMap
			var clSvc, prSvc *corev1.Service
			var sa *corev1.ServiceAccount
			var role *rbac.Role
			var rb *rbac.RoleBinding

			instance = generateEtcd(name, "default")
			c = mgr.GetClient()
			ns := corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: instance.Namespace,
				},
			}

			_, err = controllerutil.CreateOrUpdate(context.TODO(), c, &ns, func() error { return nil })
			Expect(err).To(Not(HaveOccurred()))

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
			clSvc = &corev1.Service{}
			Eventually(func() error { return clientServiceIsCorrectlyReconciled(c, instance, clSvc) }, timeout, pollingInterval).Should(BeNil())
			prSvc = &corev1.Service{}
			Eventually(func() error { return peerServiceIsCorrectlyReconciled(c, instance, prSvc) }, timeout, pollingInterval).Should(BeNil())
			sa = &corev1.ServiceAccount{}
			Eventually(func() error { return serviceAccountIsCorrectlyReconciled(c, instance, sa) }, timeout, pollingInterval).Should(BeNil())
			role = &rbac.Role{}
			Eventually(func() error { return roleIsCorrectlyReconciled(c, instance, role) }, timeout, pollingInterval).Should(BeNil())
			rb = &rbac.RoleBinding{}
			Eventually(func() error { return roleBindingIsCorrectlyReconciled(c, instance, rb) }, timeout, pollingInterval).Should(BeNil())

			validate(instance, s, cm, clSvc, prSvc)
			validateRole(instance, role)

			setStatefulSetReady(s)
			err = c.Status().Update(context.TODO(), s)
			Expect(err).NotTo(HaveOccurred())
		},
		Entry("if fields are not set in etcd.Spec, the statefulset should reflect the spec changes", "foo28", getEtcdWithDefault, validateEtcdWithDefaults),
		Entry("if fields are set in etcd.Spec and TLS enabled, the resources should reflect the spec changes", "foo29", getEtcdWithTLS, validateEtcd),
		Entry("if the store is GCS, the statefulset should reflect the spec changes", "foo30", getEtcdWithGCS, validateStoreGCP),
		Entry("if the store is S3, the statefulset should reflect the spec changes", "foo31", getEtcdWithS3, validateStoreAWS),
		Entry("if the store is ABS, the statefulset should reflect the spec changes", "foo32", getEtcdWithABS, validateStoreAzure),
		Entry("if the store is Swift, the statefulset should reflect the spec changes", "foo33", getEtcdWithSwift, validateStoreOpenstack),
		Entry("if the store is OSS, the statefulset should reflect the spec changes", "foo34", getEtcdWithOSS, validateStoreAlicloud),
	)
})

var _ = Describe("Multinode ETCD", func() {
	//Reconciliation of new etcd resource deployment without any existing statefulsets.
	Context("when adding etcd resources", func() {
		var (
			err      error
			instance *druidv1alpha1.Etcd
			sts      *appsv1.StatefulSet
			svc      *corev1.Service
			c        client.Client

			ctx = context.TODO()
		)

		BeforeEach(func() {
			instance = getEtcd("foo82", "default", false)
			c = mgr.GetClient()
			ns := corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: instance.Namespace,
				},
			}
			_, err = controllerutil.CreateOrUpdate(ctx, c, &ns, func() error { return nil })
			Expect(err).To(Not(HaveOccurred()))

			storeSecret := instance.Spec.Backup.Store.SecretRef.Name
			errors := createSecrets(c, instance.Namespace, storeSecret)
			Expect(len(errors)).Should(BeZero())
		})
		It("should create the statefulset based on the replicas in ETCD CR", func() {
			// First delete existing statefulset if any.
			// This is required due to a bug
			sts = &appsv1.StatefulSet{}
			sts.Name = instance.Name
			sts.Namespace = instance.Namespace
			Expect(client.IgnoreNotFound(c.Delete(ctx, sts))).To(Succeed())

			By("update replicas in ETCD resource with 0")
			instance.Spec.Replicas = 4
			Expect(c.Create(ctx, instance)).To(Succeed())

			Eventually(func() error {
				return c.Get(ctx, types.NamespacedName{
					Name:      instance.Name,
					Namespace: instance.Namespace,
				}, instance)
			}, timeout, pollingInterval).Should(BeNil())

			By("no StatefulSet has been created by controller as even number of replicas are not allowed")
			Eventually(func() error {
				return c.Get(ctx, types.NamespacedName{
					Name:      instance.Name,
					Namespace: instance.Namespace,
				}, sts)
			}, timeout, pollingInterval).Should(matchers.BeNotFoundError())

			By("update replicas in ETCD resource with 3")
			patch := client.MergeFrom(instance.DeepCopy())
			instance.Spec.Replicas = 3
			Expect(c.Patch(ctx, instance, patch)).To(Succeed())

			By("statefulsets are created when ETCD replicas are odd number")
			Eventually(func() error { return statefulsetIsCorrectlyReconciled(c, instance, sts) }, timeout, pollingInterval).Should(BeNil())
			Expect(int(*sts.Spec.Replicas)).To(Equal(3))

			By("client Service has been created by controller")
			svc = &corev1.Service{}
			Eventually(func() error { return clientServiceIsCorrectlyReconciled(c, instance, svc) }, timeout, pollingInterval).Should(BeNil())

			By("should raise an event if annotation to ignore reconciliation is applied on ETCD CR")
			patch = client.MergeFrom(instance.DeepCopy())
			annotations := utils.MergeStringMaps(
				map[string]string{
					IgnoreReconciliationAnnotation: "true",
				},
				instance.Annotations,
			)
			instance.Annotations = annotations
			Expect(c.Patch(ctx, instance, patch)).To(Succeed())

			config := mgr.GetConfig()
			clientset, _ := kubernetes.NewForConfig(config)
			Eventually(func() error {
				events, err := clientset.CoreV1().Events(instance.Namespace).List(ctx, metav1.ListOptions{})
				if err != nil {
					fmt.Printf("The error is : %v", err)
					return err
				}

				if events == nil || len(events.Items) == 0 {
					return fmt.Errorf("No events generated for annotation to ignore reconciliation")
				}

				for _, event := range events.Items {
					if event.Reason == "ReconciliationIgnored" {
						return nil
					}
				}
				return nil

			}, timeout, pollingInterval).Should(BeNil())

			By("delete `etcd` instance")
			Expect(client.IgnoreNotFound(c.Delete(context.TODO(), instance))).To(Succeed())
			Eventually(func() error {
				return c.Get(context.TODO(), client.ObjectKeyFromObject(instance), &druidv1alpha1.Etcd{})
			}, timeout, pollingInterval).Should(matchers.BeNotFoundError())

			By("delete service manually because garbage collection is not available in `envtest`")
			if svc != nil {
				Expect(c.Delete(context.TODO(), svc)).To(Succeed())
				Eventually(func() error {
					return c.Get(context.TODO(), client.ObjectKeyFromObject(svc), &corev1.Service{})
				}, timeout, pollingInterval).Should(matchers.BeNotFoundError())
			}
		})
	})
	DescribeTable("configmaps are mounted properly when ETCD replicas are odd number", func(name string, replicas int, getEtcdWithReplicas func(string, string, int) *druidv1alpha1.Etcd) {
		var err error
		var instance *druidv1alpha1.Etcd
		var c client.Client
		var sts *appsv1.StatefulSet
		var cm *corev1.ConfigMap
		var svc *corev1.Service

		instance = getEtcdWithReplicas(name, "default", replicas)
		c = mgr.GetClient()
		ns := corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: instance.Namespace,
			},
		}

		_, err = controllerutil.CreateOrUpdate(context.TODO(), c, &ns, func() error { return nil })
		Expect(err).To(Not(HaveOccurred()))

		if instance.Spec.Backup.Store != nil && instance.Spec.Backup.Store.SecretRef != nil {
			storeSecret := instance.Spec.Backup.Store.SecretRef.Name
			errors := createSecrets(c, instance.Namespace, storeSecret)
			Expect(len(errors)).Should(BeZero())
		}
		err = c.Create(context.TODO(), instance)
		Expect(err).NotTo(HaveOccurred())
		sts = &appsv1.StatefulSet{}
		Eventually(func() error { return statefulsetIsCorrectlyReconciled(c, instance, sts) }, timeout, pollingInterval).Should(BeNil())
		cm = &corev1.ConfigMap{}
		Eventually(func() error { return configMapIsCorrectlyReconciled(c, instance, cm) }, timeout, pollingInterval).Should(BeNil())
		svc = &corev1.Service{}
		Eventually(func() error { return clientServiceIsCorrectlyReconciled(c, instance, svc) }, timeout, pollingInterval).Should(BeNil())

		// Validate statefulset
		Expect(*sts.Spec.Replicas).To(Equal(int32(instance.Spec.Replicas)))

		if instance.Spec.Replicas == 1 {
			matcher := "initial-cluster: foo83-0=http://foo83-0.foo83-peer.default.svc:2380"
			Expect(strings.Contains(cm.Data["etcd.conf.yaml"], matcher)).To(BeTrue())
		}

		if instance.Spec.Replicas > 1 {
			matcher := "initial-cluster: foo84-0=http://foo84-0.foo84-peer.default.svc:2380,foo84-1=http://foo84-1.foo84-peer.default.svc:2380,foo84-2=http://foo84-2.foo84-peer.default.svc:2380"
			Expect(strings.Contains(cm.Data["etcd.conf.yaml"], matcher)).To(BeTrue())
		}
	},
		Entry("verify configmap mount path and etcd.conf.yaml when replica is 1 ", "foo83", 1, getEtcdWithReplicas),
		Entry("verify configmap mount path and etcd.conf.yaml when replica is 3 ", "foo84", 3, getEtcdWithReplicas),
	)
})

func validateRole(instance *druidv1alpha1.Etcd, role *rbac.Role) {
	Expect(*role).To(MatchFields(IgnoreExtras, Fields{
		"ObjectMeta": MatchFields(IgnoreExtras, Fields{
			"Name":      Equal(fmt.Sprintf("druid.gardener.cloud:etcd:%s", instance.Name)),
			"Namespace": Equal(instance.Namespace),
			"Labels": MatchKeys(IgnoreExtras, Keys{
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
		"Rules": MatchAllElements(ruleIterator, Elements{
			"coordination.k8s.io": MatchFields(IgnoreExtras, Fields{
				"APIGroups": MatchAllElements(stringArrayIterator, Elements{
					"coordination.k8s.io": Equal("coordination.k8s.io"),
				}),
				"Resources": MatchAllElements(stringArrayIterator, Elements{
					"leases": Equal("leases"),
				}),
				"Verbs": MatchAllElements(stringArrayIterator, Elements{
					"list":   Equal("list"),
					"get":    Equal("get"),
					"update": Equal("update"),
					"patch":  Equal("patch"),
					"watch":  Equal("watch"),
				}),
			}),
			"apps": MatchFields(IgnoreExtras, Fields{
				"APIGroups": MatchAllElements(stringArrayIterator, Elements{
					"apps": Equal("apps"),
				}),
				"Resources": MatchAllElements(stringArrayIterator, Elements{
					"statefulsets": Equal("statefulsets"),
				}),
				"Verbs": MatchAllElements(stringArrayIterator, Elements{
					"list":   Equal("list"),
					"get":    Equal("get"),
					"update": Equal("update"),
					"patch":  Equal("patch"),
					"watch":  Equal("watch"),
				}),
			}),
			"": MatchFields(IgnoreExtras, Fields{
				"APIGroups": MatchAllElements(stringArrayIterator, Elements{
					"": Equal(""),
				}),
				"Resources": MatchAllElements(stringArrayIterator, Elements{
					"pods": Equal("pods"),
				}),
				"Verbs": MatchAllElements(stringArrayIterator, Elements{
					"list":  Equal("list"),
					"get":   Equal("get"),
					"watch": Equal("watch"),
				}),
			}),
		}),
	}))
}

func validateEtcdWithDefaults(instance *druidv1alpha1.Etcd, s *appsv1.StatefulSet, cm *corev1.ConfigMap, clSvc *corev1.Service, prSvc *corev1.Service) {
	configYML := cm.Data[etcdConfig]
	config := map[string]interface{}{}
	err := yaml.Unmarshal([]byte(configYML), &config)
	Expect(err).NotTo(HaveOccurred())

	// Validate ETCD annotation for configmap checksum
	jsonString, err := json.Marshal(cm.Data)
	Expect(err).NotTo(HaveOccurred())
	configMapChecksum := gardenerUtils.ComputeSHA256Hex(jsonString)

	// Validate Metrics MetricsLevel
	Expect(instance.Spec.Etcd.Metrics).To(BeNil())
	Expect(config).To(HaveKeyWithValue(metricsKey, string(druidv1alpha1.Basic)))

	// Validate DefragmentationSchedule *string
	Expect(instance.Spec.Etcd.DefragmentationSchedule).To(BeNil())

	// Validate ServerPort and ClientPort
	Expect(instance.Spec.Etcd.ServerPort).To(BeNil())
	Expect(instance.Spec.Etcd.ClientPort).To(BeNil())

	Expect(instance.Spec.Etcd.Image).To(BeNil())
	imageVector, err := imagevector.ReadGlobalImageVectorWithEnvOverride(getImageYAMLPath())
	Expect(err).NotTo(HaveOccurred())
	images, err := imagevector.FindImages(imageVector, imageNames)
	Expect(err).NotTo(HaveOccurred())

	// Validate Resources
	// resources:
	//	  requests:
	//		cpu: 50m
	//		memory: 128Mi
	Expect(instance.Spec.Etcd.Resources).To(BeNil())

	// Validate TLS. Ensure that enableTLS flag is not triggered in the go-template
	Expect(instance.Spec.Etcd.PeerUrlTLS).To(BeNil())

	Expect(config).To(MatchKeys(IgnoreExtras, Keys{
		"name":                        Equal(fmt.Sprintf("etcd-%s", instance.UID[:6])),
		"data-dir":                    Equal("/var/etcd/data/new.etcd"),
		"metrics":                     Equal(string(druidv1alpha1.Basic)),
		"snapshot-count":              Equal(float64(75000)),
		"enable-v2":                   Equal(false),
		"quota-backend-bytes":         Equal(float64(8589934592)),
		"listen-client-urls":          Equal(fmt.Sprintf("http://0.0.0.0:%d", clientPort)),
		"advertise-client-urls":       Equal(fmt.Sprintf("%s@%s@%s@%d", "http", prSvc.Name, instance.Namespace, clientPort)),
		"listen-peer-urls":            Equal(fmt.Sprintf("http://0.0.0.0:%d", serverPort)),
		"initial-advertise-peer-urls": Equal(fmt.Sprintf("%s@%s@%s@%d", "http", prSvc.Name, instance.Namespace, serverPort)),
		"initial-cluster-token":       Equal("etcd-cluster"),
		"initial-cluster-state":       Equal("new"),
		"auto-compaction-mode":        Equal(string(druidv1alpha1.Periodic)),
		"auto-compaction-retention":   Equal(DefaultAutoCompactionRetention),
	}))

	Expect(*clSvc).To(MatchFields(IgnoreExtras, Fields{
		"ObjectMeta": MatchFields(IgnoreExtras, Fields{
			"Name":      Equal(utils.GetClientServiceName(instance)),
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
				"checksum/etcd-configmap":   Equal(configMapChecksum),
			}),
			"Labels": MatchAllKeys(Keys{
				"name":     Equal("etcd"),
				"instance": Equal(instance.Name),
				"app":      Equal("etcd-statefulset"),
			}),
		}),
		"Spec": MatchFields(IgnoreExtras, Fields{
			"UpdateStrategy": MatchFields(IgnoreExtras, Fields{
				"Type": Equal(appsv1.RollingUpdateStatefulSetStrategyType),
			}),
			"ServiceName": Equal(utils.GetPeerServiceName(instance)),
			"Replicas":    PointTo(Equal(int32(instance.Spec.Replicas))),
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
						"app":      Equal("etcd-statefulset"),
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
								{
									Name:          "server",
									Protocol:      corev1.ProtocolTCP,
									HostPort:      0,
									ContainerPort: serverPort,
								},
								{
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
							}),
							"ReadinessProbe": PointTo(MatchFields(IgnoreExtras, Fields{
								"Handler": MatchFields(IgnoreExtras, Fields{
									"HTTPGet": PointTo(MatchFields(IgnoreExtras, Fields{
										"Path":   Equal("/healthz"),
										"Port":   Equal(intstr.FromInt(int(backupPort))),
										"Scheme": Equal(corev1.URISchemeHTTP),
									})),
								}),
								"InitialDelaySeconds": Equal(int32(15)),
								"PeriodSeconds":       Equal(int32(5)),
								"FailureThreshold":    Equal(int32(5)),
							})),
							"VolumeMounts": MatchAllElements(volumeMountIterator, Elements{
								instance.Name: MatchFields(IgnoreExtras, Fields{
									"Name":      Equal(instance.Name),
									"MountPath": Equal("/var/etcd/data/"),
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
								"--etcd-process-name=etcd":                       Equal("--etcd-process-name=etcd"),
								"--enable-member-lease-renewal=true":             Equal("--enable-member-lease-renewal=true"),
								"--k8s-heartbeat-duration=10s":                   Equal("--k8s-heartbeat-duration=10s"),

								fmt.Sprintf("--delta-snapshot-memory-limit=%d", deltaSnapShotMemLimit.Value()):                    Equal(fmt.Sprintf("--delta-snapshot-memory-limit=%d", deltaSnapShotMemLimit.Value())),
								fmt.Sprintf("--garbage-collection-policy=%s", druidv1alpha1.GarbageCollectionPolicyLimitBased):    Equal(fmt.Sprintf("--garbage-collection-policy=%s", druidv1alpha1.GarbageCollectionPolicyLimitBased)),
								fmt.Sprintf("--endpoints=http://%s-local:%d", instance.Name, clientPort):                          Equal(fmt.Sprintf("--endpoints=http://%s-local:%d", instance.Name, clientPort)),
								fmt.Sprintf("--service-endpoints=http://%s:%d", utils.GetClientServiceName(instance), clientPort): Equal(fmt.Sprintf("--service-endpoints=http://%s:%d", utils.GetClientServiceName(instance), clientPort)),
								fmt.Sprintf("--embedded-etcd-quota-bytes=%d", int64(quota.Value())):                               Equal(fmt.Sprintf("--embedded-etcd-quota-bytes=%d", int64(quota.Value()))),
								fmt.Sprintf("--max-backups=%d", maxBackups):                                                       Equal(fmt.Sprintf("--max-backups=%d", maxBackups)),
								fmt.Sprintf("--auto-compaction-mode=%s", druidv1alpha1.Periodic):                                  Equal(fmt.Sprintf("--auto-compaction-mode=%s", druidv1alpha1.Periodic)),
								fmt.Sprintf("--auto-compaction-retention=%s", DefaultAutoCompactionRetention):                     Equal(fmt.Sprintf("--auto-compaction-retention=%s", DefaultAutoCompactionRetention)),
								fmt.Sprintf("%s=%s", "--etcd-snapshot-timeout", "15m"):                                            Equal(fmt.Sprintf("%s=%s", "--etcd-snapshot-timeout", "15m")),
								fmt.Sprintf("%s=%s", "--etcd-defrag-timeout", "15m"):                                              Equal(fmt.Sprintf("%s=%s", "--etcd-defrag-timeout", "15m")),
							}),
							"Ports": ConsistOf([]corev1.ContainerPort{
								{
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

func validateEtcd(instance *druidv1alpha1.Etcd, s *appsv1.StatefulSet, cm *corev1.ConfigMap, clSvc *corev1.Service, prSvc *corev1.Service) {
	configYML := cm.Data[etcdConfig]
	config := map[string]interface{}{}
	err := yaml.Unmarshal([]byte(configYML), &config)
	Expect(err).NotTo(HaveOccurred())

	// Validate ETCD annotation for configmap checksum
	jsonString, err := json.Marshal(cm.Data)
	Expect(err).NotTo(HaveOccurred())
	configMapChecksum := gardenerUtils.ComputeSHA256Hex(jsonString)

	// Validate Metrics MetricsLevel
	Expect(instance.Spec.Etcd.Metrics).NotTo(BeNil())
	Expect(config).To(HaveKeyWithValue(metricsKey, string(*instance.Spec.Etcd.Metrics)))

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
	}))

	Expect(config).To(MatchKeys(IgnoreExtras, Keys{
		"name":                Equal(fmt.Sprintf("etcd-%s", instance.UID[:6])),
		"data-dir":            Equal("/var/etcd/data/new.etcd"),
		"metrics":             Equal(string(*instance.Spec.Etcd.Metrics)),
		"snapshot-count":      Equal(float64(75000)),
		"enable-v2":           Equal(false),
		"quota-backend-bytes": Equal(float64(instance.Spec.Etcd.Quota.Value())),

		"client-transport-security": MatchKeys(IgnoreExtras, Keys{
			"cert-file":        Equal("/var/etcd/ssl/client/server/tls.crt"),
			"key-file":         Equal("/var/etcd/ssl/client/server/tls.key"),
			"client-cert-auth": Equal(true),
			"trusted-ca-file":  Equal("/var/etcd/ssl/client/ca/ca.crt"),
			"auto-tls":         Equal(false),
		}),
		"listen-client-urls":    Equal(fmt.Sprintf("https://0.0.0.0:%d", *instance.Spec.Etcd.ClientPort)),
		"advertise-client-urls": Equal(fmt.Sprintf("%s@%s@%s@%d", "https", prSvc.Name, instance.Namespace, *instance.Spec.Etcd.ClientPort)),

		"peer-transport-security": MatchKeys(IgnoreExtras, Keys{
			"cert-file":        Equal("/var/etcd/ssl/peer/server/tls.crt"),
			"key-file":         Equal("/var/etcd/ssl/peer/server/tls.key"),
			"client-cert-auth": Equal(true),
			"trusted-ca-file":  Equal("/var/etcd/ssl/peer/ca/ca.crt"),
			"auto-tls":         Equal(false),
		}),
		"listen-peer-urls":            Equal(fmt.Sprintf("https://0.0.0.0:%d", *instance.Spec.Etcd.ServerPort)),
		"initial-advertise-peer-urls": Equal(fmt.Sprintf("%s@%s@%s@%d", "https", prSvc.Name, instance.Namespace, *instance.Spec.Etcd.ServerPort)),

		"initial-cluster-token":     Equal("etcd-cluster"),
		"initial-cluster-state":     Equal("new"),
		"auto-compaction-mode":      Equal(string(*instance.Spec.Common.AutoCompactionMode)),
		"auto-compaction-retention": Equal(*instance.Spec.Common.AutoCompactionRetention),
	}))

	Expect(*clSvc).To(MatchFields(IgnoreExtras, Fields{
		"ObjectMeta": MatchFields(IgnoreExtras, Fields{
			"Name":      Equal(utils.GetClientServiceName(instance)),
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
				"checksum/etcd-configmap":   Equal(configMapChecksum),
			}),
			"Labels": MatchAllKeys(Keys{
				"name":     Equal("etcd"),
				"instance": Equal(instance.Name),
				"app":      Equal("etcd-statefulset"),
			}),
		}),

		"Spec": MatchFields(IgnoreExtras, Fields{
			"UpdateStrategy": MatchFields(IgnoreExtras, Fields{
				"Type": Equal(appsv1.RollingUpdateStatefulSetStrategyType),
			}),
			"ServiceName": Equal(utils.GetPeerServiceName(instance)),
			"Replicas":    PointTo(Equal(int32(instance.Spec.Replicas))),
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
						"app":      Equal("etcd-statefulset"),
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
								{
									Name:          "server",
									Protocol:      corev1.ProtocolTCP,
									HostPort:      0,
									ContainerPort: *instance.Spec.Etcd.ServerPort,
								},
								{
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
										"Path":   Equal("/healthz"),
										"Port":   Equal(intstr.FromInt(int(backupPort))),
										"Scheme": Equal(corev1.URISchemeHTTPS),
									})),
								}),
								"InitialDelaySeconds": Equal(int32(15)),
								"PeriodSeconds":       Equal(int32(5)),
								"FailureThreshold":    Equal(int32(5)),
							})),
							"VolumeMounts": MatchAllElements(volumeMountIterator, Elements{
								*instance.Spec.VolumeClaimTemplate: MatchFields(IgnoreExtras, Fields{
									"Name":      Equal(*instance.Spec.VolumeClaimTemplate),
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
								"--cert=/var/etcd/ssl/client/client/tls.crt":                                                                        Equal("--cert=/var/etcd/ssl/client/client/tls.crt"),
								"--key=/var/etcd/ssl/client/client/tls.key":                                                                         Equal("--key=/var/etcd/ssl/client/client/tls.key"),
								"--cacert=/var/etcd/ssl/client/ca/ca.crt":                                                                           Equal("--cacert=/var/etcd/ssl/client/ca/ca.crt"),
								"--server-cert=/var/etcd/ssl/client/server/tls.crt":                                                                 Equal("--server-cert=/var/etcd/ssl/client/server/tls.crt"),
								"--server-key=/var/etcd/ssl/client/server/tls.key":                                                                  Equal("--server-key=/var/etcd/ssl/client/server/tls.key"),
								"--data-dir=/var/etcd/data/new.etcd":                                                                                Equal("--data-dir=/var/etcd/data/new.etcd"),
								"--insecure-transport=false":                                                                                        Equal("--insecure-transport=false"),
								"--insecure-skip-tls-verify=false":                                                                                  Equal("--insecure-skip-tls-verify=false"),
								"--snapstore-temp-directory=/var/etcd/data/temp":                                                                    Equal("--snapstore-temp-directory=/var/etcd/data/temp"),
								"--etcd-process-name=etcd":                                                                                          Equal("--etcd-process-name=etcd"),
								"--etcd-connection-timeout=5m":                                                                                      Equal("--etcd-connection-timeout=5m"),
								"--enable-snapshot-lease-renewal=true":                                                                              Equal("--enable-snapshot-lease-renewal=true"),
								"--enable-member-lease-renewal=true":                                                                                Equal("--enable-member-lease-renewal=true"),
								"--k8s-heartbeat-duration=10s":                                                                                      Equal("--k8s-heartbeat-duration=10s"),
								fmt.Sprintf("--defragmentation-schedule=%s", *instance.Spec.Etcd.DefragmentationSchedule):                           Equal(fmt.Sprintf("--defragmentation-schedule=%s", *instance.Spec.Etcd.DefragmentationSchedule)),
								fmt.Sprintf("--schedule=%s", *instance.Spec.Backup.FullSnapshotSchedule):                                            Equal(fmt.Sprintf("--schedule=%s", *instance.Spec.Backup.FullSnapshotSchedule)),
								fmt.Sprintf("%s=%s", "--garbage-collection-policy", *instance.Spec.Backup.GarbageCollectionPolicy):                  Equal(fmt.Sprintf("%s=%s", "--garbage-collection-policy", *instance.Spec.Backup.GarbageCollectionPolicy)),
								fmt.Sprintf("%s=%s", "--storage-provider", store):                                                                   Equal(fmt.Sprintf("%s=%s", "--storage-provider", store)),
								fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix):                                           Equal(fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix)),
								fmt.Sprintf("--delta-snapshot-memory-limit=%d", instance.Spec.Backup.DeltaSnapshotMemoryLimit.Value()):              Equal(fmt.Sprintf("--delta-snapshot-memory-limit=%d", instance.Spec.Backup.DeltaSnapshotMemoryLimit.Value())),
								fmt.Sprintf("--garbage-collection-policy=%s", *instance.Spec.Backup.GarbageCollectionPolicy):                        Equal(fmt.Sprintf("--garbage-collection-policy=%s", *instance.Spec.Backup.GarbageCollectionPolicy)),
								fmt.Sprintf("--endpoints=https://%s-local:%d", instance.Name, clientPort):                                           Equal(fmt.Sprintf("--endpoints=https://%s-local:%d", instance.Name, clientPort)),
								fmt.Sprintf("--service-endpoints=https://%s:%d", utils.GetClientServiceName(instance), clientPort):                  Equal(fmt.Sprintf("--service-endpoints=https://%s:%d", utils.GetClientServiceName(instance), clientPort)),
								fmt.Sprintf("--embedded-etcd-quota-bytes=%d", int64(instance.Spec.Etcd.Quota.Value())):                              Equal(fmt.Sprintf("--embedded-etcd-quota-bytes=%d", int64(instance.Spec.Etcd.Quota.Value()))),
								fmt.Sprintf("%s=%s", "--delta-snapshot-period", instance.Spec.Backup.DeltaSnapshotPeriod.Duration.String()):         Equal(fmt.Sprintf("%s=%s", "--delta-snapshot-period", instance.Spec.Backup.DeltaSnapshotPeriod.Duration.String())),
								fmt.Sprintf("%s=%s", "--garbage-collection-period", instance.Spec.Backup.GarbageCollectionPeriod.Duration.String()): Equal(fmt.Sprintf("%s=%s", "--garbage-collection-period", instance.Spec.Backup.GarbageCollectionPeriod.Duration.String())),
								fmt.Sprintf("%s=%s", "--auto-compaction-mode", *instance.Spec.Common.AutoCompactionMode):                            Equal(fmt.Sprintf("%s=%s", "--auto-compaction-mode", autoCompactionMode)),
								fmt.Sprintf("%s=%s", "--auto-compaction-retention", *instance.Spec.Common.AutoCompactionRetention):                  Equal(fmt.Sprintf("%s=%s", "--auto-compaction-retention", autoCompactionRetention)),
								fmt.Sprintf("%s=%s", "--etcd-snapshot-timeout", instance.Spec.Backup.EtcdSnapshotTimeout.Duration.String()):         Equal(fmt.Sprintf("%s=%s", "--etcd-snapshot-timeout", instance.Spec.Backup.EtcdSnapshotTimeout.Duration.String())),
								fmt.Sprintf("%s=%s", "--etcd-defrag-timeout", instance.Spec.Etcd.EtcdDefragTimeout.Duration.String()):               Equal(fmt.Sprintf("%s=%s", "--etcd-defrag-timeout", instance.Spec.Etcd.EtcdDefragTimeout.Duration.String())),
								fmt.Sprintf("%s=%s", "--owner-name", instance.Spec.Backup.OwnerCheck.Name):                                          Equal(fmt.Sprintf("%s=%s", "--owner-name", instance.Spec.Backup.OwnerCheck.Name)),
								fmt.Sprintf("%s=%s", "--owner-id", instance.Spec.Backup.OwnerCheck.ID):                                              Equal(fmt.Sprintf("%s=%s", "--owner-id", instance.Spec.Backup.OwnerCheck.ID)),
								fmt.Sprintf("%s=%s", "--owner-check-interval", instance.Spec.Backup.OwnerCheck.Interval.Duration.String()):          Equal(fmt.Sprintf("%s=%s", "--owner-check-interval", instance.Spec.Backup.OwnerCheck.Interval.Duration.String())),
								fmt.Sprintf("%s=%s", "--owner-check-timeout", instance.Spec.Backup.OwnerCheck.Timeout.Duration.String()):            Equal(fmt.Sprintf("%s=%s", "--owner-check-timeout", instance.Spec.Backup.OwnerCheck.Timeout.Duration.String())),
								fmt.Sprintf("%s=%s", "--owner-check-dns-cache-ttl", instance.Spec.Backup.OwnerCheck.DNSCacheTTL.Duration.String()):  Equal(fmt.Sprintf("%s=%s", "--owner-check-dns-cache-ttl", instance.Spec.Backup.OwnerCheck.DNSCacheTTL.Duration.String())),
								fmt.Sprintf("%s=%s", "--delta-snapshot-lease-name", utils.GetDeltaSnapshotLeaseName(instance)):                      Equal(fmt.Sprintf("%s=%s", "--delta-snapshot-lease-name", utils.GetDeltaSnapshotLeaseName(instance))),
								fmt.Sprintf("%s=%s", "--full-snapshot-lease-name", utils.GetFullSnapshotLeaseName(instance)):                        Equal(fmt.Sprintf("%s=%s", "--full-snapshot-lease-name", utils.GetFullSnapshotLeaseName(instance))),
							}),
							"Ports": ConsistOf([]corev1.ContainerPort{
								{
									Name:          "server",
									Protocol:      corev1.ProtocolTCP,
									HostPort:      0,
									ContainerPort: backupPort,
								},
							}),
							"Image":           Equal(*instance.Spec.Backup.Image),
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
								"host-storage": MatchFields(IgnoreExtras, Fields{
									"Name":      Equal("host-storage"),
									"MountPath": Equal(*instance.Spec.Backup.Store.Container),
								}),
							}),
							"Env": MatchElements(envIterator, IgnoreExtras, Elements{
								"STORAGE_CONTAINER": MatchFields(IgnoreExtras, Fields{
									"Name":  Equal("STORAGE_CONTAINER"),
									"Value": Equal(*instance.Spec.Backup.Store.Container),
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
						"host-storage": MatchFields(IgnoreExtras, Fields{
							"Name": Equal("host-storage"),
							"VolumeSource": MatchFields(IgnoreExtras, Fields{
								"HostPath": PointTo(MatchFields(IgnoreExtras, Fields{
									"Path": Equal(fmt.Sprintf("/etc/gardener/local-backupbuckets/%s", *instance.Spec.Backup.Store.Container)),
									"Type": PointTo(Equal(corev1.HostPathType("Directory"))),
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
						"client-url-etcd-server-tls": MatchFields(IgnoreExtras, Fields{
							"Name": Equal("client-url-etcd-server-tls"),
							"VolumeSource": MatchFields(IgnoreExtras, Fields{
								"Secret": PointTo(MatchFields(IgnoreExtras, Fields{
									"SecretName": Equal(instance.Spec.Etcd.ClientUrlTLS.ServerTLSSecretRef.Name),
								})),
							}),
						}),
						"client-url-etcd-client-tls": MatchFields(IgnoreExtras, Fields{
							"Name": Equal("client-url-etcd-client-tls"),
							"VolumeSource": MatchFields(IgnoreExtras, Fields{
								"Secret": PointTo(MatchFields(IgnoreExtras, Fields{
									"SecretName": Equal(instance.Spec.Etcd.ClientUrlTLS.ClientTLSSecretRef.Name),
								})),
							}),
						}),
						"client-url-ca-etcd": MatchFields(IgnoreExtras, Fields{
							"Name": Equal("client-url-ca-etcd"),
							"VolumeSource": MatchFields(IgnoreExtras, Fields{
								"Secret": PointTo(MatchFields(IgnoreExtras, Fields{
									"SecretName": Equal(instance.Spec.Etcd.ClientUrlTLS.TLSCASecretRef.Name),
								})),
							}),
						}),
						"peer-url-etcd-server-tls": MatchFields(IgnoreExtras, Fields{
							"Name": Equal("peer-url-etcd-server-tls"),
							"VolumeSource": MatchFields(IgnoreExtras, Fields{
								"Secret": PointTo(MatchFields(IgnoreExtras, Fields{
									"SecretName": Equal(instance.Spec.Etcd.PeerUrlTLS.ServerTLSSecretRef.Name),
								})),
							}),
						}),
						"peer-url-ca-etcd": MatchFields(IgnoreExtras, Fields{
							"Name": Equal("peer-url-ca-etcd"),
							"VolumeSource": MatchFields(IgnoreExtras, Fields{
								"Secret": PointTo(MatchFields(IgnoreExtras, Fields{
									"SecretName": Equal(instance.Spec.Etcd.PeerUrlTLS.TLSCASecretRef.Name),
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

func validateStoreGCP(instance *druidv1alpha1.Etcd, s *appsv1.StatefulSet, cm *corev1.ConfigMap, clSvc *corev1.Service, prSvc *corev1.Service) {

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

func validateStoreAzure(instance *druidv1alpha1.Etcd, s *appsv1.StatefulSet, cm *corev1.ConfigMap, clSvc *corev1.Service, prSvc *corev1.Service) {
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

func validateStoreOpenstack(instance *druidv1alpha1.Etcd, s *appsv1.StatefulSet, cm *corev1.ConfigMap, clSvc *corev1.Service, prSvc *corev1.Service) {
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
								"OPENSTACK_APPLICATION_CREDENTIALS": MatchFields(IgnoreExtras, Fields{
									"Name":  Equal("OPENSTACK_APPLICATION_CREDENTIALS"),
									"Value": Equal("/root/etcd-backup"),
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

func validateStoreAlicloud(instance *druidv1alpha1.Etcd, s *appsv1.StatefulSet, cm *corev1.ConfigMap, clSvc *corev1.Service, prSvc *corev1.Service) {
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
								"ALICLOUD_APPLICATION_CREDENTIALS": MatchFields(IgnoreExtras, Fields{
									"Name":  Equal("ALICLOUD_APPLICATION_CREDENTIALS"),
									"Value": Equal("/root/etcd-backup"),
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

func validateStoreAWS(instance *druidv1alpha1.Etcd, s *appsv1.StatefulSet, cm *corev1.ConfigMap, clSvc *corev1.Service, prSvc *corev1.Service) {
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
								"AWS_APPLICATION_CREDENTIALS": MatchFields(IgnoreExtras, Fields{
									"Name":  Equal("AWS_APPLICATION_CREDENTIALS"),
									"Value": Equal("/root/etcd-backup"),
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

func etcdRemoved(c client.Client, etcd *druidv1alpha1.Etcd) error {
	ctx, cancel := context.WithTimeout(context.TODO(), timeout)
	defer cancel()
	e := &druidv1alpha1.Etcd{}
	req := types.NamespacedName{
		Name:      etcd.Name,
		Namespace: etcd.Namespace,
	}
	if err := c.Get(ctx, req, e); err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers
			return nil
		}
		return err
	}
	return fmt.Errorf("etcd not deleted")
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
		if apierrors.IsNotFound(err) {
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
		Name:      instance.Name,
		Namespace: instance.Namespace,
	}

	if err := c.Get(ctx, req, ss); err != nil {
		return err
	}
	if !checkEtcdOwnerReference(ss.GetOwnerReferences(), instance) {
		return fmt.Errorf("ownerReference does not exist")
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

func clientServiceIsCorrectlyReconciled(c client.Client, instance *druidv1alpha1.Etcd, svc *corev1.Service) error {
	ctx, cancel := context.WithTimeout(context.TODO(), timeout)
	defer cancel()
	req := types.NamespacedName{
		Name:      utils.GetClientServiceName(instance),
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

func peerServiceIsCorrectlyReconciled(c client.Client, instance *druidv1alpha1.Etcd, svc *corev1.Service) error {
	ctx, cancel := context.WithTimeout(context.TODO(), timeout)
	defer cancel()
	req := types.NamespacedName{
		Name:      utils.GetPeerServiceName(instance),
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

func serviceAccountIsCorrectlyReconciled(c client.Client, instance *druidv1alpha1.Etcd, sa *corev1.ServiceAccount) error {
	ctx, cancel := context.WithTimeout(context.TODO(), timeout)
	defer cancel()
	req := types.NamespacedName{
		Name:      instance.Name,
		Namespace: instance.Namespace,
	}

	if err := c.Get(ctx, req, sa); err != nil {
		return err
	}
	return nil
}

func roleIsCorrectlyReconciled(c client.Client, instance *druidv1alpha1.Etcd, role *rbac.Role) error {
	ctx, cancel := context.WithTimeout(context.TODO(), timeout)
	defer cancel()
	req := types.NamespacedName{
		Name:      fmt.Sprintf("druid.gardener.cloud:etcd:%s", instance.Name),
		Namespace: instance.Namespace,
	}

	if err := c.Get(ctx, req, role); err != nil {
		return err
	}
	return nil
}

func roleBindingIsCorrectlyReconciled(c client.Client, instance *druidv1alpha1.Etcd, rb *rbac.RoleBinding) error {
	ctx, cancel := context.WithTimeout(context.TODO(), timeout)
	defer cancel()
	req := types.NamespacedName{
		Name:      fmt.Sprintf("druid.gardener.cloud:etcd:%s", instance.Name),
		Namespace: instance.Namespace,
	}

	if err := c.Get(ctx, req, rb); err != nil {
		return err
	}
	return nil
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

func getEtcdWithDefault(name, namespace string) *druidv1alpha1.Etcd {
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
			Replicas: 1,
			Backup:   druidv1alpha1.BackupSpec{},
			Etcd:     druidv1alpha1.EtcdConfig{},
			Common:   druidv1alpha1.SharedConfig{},
		},
	}
	return instance
}

func getEtcd(name, namespace string, tlsEnabled bool) *druidv1alpha1.Etcd {

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
					Provider:  &provider,
					Prefix:    prefix,
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

func getEtcdWithReplicas(name, namespace string, replicas int) *druidv1alpha1.Etcd {
	instance := getEtcdWithDefault(name, namespace)
	instance.Spec.Replicas = int32(replicas)
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
	s.Status.ObservedGeneration = s.Generation

	replicas := int32(1)
	if s.Spec.Replicas != nil {
		replicas = *s.Spec.Replicas
	}
	s.Status.Replicas = replicas
	s.Status.ReadyReplicas = replicas
}

var _ = Describe("buildPredicate", func() {
	var (
		etcd                              *druidv1alpha1.Etcd
		evalCreate                        = func(p predicate.Predicate, obj client.Object) bool { return p.Create(event.CreateEvent{Object: obj}) }
		evalDelete                        = func(p predicate.Predicate, obj client.Object) bool { return p.Delete(event.DeleteEvent{Object: obj}) }
		evalGeneric                       = func(p predicate.Predicate, obj client.Object) bool { return p.Generic(event.GenericEvent{Object: obj}) }
		evalUpdateWithoutGenerationChange = func(p predicate.Predicate, obj client.Object) bool {
			return p.Update(event.UpdateEvent{ObjectOld: obj, ObjectNew: obj.DeepCopyObject().(client.Object)})
		}
		evalUpdateWithGenerationChange = func(p predicate.Predicate, obj client.Object) bool {
			objCopy := obj.DeepCopyObject().(client.Object)
			objCopy.SetGeneration(obj.GetGeneration() + 1)
			return p.Update(event.UpdateEvent{ObjectOld: obj, ObjectNew: objCopy})
		}
	)

	BeforeEach(func() {
		etcd = &druidv1alpha1.Etcd{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{},
			},
		}
	})

	DescribeTable(
		"with ignoreOperationAnnotation true",
		func(evalFn func(p predicate.Predicate, obj client.Object) bool, expect bool) {
			Expect(evalFn(buildPredicate(true), etcd)).To(Equal(expect))
		},
		Entry("Create should match", evalCreate, true),
		Entry("Delete should match", evalDelete, true),
		Entry("Generic should match", evalGeneric, true),
		Entry("Update without generation change should not match", evalUpdateWithoutGenerationChange, false),
		Entry("Update with generation change should match", evalUpdateWithGenerationChange, true),
	)

	Describe("with ignoreOperationAnnotation false", func() {
		DescribeTable(
			"without operation annotation or last error or deletion timestamp",
			func(evalFn func(p predicate.Predicate, obj client.Object) bool, expect bool) {
				Expect(evalFn(buildPredicate(false), etcd)).To(Equal(expect))
			},
			Entry("Create should not match", evalCreate, false),
			Entry("Delete should match", evalDelete, true),
			Entry("Generic should  not match", evalGeneric, false),
			Entry("Update without generation change should not match", evalUpdateWithoutGenerationChange, false),
			Entry("Update with generation change should not match", evalUpdateWithGenerationChange, false),
		)
		DescribeTable(
			"with operation annotation",
			func(evalFn func(p predicate.Predicate, obj client.Object) bool, expect bool) {
				etcd.Annotations[v1beta1constants.GardenerOperation] = v1beta1constants.GardenerOperationReconcile
				Expect(evalFn(buildPredicate(false), etcd)).To(Equal(expect))
			},
			Entry("Create should match", evalCreate, true),
			Entry("Delete should match", evalDelete, true),
			Entry("Generic should match", evalGeneric, true),
			Entry("Update without generation change should match", evalUpdateWithoutGenerationChange, true),
			Entry("Update with generation change should match", evalUpdateWithGenerationChange, true),
		)
		DescribeTable(
			"with last error",
			func(evalFn func(p predicate.Predicate, obj client.Object) bool, expect bool) {
				etcd.Status.LastError = pointer.StringPtr("error")
				Expect(evalFn(buildPredicate(false), etcd)).To(Equal(expect))
			},
			Entry("Create should match", evalCreate, true),
			Entry("Delete should match", evalDelete, true),
			Entry("Generic should match", evalGeneric, true),
			Entry("Update without generation change should match", evalUpdateWithoutGenerationChange, true),
			Entry("Update with generation change should match", evalUpdateWithGenerationChange, true),
		)
		DescribeTable(
			"with deletion timestamp",
			func(evalFn func(p predicate.Predicate, obj client.Object) bool, expect bool) {
				now := metav1.Time{Time: time.Now()}
				etcd.DeletionTimestamp = &now
				Expect(evalFn(buildPredicate(false), etcd)).To(Equal(expect))
			},
			Entry("Create should match", evalCreate, true),
			Entry("Delete should match", evalDelete, true),
			Entry("Generic should match", evalGeneric, true),
			Entry("Update without generation change should match", evalUpdateWithoutGenerationChange, true),
			Entry("Update with generation change should match", evalUpdateWithGenerationChange, true),
		)
	})
})
