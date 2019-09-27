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
	"time"

	"k8s.io/apimachinery/pkg/api/resource"

	druidv1 "github.com/gardener/etcd-druid/api/v1"
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
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const timeout = time.Second * 30

var _ = Describe("Druid", func() {
	Context("when adding etcd resources", func() {

		var err error
		var instance *druidv1.Etcd
		var c client.Client
		BeforeEach(func() {
			instance = getEtcd("foo1", "default")
			c = mgr.GetClient()
			storeSecret := instance.Spec.Store.StoreSecret
			tlsClientSecret := instance.Spec.TLSClientSecretName
			tlsServerName := instance.Spec.TLSClientSecretName
			createSecrets(c, instance.Namespace, storeSecret, tlsClientSecret, tlsServerName)
		})
		It("should call reconcile", func() {
			expectedRequest := reconcile.Request{NamespacedName: types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}}
			err = c.Create(context.TODO(), instance)
			// The instance object may not be a valid object because it might be missing some required fields.
			// Please modify the instance object by adding required fields and then remove the following if statement.
			if apierrors.IsInvalid(err) {
				testLog.Info("failed to create object", "got an invalid object error", err)
				return
			}
			Expect(err).NotTo(HaveOccurred())
			defer c.Delete(context.TODO(), instance)
			Eventually(requests, timeout).Should(Receive(Equal(expectedRequest)))
		})
	})
	Context("when adding etcd resources", func() {
		var err error
		var instance *druidv1.Etcd
		var c client.Client

		BeforeEach(func() {
			instance = getEtcd("foo2", "default")

			// Setup the Manager and Controller.  Wrap the Controller Reconcile function so it writes each request to a
			// channel when it is finished.

			Expect(err).NotTo(HaveOccurred())
			c = mgr.GetClient()
			storeSecret := instance.Spec.Store.StoreSecret
			tlsClientSecret := instance.Spec.TLSClientSecretName
			tlsServerName := instance.Spec.TLSClientSecretName
			createSecrets(c, instance.Namespace, storeSecret, tlsClientSecret, tlsServerName)
		})
		It("should create statefulset", func() {

			err = c.Create(context.TODO(), instance)
			// The instance object may not be a valid object because it might be missing some required fields.
			// Please modify the instance object by adding required fields and then remove the following if statement.
			if apierrors.IsInvalid(err) {
				testLog.Error(err, "got an invalid object error")
				return
			}
			ss := appsv1.StatefulSet{}
			getStatefulset(c, instance, &ss)
			testLog.Info("fetched Statefulset", "statefulset", ss.Name)
			defer c.Delete(context.TODO(), instance)
			Eventually(ss.Name, timeout).ShouldNot(BeEmpty())
		})
	})
})

func getStatefulset(c client.Client, instance *druidv1.Etcd, ss *appsv1.StatefulSet) {
	ctx, cancel := context.WithTimeout(context.TODO(), timeout)
	defer cancel()
	wait.PollImmediateUntil(1*time.Second, func() (bool, error) {
		req := types.NamespacedName{
			Name:      fmt.Sprintf("etcd-%s", instance.Name),
			Namespace: instance.Namespace,
		}
		if err := c.Get(context.TODO(), req, ss); err != nil {
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
func getEtcd(name, namespace string) *druidv1.Etcd {
	instance := &druidv1.Etcd{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo2",
			Namespace: "default",
		},
		Spec: druidv1.Spec{
			Annotations: map[string]string{
				"app":  "etcd-statefulset",
				"role": "test",
			},
			Labels: map[string]string{
				"app":  "etcd-statefulset",
				"role": "test",
			},
			PVCRetentionPolicy:  druidv1.PolicyDeleteAll,
			Replicas:            1,
			StorageClass:        "gardener.cloud-fast",
			TLSClientSecretName: "etcd-client-tls",
			TLSServerSecretName: "etcd-server-tls",
			StorageCapacity:     "80Gi",
			Store: druidv1.StoreSpec{
				StoreSecret:      "etcd-backup",
				StorageContainer: "shoot--dev--i308301-1--b3caa",
				StorageProvider:  "S3",
				StorePrefix:      "etcd-test",
			},
			Backup: druidv1.BackupSpec{
				PullPolicy:               corev1.PullIfNotPresent,
				ImageRepository:          "eu.gcr.io/gardener-project/gardener/etcdbrctl",
				ImageVersion:             "0.8.0-dev",
				Port:                     8080,
				FullSnapshotSchedule:     "0 */24 * * *",
				GarbageCollectionPolicy:  druidv1.GarbageCollectionPolicyExponential,
				EtcdQuotaBytes:           8589934592,
				EtcdConnectionTimeout:    "300s",
				SnapstoreTempDir:         "/var/etcd/data/temp",
				GarbageCollectionPeriod:  "43200s",
				DeltaSnapshotPeriod:      "300s",
				DeltaSnapshotMemoryLimit: 104857600,
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						"cpu":    parseQuantity("500m"),
						"memory": parseQuantity("2Gi"),
					},
					Requests: corev1.ResourceList{
						"cpu":    parseQuantity("23m"),
						"memory": parseQuantity("128Mi"),
					},
				},
			},
			Etcd: druidv1.EtcdSpec{
				EnableTLS:               false,
				MetricLevel:             druidv1.Basic,
				ImageRepository:         "quay.io/coreos/etcd",
				ImageVersion:            "v3.3.13",
				DefragmentationSchedule: "0 */24 * * *",
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						"cpu":    parseQuantity("2500m"),
						"memory": parseQuantity("4Gi"),
					},
					Requests: corev1.ResourceList{
						"cpu":    parseQuantity("500m"),
						"memory": parseQuantity("1000Mi"),
					},
				},
				ClientPort:          2379,
				ServerPort:          2380,
				PullPolicy:          corev1.PullIfNotPresent,
				InitialClusterState: "new",
				InitialClusterToken: "new",
			},
		},
	}
	return instance
}

func parseQuantity(q string) resource.Quantity {
	val, _ := resource.ParseQuantity(q)
	return val
}

func createSecrets(c client.Client, namespace string, secrets ...string) {
	for _, secret := range secrets {
		secret := corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secret,
				Namespace: namespace,
			},
			Data: map[string][]byte{
				"test": []byte("test"),
			},
		}
		c.Create(context.TODO(), &secret)
	}
}
