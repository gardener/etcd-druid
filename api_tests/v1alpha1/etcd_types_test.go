/*
Copyright (c) 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"context"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"k8s.io/apimachinery/pkg/api/resource"

	corev1 "k8s.io/api/core/v1"
)

// These tests are written in BDD-style using Ginkgo framework. Refer to
// http://onsi.github.io/ginkgo to learn more.

var _ = Describe("Etcd", func() {
	var (
		key              types.NamespacedName
		created, fetched *druidv1alpha1.Etcd
	)

	BeforeEach(func() {
		// Add any setup steps that needs to be executed before each test
	})

	AfterEach(func() {
		// Add any teardown steps that needs to be executed after each test
	})

	// Add Tests for OpenAPI validation (or additional CRD features) specified in
	// your API definition.
	// Avoid adding tests for vanilla CRUD operations because they would
	// test Kubernetes API server, which isn't the goal here.
	Context("Create API", func() {

		It("should create an object successfully", func() {

			key = types.NamespacedName{
				Name:      "foo",
				Namespace: "default",
			}
			created = getEtcd("foo", "default")

			By("creating an API obj")
			Expect(k8sClient.Create(context.TODO(), created)).To(Succeed())

			fetched = &druidv1alpha1.Etcd{}
			Expect(k8sClient.Get(context.TODO(), key, fetched)).To(Succeed())
			Expect(fetched).To(Equal(created))

			By("deleting the created object")
			Expect(k8sClient.Delete(context.TODO(), created)).To(Succeed())
			Expect(k8sClient.Get(context.TODO(), key, created)).ToNot(Succeed())
		})

	})

})

func getEtcd(name, namespace string) *druidv1alpha1.Etcd {
	var (
		clientPort  int32                      = 2379
		serverPort  int32                      = 2380
		backupPort  int32                      = 8080
		metricLevel druidv1alpha1.MetricsLevel = druidv1alpha1.Basic
	)

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
				Metrics:                 &metricLevel,
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
