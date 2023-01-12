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

package configmap_test

import (
	"context"
	"encoding/json"
	"fmt"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/pkg/client/kubernetes"
	. "github.com/gardener/etcd-druid/pkg/component/etcd/configmap"
	"github.com/ghodss/yaml"

	"github.com/gardener/gardener/pkg/operation/botanist/component"
	"github.com/gardener/gardener/pkg/utils"
	kutil "github.com/gardener/gardener/pkg/utils/kubernetes"
	. "github.com/gardener/gardener/pkg/utils/test/matchers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const etcdConfig = "etcd.conf.yaml"

var _ = Describe("Configmap", func() {
	var (
		ctx context.Context
		cl  client.Client

		etcd      *druidv1alpha1.Etcd
		namespace string
		name      string
		uid       types.UID

		metricsLevel            druidv1alpha1.MetricsLevel
		quota                   resource.Quantity
		clientPort, serverPort  int32
		autoCompactionMode      druidv1alpha1.CompactionMode
		autoCompactionRetention string
		labels                  map[string]string

		cm *corev1.ConfigMap

		values            *Values
		configmapDeployer component.Deployer
	)

	BeforeEach(func() {
		ctx = context.Background()
		cl = fakeclient.NewClientBuilder().WithScheme(kubernetes.Scheme).Build()

		name = "configmap"
		namespace = "default"
		uid = "a9b8c7d6e5f4"

		metricsLevel = druidv1alpha1.Basic
		quota = resource.MustParse("8Gi")
		clientPort = 2222
		serverPort = 3333
		autoCompactionMode = "periodic"
		autoCompactionRetention = "30m"

		labels = map[string]string{
			"app":      "etcd-statefulset",
			"instance": "configmap",
			"name":     "etcd",
			"role":     "main",
		}

		etcd = &druidv1alpha1.Etcd{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				UID:       uid,
			},
			Spec: druidv1alpha1.EtcdSpec{
				Labels:   labels,
				Selector: metav1.SetAsLabelSelector(labels),
				Replicas: 3,
				Etcd: druidv1alpha1.EtcdConfig{
					Quota:   &quota,
					Metrics: &metricsLevel,
					ClientUrlTLS: &druidv1alpha1.TLSConfig{
						ClientTLSSecretRef: corev1.SecretReference{
							Name: "client-url-etcd-client-tls",
						},
						ServerTLSSecretRef: corev1.SecretReference{
							Name: "client-url-etcd-server-cert",
						},
						TLSCASecretRef: druidv1alpha1.SecretReference{
							SecretReference: corev1.SecretReference{
								Name: "client-url-ca-etcd",
							},
						},
					},
					PeerUrlTLS: &druidv1alpha1.TLSConfig{
						TLSCASecretRef: druidv1alpha1.SecretReference{
							SecretReference: corev1.SecretReference{
								Name: "peer-url-ca-etcd",
							},
						},
						ServerTLSSecretRef: corev1.SecretReference{
							Name: "peer-url-etcd-server-tls",
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

		values = GenerateValues(etcd)

		cm = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("etcd-bootstrap-%s", string(values.EtcdUID[:6])),
				Namespace: values.EtcdName,
			},
		}

		configmapDeployer = New(cl, namespace, values)
	})

	Describe("#Deploy", func() {
		Context("when configmap does not exist", func() {
			It("should create the configmap successfully", func() {
				Expect(configmapDeployer.Deploy(ctx)).To(Succeed())

				cm := &corev1.ConfigMap{}

				Expect(cl.Get(ctx, kutil.Key(namespace, values.ConfigMapName), cm)).To(Succeed())
				checkConfigmap(cm, values)

			})
		})

		Context("when configmap exists", func() {
			It("should update the configmap successfully", func() {
				Expect(cl.Create(ctx, cm)).To(Succeed())

				Expect(configmapDeployer.Deploy(ctx)).To(Succeed())

				cm := &corev1.ConfigMap{}

				Expect(cl.Get(ctx, kutil.Key(namespace, values.ConfigMapName), cm)).To(Succeed())
				checkConfigmap(cm, values)
			})
		})
	})

	Describe("#Destroy", func() {
		Context("when configmap do not exist", func() {
			It("should destroy successfully", func() {
				Expect(configmapDeployer.Destroy(ctx)).To(Succeed())
				Expect(cl.Get(ctx, client.ObjectKeyFromObject(cm), &corev1.ConfigMap{})).To(BeNotFoundError())
			})
		})

		Context("when configmap exist", func() {
			It("should destroy successfully", func() {
				Expect(cl.Create(ctx, cm)).To(Succeed())

				Expect(configmapDeployer.Destroy(ctx)).To(Succeed())

				Expect(cl.Get(ctx, kutil.Key(namespace, cm.Name), &corev1.ConfigMap{})).To(BeNotFoundError())
			})
		})
	})
})

func checkConfigmap(cm *corev1.ConfigMap, values *Values) {
	checkConfigmapMetadata(&cm.ObjectMeta, values)

	configYML := cm.Data[etcdConfig]
	config := map[string]interface{}{}
	err := yaml.Unmarshal([]byte(configYML), &config)
	Expect(err).NotTo(HaveOccurred())

	Expect(config).To(MatchKeys(IgnoreExtras, Keys{
		"name":                Equal(fmt.Sprintf("etcd-%s", values.EtcdUID[:6])),
		"data-dir":            Equal("/var/etcd/data/new.etcd"),
		"metrics":             Equal(string(druidv1alpha1.Basic)),
		"snapshot-count":      Equal(float64(75000)),
		"enable-v2":           Equal(false),
		"quota-backend-bytes": Equal(float64(values.Quota.Value())),

		"client-transport-security": MatchKeys(IgnoreExtras, Keys{
			"cert-file":        Equal("/var/etcd/ssl/client/server/tls.crt"),
			"key-file":         Equal("/var/etcd/ssl/client/server/tls.key"),
			"client-cert-auth": Equal(true),
			"trusted-ca-file":  Equal("/var/etcd/ssl/client/ca/ca.crt"),
			"auto-tls":         Equal(false),
		}),
		"listen-client-urls":    Equal(fmt.Sprintf("https://0.0.0.0:%d", *values.ClientPort)),
		"advertise-client-urls": Equal(fmt.Sprintf("%s@%s@%s@%d", "https", values.PeerServiceName, values.EtcdNameSpace, *values.ClientPort)),

		"peer-transport-security": MatchKeys(IgnoreExtras, Keys{
			"cert-file":        Equal("/var/etcd/ssl/peer/server/tls.crt"),
			"key-file":         Equal("/var/etcd/ssl/peer/server/tls.key"),
			"client-cert-auth": Equal(true),
			"trusted-ca-file":  Equal("/var/etcd/ssl/peer/ca/ca.crt"),
			"auto-tls":         Equal(false),
		}),
		"listen-peer-urls":            Equal(fmt.Sprintf("https://0.0.0.0:%d", *values.ServerPort)),
		"initial-advertise-peer-urls": Equal(fmt.Sprintf("%s@%s@%s@%d", "https", values.PeerServiceName, values.EtcdNameSpace, *values.ServerPort)),

		"initial-cluster-token":     Equal("etcd-cluster"),
		"initial-cluster-state":     Equal("new"),
		"auto-compaction-mode":      Equal(string(*values.AutoCompactionMode)),
		"auto-compaction-retention": Equal(*values.AutoCompactionRetention),
	}))

	jsonString, err := json.Marshal(cm.Data)
	Expect(err).NotTo(HaveOccurred())
	configMapChecksum := utils.ComputeSHA256Hex(jsonString)

	Expect(configMapChecksum).To(Equal(values.ConfigMapChecksum))
}

func checkConfigmapMetadata(meta *metav1.ObjectMeta, values *Values) {
	Expect(meta.OwnerReferences).To(ConsistOf(Equal(metav1.OwnerReference{
		APIVersion:         druidv1alpha1.GroupVersion.String(),
		Kind:               "Etcd",
		Name:               values.EtcdName,
		UID:                values.EtcdUID,
		Controller:         pointer.BoolPtr(true),
		BlockOwnerDeletion: pointer.BoolPtr(true),
	})))
	Expect(meta.Labels).To(Equal(configmapLabels(values)))
}

func configmapLabels(val *Values) map[string]string {
	labels := map[string]string{
		"name":     "etcd",
		"instance": val.EtcdName,
		"app":      "etcd-statefulset",
		"role":     "main",
	}

	return labels
}
