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

	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/gardener/gardener/pkg/utils/imagevector"
	"github.com/gardener/gardener/pkg/utils/test/matchers"
	"github.com/ghodss/yaml"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/pkg/common"
	"github.com/gardener/etcd-druid/pkg/utils"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	"github.com/gardener/gardener/pkg/utils/kubernetes/health"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/batch/v1"
	batchv1 "k8s.io/api/batch/v1beta1"
	corev1 "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

type StatefulSetInitializer int

const (
	WithoutOwner        StatefulSetInitializer = 0
	WithOwnerReference  StatefulSetInitializer = 1
	WithOwnerAnnotation StatefulSetInitializer = 2
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
	volumeClaimTemplateName       = "etcd-main"
	garbageCollectionPolicy       = druidv1alpha1.GarbageCollectionPolicy(druidv1alpha1.GarbageCollectionPolicyExponential)
	metricsBasic                  = druidv1alpha1.Basic
	maxBackups                    = 7
	imageNames                    = []string{
		common.Etcd,
		common.BackupRestore,
	}
	backupCompactionSchedule = "15 */24 * * *"
	etcdSnapshotTimeout      = metav1.Duration{
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
	return string(element.(string))
}

func ruleIterator(element interface{}) string {
	return string(element.(rbac.PolicyRule).APIGroups[0])
}

func stringArrayIterator(element interface{}) string {
	return string(element.(string))
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
					Name:      fmt.Sprintf("%s-client", instance.Name),
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
			}, timeout, pollingInterval).Should(Equal(pointer.Int32Ptr(int32(instance.Spec.Replicas))))
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
		Context("when adding etcd resources with statefulset already present", func() {
			var (
				instance *druidv1alpha1.Etcd
				sts      *appsv1.StatefulSet
				c        client.Client
			)

			BeforeEach(func() {
				ctx, cancel := context.WithTimeout(context.TODO(), timeout)
				defer cancel()

				instance = getEtcd("foo81", "default", false)
				c = mgr.GetClient()

				// Create StatefulSet
				sts = createStatefulset(instance.Name, instance.Namespace, instance.Spec.Labels)
				Expect(c.Create(ctx, sts)).To(Succeed())

				Eventually(func() error { return c.Get(ctx, client.ObjectKeyFromObject(instance), sts) }, timeout, pollingInterval).Should(Succeed())

				sts.Status.Replicas = 1
				sts.Status.ReadyReplicas = 1
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

				// Create ETCD instance
				storeSecret := instance.Spec.Backup.Store.SecretRef.Name
				errors := createSecrets(c, instance.Namespace, storeSecret)
				Expect(len(errors)).Should(BeZero())
				Expect(c.Create(ctx, instance)).To(Succeed())

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

	DescribeTable("when adding etcd resources with statefulset already present",
		func(name string, setupStatefulSet StatefulSetInitializer) {
			var sts *appsv1.StatefulSet
			instance := getEtcd(name, "default", false)
			c := mgr.GetClient()

			switch setupStatefulSet {
			case WithoutOwner:
				sts = createStatefulset(name, "default", instance.Spec.Labels)
				Expect(sts.OwnerReferences).Should(BeNil())
			case WithOwnerReference:
				sts = createStatefulsetWithOwnerReference(instance)
				Expect(len(sts.OwnerReferences)).Should(Equal(1))
			case WithOwnerAnnotation:
				sts = createStatefulsetWithOwnerAnnotations(instance)
			default:
				Fail("StatefulSetInitializer invalid")
			}
			needsOwnerRefUpdate := len(sts.OwnerReferences) > 0
			storeSecret := instance.Spec.Backup.Store.SecretRef.Name

			// Create Secrets
			errors := createSecrets(c, instance.Namespace, storeSecret)
			Expect(len(errors)).Should(BeZero())

			// Create StatefulSet
			Expect(c.Create(context.TODO(), sts)).To(Succeed())

			// Create StatefulSet
			Expect(c.Create(context.TODO(), instance)).To(Succeed())

			// Update OwnerRef with UID from just created `etcd` instance
			if needsOwnerRefUpdate {
				Eventually(func() error {
					if err := c.Get(context.TODO(), client.ObjectKeyFromObject(instance), instance); err != nil {
						return err
					}
					if instance.UID != "" {
						instance.TypeMeta = metav1.TypeMeta{
							APIVersion: "druid.gardener.cloud/v1alpha1",
							Kind:       "etcd",
						}
						return nil
					}
					return fmt.Errorf("etcd object not yet created")
				}, timeout, pollingInterval).Should(BeNil())
				sts.OwnerReferences[0].UID = instance.UID
				Expect(c.Update(context.TODO(), sts)).To(Succeed())
			}

			sts = &appsv1.StatefulSet{}
			Eventually(func() error { return statefulsetIsCorrectlyReconciled(c, instance, sts) }, timeout, pollingInterval).Should(BeNil())
			setStatefulSetReady(sts)
			Expect(c.Status().Update(context.TODO(), sts)).To(Succeed())
			Expect(c.Delete(context.TODO(), instance)).To(Succeed())
		},
		Entry("when statefulset not owned by etcd, druid should adopt the statefulset", "foo20", WithoutOwner),
		Entry("when statefulset owned by etcd with owner reference set, druid should remove ownerref and add annotations", "foo21", WithOwnerReference),
		Entry("when statefulset owned by etcd with owner annotations set, druid should persist the annotations", "foo22", WithOwnerAnnotation),
		Entry("when etcd has the spec changed, druid should reconcile statefulset", "foo3", WithoutOwner),
	)

	Describe("when adding etcd resources with statefulset already present", func() {
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
				Expect(c.Create(context.TODO(), p)).To(Succeed())
				Expect(c.Create(context.TODO(), ss)).To(Succeed())
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
				Expect(c.Create(context.TODO(), instance)).To(Succeed())
				Eventually(func() error { return podDeleted(c, instance) }, timeout, pollingInterval).Should(BeNil())
			})
			AfterEach(func() {
				s := &appsv1.StatefulSet{}
				Eventually(func() error { return statefulsetIsCorrectlyReconciled(c, instance, s) }, timeout, pollingInterval).Should(BeNil())
				setStatefulSetReady(s)
				Expect(c.Status().Update(context.TODO(), s)).To(Succeed())
				Expect(c.Delete(context.TODO(), instance)).To(Succeed())
			})
		})
	})

	DescribeTable("when deleting etcd resources",
		func(name string, setupStatefulSet StatefulSetInitializer) {
			var sts *appsv1.StatefulSet
			instance := getEtcd(name, "default", false)
			c := mgr.GetClient()

			switch setupStatefulSet {
			case WithoutOwner:
				sts = createStatefulset(name, "default", instance.Spec.Labels)
				Expect(sts.OwnerReferences).Should(BeNil())
			case WithOwnerReference:
				sts = createStatefulsetWithOwnerReference(instance)
				Expect(len(sts.OwnerReferences)).ShouldNot(BeZero())
			case WithOwnerAnnotation:
				sts = createStatefulsetWithOwnerAnnotations(instance)
			default:
				Fail("StatefulSetInitializer invalid")
			}
			stopCh := make(chan struct{})
			storeSecret := instance.Spec.Backup.Store.SecretRef.Name
			needsOwnerRefUpdate := len(sts.OwnerReferences) > 0

			// Create Secrets
			errors := createSecrets(c, instance.Namespace, storeSecret)
			Expect(len(errors)).Should(BeZero())

			// Create StatefulSet
			Expect(c.Create(context.TODO(), sts)).To(Succeed())

			// Create StatefulSet
			Expect(c.Create(context.TODO(), instance)).To(Succeed())

			// Update OwnerRef with UID from just created `etcd` instance
			if needsOwnerRefUpdate {
				Eventually(func() error {
					if err := c.Get(context.TODO(), client.ObjectKeyFromObject(instance), instance); err != nil {
						return err
					}
					if instance.UID != "" {
						instance.TypeMeta = metav1.TypeMeta{
							APIVersion: "druid.gardener.cloud/v1alpha1",
							Kind:       "etcd",
						}
						return nil
					}
					return fmt.Errorf("etcd object not yet created")
				}, timeout, pollingInterval).Should(BeNil())
				sts.OwnerReferences[0].UID = instance.UID
				Expect(c.Update(context.TODO(), sts)).To(Succeed())
			}

			sts = &appsv1.StatefulSet{}
			Eventually(func() error { return statefulsetIsCorrectlyReconciled(c, instance, sts) }, timeout, pollingInterval).Should(BeNil())
			// This go-routine is to set that statefulset is ready manually as statefulset controller is absent for tests.
			go func() {
				for {
					select {
					case <-time.After(time.Second * 2):
						Expect(setAndCheckStatefulSetReady(c, instance)).To(Succeed())
					case <-stopCh:
						return
					}
				}
			}()
			Eventually(func() error { return isEtcdReady(c, instance) }, timeout, pollingInterval).Should(BeNil())
			close(stopCh)
			Expect(c.Delete(context.TODO(), instance)).To(Succeed())
			Eventually(func() error { return statefulSetRemoved(c, sts) }, timeout, pollingInterval).Should(BeNil())
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
			svc = &corev1.Service{}
			Eventually(func() error { return serviceIsCorrectlyReconciled(c, instance, svc) }, timeout, pollingInterval).Should(BeNil())
			sa = &corev1.ServiceAccount{}
			Eventually(func() error { return serviceAccountIsCorrectlyReconciled(c, instance, sa) }, timeout, pollingInterval).Should(BeNil())
			role = &rbac.Role{}
			Eventually(func() error { return roleIsCorrectlyReconciled(c, instance, role) }, timeout, pollingInterval).Should(BeNil())
			rb = &rbac.RoleBinding{}
			Eventually(func() error { return roleBindingIsCorrectlyReconciled(c, instance, rb) }, timeout, pollingInterval).Should(BeNil())

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

	DescribeTable("when etcd resource is created with backupCompactionSchedule field",
		func(name string, generateEtcd func(string, string) *druidv1alpha1.Etcd, validate func(*appsv1.StatefulSet, *corev1.ConfigMap, *corev1.Service, *batchv1.CronJob, *druidv1alpha1.Etcd)) {
			var err error
			var instance *druidv1alpha1.Etcd
			var c client.Client
			var s *appsv1.StatefulSet
			var cm *corev1.ConfigMap
			var svc *corev1.Service
			var cj *batchv1.CronJob
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
			svc = &corev1.Service{}
			Eventually(func() error { return serviceIsCorrectlyReconciled(c, instance, svc) }, timeout, pollingInterval).Should(BeNil())
			cj = &batchv1.CronJob{}
			Eventually(func() error { return cronJobIsCorrectlyReconciled(c, instance, cj) }, timeout, pollingInterval).Should(BeNil())
			sa = &corev1.ServiceAccount{}
			Eventually(func() error { return serviceAccountIsCorrectlyReconciled(c, instance, sa) }, timeout, pollingInterval).Should(BeNil())
			role = &rbac.Role{}
			Eventually(func() error { return roleIsCorrectlyReconciled(c, instance, role) }, timeout, pollingInterval).Should(BeNil())
			rb = &rbac.RoleBinding{}
			Eventually(func() error { return roleBindingIsCorrectlyReconciled(c, instance, rb) }, timeout, pollingInterval).Should(BeNil())

			//validate(s, cm, svc, instance)
			validate(s, cm, svc, cj, instance)
			validateRole(role, instance)

			setStatefulSetReady(s)
			err = c.Status().Update(context.TODO(), s)
			Expect(err).NotTo(HaveOccurred())

			Expect(c.Delete(context.TODO(), instance)).To(Succeed())
			Eventually(func() error { return statefulSetRemoved(c, s) }, timeout, pollingInterval).Should(BeNil())
			Eventually(func() error { return etcdRemoved(c, instance) }, timeout, pollingInterval).Should(BeNil())
		},
		Entry("if fields are set in etcd.Spec and TLS enabled, the resources should reflect the spec changes", "foo42", getEtcdWithCmpctScheduleTLS, validateEtcdWithCronjob),
		Entry("if the store is GCS, the statefulset and cronjob should reflect the spec changes", "foo43", getEtcdWithCmpctScheduleGCS, validateStoreGCPWithCronjob),
		Entry("if the store is S3, the statefulset and cronjob should reflect the spec changes", "foo44", getEtcdWithCmpctScheduleS3, validateStoreAWSWithCronjob),
		Entry("if the store is ABS, the statefulset and cronjob should reflect the spec changes", "foo45", getEtcdWithCmpctScheduleABS, validateStoreAzureWithCronjob),
		Entry("if the store is Swift, the statefulset and cronjob should reflect the spec changes", "foo46", getEtcdWithCmpctScheduleSwift, validateStoreOpenstackWithCronjob),
		Entry("if the store is OSS, the statefulset and cronjob should reflect the spec changes", "foo47", getEtcdWithCmpctScheduleOSS, validateStoreAlicloudWithCronjob),
	)

	Describe("with etcd resources without backupCompactionScheduled field", func() {
		Context("when creating an etcd object", func() {
			It("should not create a cronjob", func() {
				var err error
				var instance *druidv1alpha1.Etcd
				var c client.Client
				var s *appsv1.StatefulSet
				var cm *corev1.ConfigMap
				var svc *corev1.Service
				var cj *batchv1.CronJob

				instance = getEtcd("foo48", "default", true)
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
				svc = &corev1.Service{}
				Eventually(func() error { return serviceIsCorrectlyReconciled(c, instance, svc) }, timeout, pollingInterval).Should(BeNil())
				cj = &batchv1.CronJob{}
				Eventually(func() error { return cronJobIsCorrectlyReconciled(c, instance, cj) }, timeout, pollingInterval).ShouldNot(BeNil())

				setStatefulSetReady(s)
				err = c.Status().Update(context.TODO(), s)
				Expect(err).NotTo(HaveOccurred())

				Expect(c.Delete(context.TODO(), instance)).To(Succeed())
				Eventually(func() error { return statefulSetRemoved(c, s) }, timeout, pollingInterval).Should(BeNil())
				Eventually(func() error { return etcdRemoved(c, instance) }, timeout, pollingInterval).Should(BeNil())
			})
		})

		Context("when an existing cronjob is already present", func() {
			It("should delete the existing cronjob", func() {
				var err error
				var instance *druidv1alpha1.Etcd
				var c client.Client
				var s *appsv1.StatefulSet
				var cm *corev1.ConfigMap
				var svc *corev1.Service
				var cj *batchv1.CronJob

				ctx, cancel := context.WithTimeout(context.TODO(), timeout)
				defer cancel()

				instance = getEtcd("foo49", "default", true)
				c = mgr.GetClient()
				ns := corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: instance.Namespace,
					},
				}

				// Create CronJob
				cj = createCronJob(getCronJobName(instance), instance.Namespace, instance.Spec.Labels)
				Expect(c.Create(ctx, cj)).To(Succeed())
				Eventually(func() error { return cronJobIsCorrectlyReconciled(c, instance, cj) }, timeout, pollingInterval).Should(BeNil())

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
				svc = &corev1.Service{}
				Eventually(func() error { return serviceIsCorrectlyReconciled(c, instance, svc) }, timeout, pollingInterval).Should(BeNil())
				//Cronjob should not exist
				cj = &batchv1.CronJob{}
				Eventually(func() error { return cronJobRemoved(c, cj) }, timeout, pollingInterval).Should(BeNil())

				setStatefulSetReady(s)
				err = c.Status().Update(context.TODO(), s)
				Expect(err).NotTo(HaveOccurred())

				Expect(c.Delete(context.TODO(), instance)).To(Succeed())
				Eventually(func() error { return statefulSetRemoved(c, s) }, timeout, pollingInterval).Should(BeNil())
				Eventually(func() error { return etcdRemoved(c, instance) }, timeout, pollingInterval).Should(BeNil())
			})
		})
	})

})

func validateRole(role *rbac.Role, instance *druidv1alpha1.Etcd) {
	Expect(*role).To(MatchFields(IgnoreExtras, Fields{
		"ObjectMeta": MatchFields(IgnoreExtras, Fields{
			"Name":      Equal(fmt.Sprintf("%s-br-role", instance.Name)),
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
		}),
	}))
}

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

func validateEtcdWithCronjob(s *appsv1.StatefulSet, cm *corev1.ConfigMap, svc *corev1.Service, cj *batchv1.CronJob, instance *druidv1alpha1.Etcd) {
	validateEtcd(s, cm, svc, instance)

	store, err := utils.StorageProviderFromInfraProvider(instance.Spec.Backup.Store.Provider)
	Expect(err).NotTo(HaveOccurred())

	Expect(*cj).To(MatchFields(IgnoreExtras, Fields{
		"ObjectMeta": MatchFields(IgnoreExtras, Fields{
			"Name":      Equal(getCronJobName(instance)),
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
		"Spec": MatchFields(IgnoreExtras, Fields{
			"ConcurrencyPolicy": Equal(batchv1.ForbidConcurrent),
			"JobTemplate": MatchFields(IgnoreExtras, Fields{
				"Spec": MatchFields(IgnoreExtras, Fields{
					"BackoffLimit": PointTo(Equal(int32(0))),
					"Template": MatchFields(IgnoreExtras, Fields{
						"Spec": MatchFields(IgnoreExtras, Fields{
							"RestartPolicy": Equal(corev1.RestartPolicyNever),
							"Containers": MatchElements(containerIterator, IgnoreExtras, Elements{
								"compact-backup": MatchFields(IgnoreExtras, Fields{
									"Command": MatchElements(cmdIterator, IgnoreExtras, Elements{
										"--data-dir=/var/etcd/data":                                                                                 Equal("--data-dir=/var/etcd/data"),
										"--snapstore-temp-directory=/var/etcd/data/tmp":                                                             Equal("--snapstore-temp-directory=/var/etcd/data/tmp"),
										fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix):                                   Equal(fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix)),
										fmt.Sprintf("%s=%s", "--storage-provider", store):                                                           Equal(fmt.Sprintf("%s=%s", "--storage-provider", store)),
										fmt.Sprintf("%s=%s", "--store-container", *instance.Spec.Backup.Store.Container):                            Equal(fmt.Sprintf("%s=%s", "--store-container", *instance.Spec.Backup.Store.Container)),
										fmt.Sprintf("--embedded-etcd-quota-bytes=%d", int64(instance.Spec.Etcd.Quota.Value())):                      Equal(fmt.Sprintf("--embedded-etcd-quota-bytes=%d", int64(instance.Spec.Etcd.Quota.Value()))),
										fmt.Sprintf("%s=%s", "--etcd-snapshot-timeout", instance.Spec.Backup.EtcdSnapshotTimeout.Duration.String()): Equal(fmt.Sprintf("%s=%s", "--etcd-snapshot-timeout", instance.Spec.Backup.EtcdSnapshotTimeout.Duration.String())),
										fmt.Sprintf("%s=%s", "--etcd-defrag-timeout", instance.Spec.Etcd.EtcdDefragTimeout.Duration.String()):       Equal(fmt.Sprintf("%s=%s", "--etcd-defrag-timeout", instance.Spec.Etcd.EtcdDefragTimeout.Duration.String())),
									}),
									"Ports": ConsistOf([]corev1.ContainerPort{
										corev1.ContainerPort{
											Name:          "server",
											Protocol:      corev1.ProtocolTCP,
											HostPort:      0,
											ContainerPort: backupPort,
										},
									}),
									//"Image":           Equal(*instance.Spec.Backup.Image),
									"ImagePullPolicy": Equal(corev1.PullIfNotPresent),
									"VolumeMounts": MatchElements(volumeMountIterator, IgnoreExtras, Elements{
										"etcd-config-file": MatchFields(IgnoreExtras, Fields{
											"Name":      Equal("etcd-config-file"),
											"MountPath": Equal("/var/etcd/config/"),
										}),
										"etcd-workspace-dir": MatchFields(IgnoreExtras, Fields{
											"Name":      Equal("etcd-workspace-dir"),
											"MountPath": Equal("/var/etcd/data"),
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
								"etcd-workspace-dir": MatchFields(IgnoreExtras, Fields{
									"Name": Equal("etcd-workspace-dir"),
									"VolumeSource": MatchFields(IgnoreExtras, Fields{
										"HostPath": BeNil(),
										"EmptyDir": PointTo(MatchFields(IgnoreExtras, Fields{
											"SizeLimit": BeNil(),
										})),
									}),
								}),
							}),
						}),
					}),
				}),
			}),
		}),
	}))
}

func validateStoreGCPWithCronjob(s *appsv1.StatefulSet, cm *corev1.ConfigMap, svc *corev1.Service, cj *batchv1.CronJob, instance *druidv1alpha1.Etcd) {
	validateStoreGCP(s, cm, svc, instance)

	Expect(*cj).To(MatchFields(IgnoreExtras, Fields{
		"Spec": MatchFields(IgnoreExtras, Fields{
			"JobTemplate": MatchFields(IgnoreExtras, Fields{
				"Spec": MatchFields(IgnoreExtras, Fields{
					"Template": MatchFields(IgnoreExtras, Fields{
						"Spec": MatchFields(IgnoreExtras, Fields{
							"Containers": MatchElements(containerIterator, IgnoreExtras, Elements{
								"compact-backup": MatchFields(IgnoreExtras, Fields{
									"Command": MatchElements(cmdIterator, IgnoreExtras, Elements{
										"--storage-provider=GCS": Equal("--storage-provider=GCS"),
										fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix):        Equal(fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix)),
										fmt.Sprintf("%s=%s", "--store-container", *instance.Spec.Backup.Store.Container): Equal(fmt.Sprintf("%s=%s", "--store-container", *instance.Spec.Backup.Store.Container)),
									}),
									"VolumeMounts": MatchElements(volumeMountIterator, IgnoreExtras, Elements{
										"etcd-backup": MatchFields(IgnoreExtras, Fields{
											"Name":      Equal("etcd-backup"),
											"MountPath": Equal("/root/.gcp/"),
										}),
										"etcd-config-file": MatchFields(IgnoreExtras, Fields{
											"Name":      Equal("etcd-config-file"),
											"MountPath": Equal("/var/etcd/config/"),
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
				}),
			}),
		}),
	}))
}

func validateStoreAWSWithCronjob(s *appsv1.StatefulSet, cm *corev1.ConfigMap, svc *corev1.Service, cj *batchv1.CronJob, instance *druidv1alpha1.Etcd) {
	validateStoreAWS(s, cm, svc, instance)

	Expect(*cj).To(MatchFields(IgnoreExtras, Fields{
		"Spec": MatchFields(IgnoreExtras, Fields{
			"JobTemplate": MatchFields(IgnoreExtras, Fields{
				"Spec": MatchFields(IgnoreExtras, Fields{
					"Template": MatchFields(IgnoreExtras, Fields{
						"Spec": MatchFields(IgnoreExtras, Fields{
							"Containers": MatchElements(containerIterator, IgnoreExtras, Elements{
								"compact-backup": MatchFields(IgnoreExtras, Fields{
									"Command": MatchElements(cmdIterator, IgnoreExtras, Elements{
										"--storage-provider=S3": Equal("--storage-provider=S3"),
										fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix):        Equal(fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix)),
										fmt.Sprintf("%s=%s", "--store-container", *instance.Spec.Backup.Store.Container): Equal(fmt.Sprintf("%s=%s", "--store-container", *instance.Spec.Backup.Store.Container)),
									}),
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
			}),
		}),
	}))
}

func validateStoreAzureWithCronjob(s *appsv1.StatefulSet, cm *corev1.ConfigMap, svc *corev1.Service, cj *batchv1.CronJob, instance *druidv1alpha1.Etcd) {
	validateStoreAzure(s, cm, svc, instance)

	Expect(*cj).To(MatchFields(IgnoreExtras, Fields{
		"Spec": MatchFields(IgnoreExtras, Fields{
			"JobTemplate": MatchFields(IgnoreExtras, Fields{
				"Spec": MatchFields(IgnoreExtras, Fields{
					"Template": MatchFields(IgnoreExtras, Fields{
						"Spec": MatchFields(IgnoreExtras, Fields{
							"Containers": MatchElements(containerIterator, IgnoreExtras, Elements{
								"compact-backup": MatchFields(IgnoreExtras, Fields{
									"Command": MatchElements(cmdIterator, IgnoreExtras, Elements{
										"--storage-provider=ABS": Equal("--storage-provider=ABS"),
										fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix):        Equal(fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix)),
										fmt.Sprintf("%s=%s", "--store-container", *instance.Spec.Backup.Store.Container): Equal(fmt.Sprintf("%s=%s", "--store-container", *instance.Spec.Backup.Store.Container)),
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
			}),
		}),
	}))
}

func validateStoreOpenstackWithCronjob(s *appsv1.StatefulSet, cm *corev1.ConfigMap, svc *corev1.Service, cj *batchv1.CronJob, instance *druidv1alpha1.Etcd) {
	validateStoreOpenstack(s, cm, svc, instance)

	Expect(*cj).To(MatchFields(IgnoreExtras, Fields{
		"Spec": MatchFields(IgnoreExtras, Fields{
			"JobTemplate": MatchFields(IgnoreExtras, Fields{
				"Spec": MatchFields(IgnoreExtras, Fields{
					"Template": MatchFields(IgnoreExtras, Fields{
						"Spec": MatchFields(IgnoreExtras, Fields{
							"Containers": MatchElements(containerIterator, IgnoreExtras, Elements{
								"compact-backup": MatchFields(IgnoreExtras, Fields{
									"Command": MatchElements(cmdIterator, IgnoreExtras, Elements{
										"--storage-provider=Swift": Equal("--storage-provider=Swift"),
										fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix):        Equal(fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix)),
										fmt.Sprintf("%s=%s", "--store-container", *instance.Spec.Backup.Store.Container): Equal(fmt.Sprintf("%s=%s", "--store-container", *instance.Spec.Backup.Store.Container)),
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
			}),
		}),
	}))
}

func validateStoreAlicloudWithCronjob(s *appsv1.StatefulSet, cm *corev1.ConfigMap, svc *corev1.Service, cj *batchv1.CronJob, instance *druidv1alpha1.Etcd) {
	validateStoreAlicloud(s, cm, svc, instance)

	Expect(*cj).To(MatchFields(IgnoreExtras, Fields{
		"Spec": MatchFields(IgnoreExtras, Fields{
			"JobTemplate": MatchFields(IgnoreExtras, Fields{
				"Spec": MatchFields(IgnoreExtras, Fields{
					"Template": MatchFields(IgnoreExtras, Fields{
						"Spec": MatchFields(IgnoreExtras, Fields{
							"Containers": MatchElements(containerIterator, IgnoreExtras, Elements{
								"compact-backup": MatchFields(IgnoreExtras, Fields{
									"Command": MatchElements(cmdIterator, IgnoreExtras, Elements{
										"--storage-provider=OSS": Equal("--storage-provider=OSS"),
										fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix):        Equal(fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix)),
										fmt.Sprintf("%s=%s", "--store-container", *instance.Spec.Backup.Store.Container): Equal(fmt.Sprintf("%s=%s", "--store-container", *instance.Spec.Backup.Store.Container)),
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
			}),
		}),
	}))
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
	//	  limits:
	//		cpu: 100m
	//		memory: 512Gi
	//	  requests:
	//		cpu: 50m
	//		memory: 128Mi
	Expect(instance.Spec.Etcd.Resources).To(BeNil())

	// Validate TLS. Ensure that enableTLS flag is not triggered in the go-template
	Expect(instance.Spec.Etcd.TLS).To(BeNil())

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
		"auto-compaction-mode":      Equal(string(druidv1alpha1.Periodic)),
		"auto-compaction-retention": Equal(DefaultAutoCompactionRetention),
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
								"--etcd-process-name=etcd":                       Equal("--etcd-process-name=etcd"),

								fmt.Sprintf("--delta-snapshot-memory-limit=%d", deltaSnapShotMemLimit.Value()):                 Equal(fmt.Sprintf("--delta-snapshot-memory-limit=%d", deltaSnapShotMemLimit.Value())),
								fmt.Sprintf("--garbage-collection-policy=%s", druidv1alpha1.GarbageCollectionPolicyLimitBased): Equal(fmt.Sprintf("--garbage-collection-policy=%s", druidv1alpha1.GarbageCollectionPolicyLimitBased)),
								fmt.Sprintf("--endpoints=http://%s-local:%d", instance.Name, clientPort):                       Equal(fmt.Sprintf("--endpoints=http://%s-local:%d", instance.Name, clientPort)),
								fmt.Sprintf("--embedded-etcd-quota-bytes=%d", int64(quota.Value())):                            Equal(fmt.Sprintf("--embedded-etcd-quota-bytes=%d", int64(quota.Value()))),
								fmt.Sprintf("--max-backups=%d", maxBackups):                                                    Equal(fmt.Sprintf("--max-backups=%d", maxBackups)),
								fmt.Sprintf("--auto-compaction-mode=%s", druidv1alpha1.Periodic):                               Equal(fmt.Sprintf("--auto-compaction-mode=%s", druidv1alpha1.Periodic)),
								fmt.Sprintf("--auto-compaction-retention=%s", DefaultAutoCompactionRetention):                  Equal(fmt.Sprintf("--auto-compaction-retention=%s", DefaultAutoCompactionRetention)),
								fmt.Sprintf("%s=%s", "--etcd-snapshot-timeout", "8m"):                                          Equal(fmt.Sprintf("%s=%s", "--etcd-snapshot-timeout", "8m")),
								fmt.Sprintf("%s=%s", "--etcd-defrag-timeout", "8m"):                                            Equal(fmt.Sprintf("%s=%s", "--etcd-defrag-timeout", "8m")),
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
		"name":                      Equal(fmt.Sprintf("etcd-%s", instance.UID[:6])),
		"data-dir":                  Equal("/var/etcd/data/new.etcd"),
		"metrics":                   Equal(string(*instance.Spec.Etcd.Metrics)),
		"snapshot-count":            Equal(float64(75000)),
		"enable-v2":                 Equal(false),
		"quota-backend-bytes":       Equal(float64(instance.Spec.Etcd.Quota.Value())),
		"listen-client-urls":        Equal(fmt.Sprintf("https://0.0.0.0:%d", *instance.Spec.Etcd.ClientPort)),
		"advertise-client-urls":     Equal(fmt.Sprintf("https://0.0.0.0:%d", *instance.Spec.Etcd.ClientPort)),
		"initial-cluster-token":     Equal("initial"),
		"initial-cluster-state":     Equal("new"),
		"auto-compaction-mode":      Equal(string(*instance.Spec.Common.AutoCompactionMode)),
		"auto-compaction-retention": Equal(*instance.Spec.Common.AutoCompactionRetention),

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
								"--etcd-process-name=etcd":                       Equal("--etcd-process-name=etcd"),
								"--etcd-connection-timeout=5m":                   Equal("--etcd-connection-timeout=5m"),
								fmt.Sprintf("--defragmentation-schedule=%s", *instance.Spec.Etcd.DefragmentationSchedule):                           Equal(fmt.Sprintf("--defragmentation-schedule=%s", *instance.Spec.Etcd.DefragmentationSchedule)),
								fmt.Sprintf("--schedule=%s", *instance.Spec.Backup.FullSnapshotSchedule):                                            Equal(fmt.Sprintf("--schedule=%s", *instance.Spec.Backup.FullSnapshotSchedule)),
								fmt.Sprintf("%s=%s", "--garbage-collection-policy", *instance.Spec.Backup.GarbageCollectionPolicy):                  Equal(fmt.Sprintf("%s=%s", "--garbage-collection-policy", *instance.Spec.Backup.GarbageCollectionPolicy)),
								fmt.Sprintf("%s=%s", "--storage-provider", store):                                                                   Equal(fmt.Sprintf("%s=%s", "--storage-provider", store)),
								fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix):                                           Equal(fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix)),
								fmt.Sprintf("--delta-snapshot-memory-limit=%d", instance.Spec.Backup.DeltaSnapshotMemoryLimit.Value()):              Equal(fmt.Sprintf("--delta-snapshot-memory-limit=%d", instance.Spec.Backup.DeltaSnapshotMemoryLimit.Value())),
								fmt.Sprintf("--garbage-collection-policy=%s", *instance.Spec.Backup.GarbageCollectionPolicy):                        Equal(fmt.Sprintf("--garbage-collection-policy=%s", *instance.Spec.Backup.GarbageCollectionPolicy)),
								fmt.Sprintf("--endpoints=https://%s-local:%d", instance.Name, clientPort):                                           Equal(fmt.Sprintf("--endpoints=https://%s-local:%d", instance.Name, clientPort)),
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
							}),
							"Ports": ConsistOf([]corev1.ContainerPort{
								corev1.ContainerPort{
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

func cronJobRemoved(c client.Client, cj *batchv1.CronJob) error {
	ctx, cancel := context.WithTimeout(context.TODO(), timeout)
	defer cancel()
	cronjob := &batchv1.CronJob{}
	req := types.NamespacedName{
		Name:      cj.Name,
		Namespace: cj.Namespace,
	}
	if err := c.Get(ctx, req, cronjob); err != nil {
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
		Name:      instance.Name,
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

func cronJobIsCorrectlyReconciled(c client.Client, instance *druidv1alpha1.Etcd, cj *batchv1.CronJob) error {
	ctx, cancel := context.WithTimeout(context.TODO(), timeout)
	defer cancel()
	req := types.NamespacedName{
		Name:      getCronJobName(instance),
		Namespace: instance.Namespace,
	}

	if err := c.Get(ctx, req, cj); err != nil {
		return err
	}
	return nil
}

func createCronJob(name, namespace string, labels map[string]string) *batchv1.CronJob {
	cj := batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: batchv1.CronJobSpec{
			Schedule:          backupCompactionSchedule,
			ConcurrencyPolicy: "Forbid",
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: v1.JobSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: labels,
						},
						Spec: corev1.PodSpec{
							RestartPolicy: "Never",
							Containers: []corev1.Container{
								{
									Name:  "compact-backup",
									Image: "eu.gcr.io/gardener-project/gardener/etcdbrctl:v0.12.0",
								},
							},
						},
					},
				},
			},
		},
	}
	return &cj
}

func serviceAccountIsCorrectlyReconciled(c client.Client, instance *druidv1alpha1.Etcd, sa *corev1.ServiceAccount) error {
	ctx, cancel := context.WithTimeout(context.TODO(), timeout)
	defer cancel()
	req := types.NamespacedName{
		Name:      fmt.Sprintf("%s-br-serviceaccount", instance.Name),
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
		Name:      fmt.Sprintf("%s-br-role", instance.Name),
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
		Name:      fmt.Sprintf("%s-br-rolebinding", instance.Name),
		Namespace: instance.Namespace,
	}

	if err := c.Get(ctx, req, rb); err != nil {
		return err
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
							Image: "eu.gcr.io/gardener-project/gardener/etcd:v3.4.13-bootstrap",
						},
						{
							Name:  "backup-restore",
							Image: "eu.gcr.io/gardener-project/gardener/etcdbrctl:v0.12.0",
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
			APIVersion:         "druid.gardener.cloud/v1alpha1",
			Kind:               "etcd",
			Name:               etcd.Name,
			UID:                "foo",
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
					Image: "eu.gcr.io/gardener-project/gardener/etcd:v3.4.13-bootstrap",
				},
				{
					Name:  "backup-restore",
					Image: "eu.gcr.io/gardener-project/gardener/etcdbrctl:v0.12.0",
				},
			},
		},
	}
	return &pod
}

func getEtcdWithCmpctScheduleTLS(name, namespace string) *druidv1alpha1.Etcd {
	etcd := getEtcdWithTLS(name, namespace)
	etcd.Spec.Backup.BackupCompactionSchedule = &backupCompactionSchedule
	return etcd
}

func getEtcdWithCmpctScheduleGCS(name, namespace string) *druidv1alpha1.Etcd {
	etcd := getEtcdWithGCS(name, namespace)
	etcd.Spec.Backup.BackupCompactionSchedule = &backupCompactionSchedule
	return etcd
}

func getEtcdWithCmpctScheduleS3(name, namespace string) *druidv1alpha1.Etcd {
	etcd := getEtcdWithS3(name, namespace)
	etcd.Spec.Backup.BackupCompactionSchedule = &backupCompactionSchedule
	return etcd
}

func getEtcdWithCmpctScheduleABS(name, namespace string) *druidv1alpha1.Etcd {
	etcd := getEtcdWithABS(name, namespace)
	etcd.Spec.Backup.BackupCompactionSchedule = &backupCompactionSchedule
	return etcd
}

func getEtcdWithCmpctScheduleSwift(name, namespace string) *druidv1alpha1.Etcd {
	etcd := getEtcdWithSwift(name, namespace)
	etcd.Spec.Backup.BackupCompactionSchedule = &backupCompactionSchedule
	return etcd
}

func getEtcdWithCmpctScheduleOSS(name, namespace string) *druidv1alpha1.Etcd {
	etcd := getEtcdWithOSS(name, namespace)
	etcd.Spec.Backup.BackupCompactionSchedule = &backupCompactionSchedule
	return etcd
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
