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

package controllers_test

import (
	"context"
	"fmt"
	"os"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const timeout = time.Minute * 2

var _ = Describe("Druid", func() {

	Context("when adding etcd resources", func() {
		var err error
		var instance *druidv1alpha1.Etcd
		var c client.Client

		BeforeEach(func() {
			instance = getEtcd("foo2", "foo2")
			c = mgr.GetClient()
			ns := corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: instance.Namespace,
				},
			}
			c.Create(context.TODO(), &ns)
			storeSecret := instance.Spec.Backup.Store.SecretRef.Name
			tlsClientSecret := instance.Spec.Etcd.TLS.ClientTLSSecretRef.Name
			tlsServerName := instance.Spec.Etcd.TLS.ServerTLSSecretRef.Name
			tlsCAName := instance.Spec.Etcd.TLS.TLSCASecretRef.Name
			errors := createSecrets(c, instance.Namespace, storeSecret, tlsCAName, tlsClientSecret, tlsServerName)
			Expect(len(errors)).Should(BeZero())
		})
		It("should create statefulset", func() {
			defer WithWd("..")()

			err = c.Create(context.TODO(), instance)
			Expect(err).NotTo(HaveOccurred())

			ss := appsv1.StatefulSet{}
			getStatefulset(c, instance, &ss)
			testLog.Info("fetched Statefulset", "statefulset", ss.Name)
			Eventually(ss.Name, timeout).ShouldNot(BeEmpty())
		})
		AfterEach(func() {
			c.Delete(context.TODO(), instance)
		})
	})
	Context("when adding etcd resources with statefulset already present", func() {
		var err error
		var instance *druidv1alpha1.Etcd
		var c client.Client
		var ss *appsv1.StatefulSet
		BeforeEach(func() {
			instance = getEtcd("foo3", "default")

			// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
			// channel when it is finished.

			Expect(err).NotTo(HaveOccurred())
			c = mgr.GetClient()
			ss = createStatefulset("foo3", "default", instance.Spec.Labels)
			storeSecret := instance.Spec.Backup.Store.SecretRef.Name
			tlsClientSecret := instance.Spec.Etcd.TLS.ClientTLSSecretRef.Name
			tlsServerName := instance.Spec.Etcd.TLS.ServerTLSSecretRef.Name
			errors := createSecrets(c, instance.Namespace, storeSecret, tlsClientSecret, tlsServerName)
			Expect(len(errors)).Should(BeZero())
			c.Create(context.TODO(), ss)
		})
		It("should adopt statefulset ", func() {
			defer WithWd("..")()
			Expect(ss.OwnerReferences).Should(BeNil())
			err = c.Create(context.TODO(), instance)

			Expect(err).NotTo(HaveOccurred())

			s := &appsv1.StatefulSet{}
			getStatefulset(c, instance, s)
			testLog.Info("fetched Statefulset", "statefulset", s.Name)
			Eventually(len(s.OwnerReferences), timeout).ShouldNot(BeZero())
		})
		AfterEach(func() {
			c.Delete(context.TODO(), instance)
		})
	})

})

func getStatefulset(c client.Client, instance *druidv1alpha1.Etcd, ss *appsv1.StatefulSet) {
	ctx, cancel := context.WithTimeout(context.TODO(), timeout)
	defer cancel()
	wait.PollImmediateUntil(1*time.Second, func() (bool, error) {
		req := types.NamespacedName{
			Name:      fmt.Sprintf("%s", instance.Name),
			Namespace: instance.Namespace,
		}
		if err := c.Get(ctx, req, ss); err != nil {
			if errors.IsNotFound(err) {
				// Object not found, return.  Created objects are automatically garbage collected.
				// For additional cleanup logic use finalizers.
				return false, nil
			}
			return false, err
		}
		return true, nil
	}, ctx.Done())
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
			Template:             corev1.PodTemplateSpec{},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{},
			ServiceName:          fmt.Sprintf("etcd-client-%s", name),
			UpdateStrategy:       appsv1.StatefulSetUpdateStrategy{},
		},
	}
	return &ss
}

func getEtcd(name, namespace string) *druidv1alpha1.Etcd {
	clientPort := 2379
	serverPort := 2380
	port := 8080
	garbageCollectionPeriod := metav1.Duration{
		Duration: 43200 * time.Second,
	}
	deltaSnapshotPeriod := metav1.Duration{
		Duration: 300 * time.Second,
	}

	imageEtcd := "quay.io/coreos/etcd:v3.3.13"
	imageBR := "eu.gcr.io/gardener-project/gardener/etcdbrctl:0.8.0-dev"
	snapshotSchedule := "0 */24 * * *"
	defragSchedule := "0 */24 * * *"
	container := "shoot--dev--i308301-1--b3caa"
	storageCapacity := resource.MustParse("80Gi")
	deltaSnapShotMemLimit := resource.MustParse("100Mi")
	quota := resource.MustParse("8Gi")
	storageClass := "gardener.cloud-fast"
	provider := druidv1alpha1.StorageProvider("aws")
	prefix := "etcd-test"
	garbageCollectionPolicy := druidv1alpha1.GarbageCollectionPolicy(druidv1alpha1.GarbageCollectionPolicyExponential)

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

	instance := &druidv1alpha1.Etcd{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: druidv1alpha1.EtcdSpec{
			Annotations: map[string]string{
				"app":  "etcd-statefulset",
				"role": "test",
			},
			Labels: map[string]string{
				"app":      "etcd-statefulset",
				"role":     "test",
				"instance": name,
			},
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app":      "etcd-statefulset",
					"instance": name,
				},
			},
			Replicas:        1,
			StorageClass:    &storageClass,
			StorageCapacity: &storageCapacity,

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
				TLS:        tlsConfig,
			},
		},
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
