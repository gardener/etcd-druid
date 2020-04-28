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

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
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

const timeout = time.Minute * 2
const pollingInterval = time.Second * 2

var (
	deltaSnapshotPeriod = metav1.Duration{
		Duration: 300 * time.Second,
	}
	garbageCollectionPeriod = metav1.Duration{
		Duration: 43200 * time.Second,
	}
	clientPort              = 2379
	serverPort              = 2380
	port                    = 8080
	imageEtcd               = "quay.io/coreos/etcd:v3.3.13"
	imageBR                 = "eu.gcr.io/gardener-project/gardener/etcdbrctl:0.8.0"
	snapshotSchedule        = "0 */24 * * *"
	defragSchedule          = "0 */24 * * *"
	container               = "default.bkp"
	storageCapacity         = resource.MustParse("5Gi")
	defaultStorageCapacity  = resource.MustParse("16Gi")
	storageClass            = "gardener.fast"
	priorityClassName       = "class_priority"
	deltaSnapShotMemLimit   = resource.MustParse("100Mi")
	quota                   = resource.MustParse("8Gi")
	provider                = druidv1alpha1.StorageProvider("Local")
	prefix                  = "/tmp"
	volumeClaimTemplateName = "etcd-main"
	garbageCollectionPolicy = druidv1alpha1.GarbageCollectionPolicy(druidv1alpha1.GarbageCollectionPolicyExponential)
)

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
		func(name string, setupStatefulSet StatefulSetInitializer, setupPod bool) {
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
		Entry("when statefulset not owned by etcd, druid should adopt the statefulset", "foo20", WithoutOwner, false),
		Entry("when statefulset owned by etcd with owner reference set, druid should remove ownerref and add annotations", "foo21", WithOwnerReference, false),
		Entry("when statefulset owned by etcd with owner annotations set, druid should persist the annotations", "foo22", WithOwnerAnnotation, false),
		Entry("when etcd has the spec changed, druid should reconcile statefulset", "foo3", WithoutOwner, false),
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
		func(name string, setupStatefulSet StatefulSetInitializer, setupPod bool) {
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

			storeSecret := instance.Spec.Backup.Store.SecretRef.Name
			errors := createSecrets(c, instance.Namespace, storeSecret)
			Expect(len(errors)).Should(BeZero())
			c.Create(context.TODO(), ss)
			defer WithWd("..")()
			err := c.Create(context.TODO(), instance)
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() error { return statefulsetIsCorrectlyReconciled(c, instance, &sts) }, timeout, pollingInterval).Should(BeNil())
			setStatefulSetReady(&sts)
			err = c.Status().Update(context.TODO(), &sts)
			Expect(err).NotTo(HaveOccurred())
			err = c.Delete(context.TODO(), instance)
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() error { return statefulSetRemoved(c, &sts) }, timeout, pollingInterval).Should(BeNil())
			Eventually(func() error { return etcdRemoved(c, instance) }, timeout, pollingInterval).Should(BeNil())
		},
		Entry("when  statefulset with ownerReference and without owner annotations, druid should adopt and delete statefulset", "foo61", WithOwnerReference, false),
		Entry("when statefulset without ownerReference and without owner annotations, druid should adopt and delete statefulset", "foo62", WithoutOwner, false),
		Entry("when  statefulset without ownerReference and with owner annotations, druid should adopt and delete statefulset", "foo63", WithOwnerAnnotation, false),
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

func getEtcdWithDefault(name, namespace string, tlsEnabled bool) *druidv1alpha1.Etcd {
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
			Backup: druidv1alpha1.BackupSpec{
				Image:                    &imageBR,
				Port:                     &port,
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
				Metrics:                 druidv1alpha1.Basic,
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
				Port:                     &port,
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
				Metrics:                 druidv1alpha1.Basic,
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
	s.Status.ObservedGeneration = s.Generation

	replicas := int32(1)
	if s.Spec.Replicas != nil {
		replicas = *s.Spec.Replicas
	}
	s.Status.Replicas = replicas
	s.Status.ReadyReplicas = replicas
}
