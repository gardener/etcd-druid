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

package poddisruptionbudget_test

import (
	"context"

	"github.com/Masterminds/semver"
	. "github.com/gardener/etcd-druid/pkg/component/etcd/poddisruptionbudget"
	. "github.com/gardener/gardener/pkg/utils/test/matchers"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	"github.com/gardener/gardener/pkg/operation/botanist/component"
	kutil "github.com/gardener/gardener/pkg/utils/kubernetes"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func GetFakeKubernetesClientSet() client.Client {
	return fake.NewClientBuilder().Build()
}

var _ = Describe("PodDisruptionBudget", func() {
	var (
		ctx context.Context
		cl  client.Client

		defaultPDB *policyv1beta1.PodDisruptionBudget
		etcd       *druidv1alpha1.Etcd
		namespace  string
		name       string
		uid        types.UID

		metricsLevel druidv1alpha1.MetricsLevel
		quota        resource.Quantity
		labels       map[string]string

		values      Values
		pdbDeployer component.Deployer

		k8sVersion_1_20_0, k8sVersion_1_25_0, k8sVersion_1_20_0_dev, k8sVersion_1_25_0_dev *semver.Version
	)

	BeforeEach(func() {
		ctx = context.Background()
		cl = GetFakeKubernetesClientSet()

		k8sVersion_1_20_0, _ = semver.NewVersion("1.20.0")
		k8sVersion_1_25_0, _ = semver.NewVersion("1.25.0")
		k8sVersion_1_20_0_dev, _ = semver.NewVersion("1.20.0-dev")
		k8sVersion_1_25_0_dev, _ = semver.NewVersion("1.25.0-dev")

		name = "poddisruptionbudget"
		namespace = "default"
		uid = "12345678"
		labels = map[string]string{
			"role": "main",
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
				},
			},
		}

		defaultPDB = &policyv1beta1.PodDisruptionBudget{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: policyv1beta1.PodDisruptionBudgetSpec{
				MinAvailable: &intstr.IntOrString{
					Type:   0,
					IntVal: 0,
				},
			},
		}
	})
	Describe("#Deploy", func() {
		var (
			pdb *policyv1beta1.PodDisruptionBudget
		)
		BeforeEach(func() {
			pdb = &policyv1beta1.PodDisruptionBudget{}
		})
		AfterEach(func() {
			Expect(cl.Delete(ctx, pdb)).To(Succeed())
		})
		Context("when PDB does not exist", func() {
			Context("when etcd replicas are 0", func() {
				It("should create the PDB successfully", func() {
					etcd.Spec.Replicas = 0
					etcd.Status = druidv1alpha1.EtcdStatus{}
					values = GenerateValues(etcd)
					pdbDeployer = New(cl, namespace, &values, *k8sVersion_1_20_0)
					Expect(pdbDeployer.Deploy(ctx)).To(Succeed())

					Expect(cl.Get(ctx, kutil.Key(namespace, values.EtcdName), pdb)).To(Succeed())
					Expect(pdb.Spec.MinAvailable.IntVal).To(BeNumerically("==", 0))
					checkV1beta1PDB(pdb, &values)
				})
			})
			Context("when etcd replicas are 5 and clusterSize in etcd status is 5", func() {
				It("should create the PDB successfully", func() {
					etcd.Spec.Replicas = 5
					etcd.Status = druidv1alpha1.EtcdStatus{
						ClusterSize: pointer.Int32Ptr(4),
					}
					values = GenerateValues(etcd)
					pdbDeployer = New(cl, namespace, &values, *k8sVersion_1_20_0)
					Expect(pdbDeployer.Deploy(ctx)).To(Succeed())

					Expect(cl.Get(ctx, kutil.Key(namespace, values.EtcdName), pdb)).To(Succeed())
					Expect(pdb.Spec.MinAvailable.IntVal).To(BeNumerically("==", 3))
					checkV1beta1PDB(pdb, &values)
				})
			})
		})

		Context("when pdb already exists", func() {
			It("should update the pdb successfully", func() {
				values = GenerateValues(etcd)
				Expect(cl.Create(ctx, defaultPDB)).To(Succeed())
				Expect(cl.Get(ctx, kutil.Key(namespace, values.EtcdName), pdb)).To(Succeed())
				Expect(pdb.Spec.MinAvailable.IntVal).To(BeNumerically("==", 0))

				etcd.Spec.Replicas = 5
				etcd.Status = druidv1alpha1.EtcdStatus{
					ClusterSize: pointer.Int32Ptr(4),
				}
				values = GenerateValues(etcd)
				pdbDeployer = New(cl, namespace, &values, *k8sVersion_1_20_0)
				Expect(pdbDeployer.Deploy(ctx)).To(Succeed())

				Expect(cl.Get(ctx, kutil.Key(namespace, values.EtcdName), pdb)).To(Succeed())
				Expect(pdb.Spec.MinAvailable.IntVal).To(BeNumerically("==", 3))
				checkV1beta1PDB(pdb, &values)
			})
		})
	})

	Describe("#Destroy", func() {
		var (
			pdb    *policyv1beta1.PodDisruptionBudget
			values Values
		)
		BeforeEach(func() {
			pdb = &policyv1beta1.PodDisruptionBudget{}
			values = GenerateValues(etcd)
		})
		Context("when PDB does not exist", func() {
			It("should destroy successfully", func() {
				pdbDeployer = New(cl, namespace, &values, *k8sVersion_1_20_0)
				Expect(pdbDeployer.Destroy(ctx)).To(Succeed())
				Expect(cl.Get(ctx, kutil.Key(namespace, values.EtcdName), pdb)).To(BeNotFoundError())
			})
		})

		Context("when PDB exists", func() {
			It("should destroy successfully", func() {
				Expect(cl.Create(ctx, defaultPDB)).To(Succeed())
				pdbDeployer = New(cl, namespace, &values, *k8sVersion_1_20_0)
				Expect(pdbDeployer.Destroy(ctx)).To(Succeed())
				Expect(cl.Get(ctx, kutil.Key(namespace, values.EtcdName), pdb)).To(BeNotFoundError())
			})
		})
	})

	Describe("#K8sUpgrade", func() {
		Context("When a v1beta1 PDB exists and we try to deploy a v1 PDB", func() {
			It("Should successfully deploy the v1 PDB", func() {
				pdb := &policyv1beta1.PodDisruptionBudget{}
				Expect(cl.Create(ctx, defaultPDB)).To(Succeed())
				Expect(cl.Get(ctx, kutil.Key(namespace, name), pdb)).To(Succeed())
				Expect(pdb.Spec.MinAvailable.IntVal).To(BeNumerically("==", 0))
				Expect(pdb.APIVersion).To(Equal("policy/v1beta1"))

				etcd.Spec.Replicas = 5
				etcd.Status = druidv1alpha1.EtcdStatus{
					ClusterSize: pointer.Int32Ptr(4),
				}
				values = GenerateValues(etcd)
				pdbDeployer = New(cl, namespace, &values, *k8sVersion_1_25_0)
				Expect(pdbDeployer.Deploy(ctx)).To(Succeed())

				pdb1 := &policyv1.PodDisruptionBudget{}
				Expect(cl.Get(ctx, kutil.Key(namespace, name), pdb1)).To(Succeed())
				Expect(pdb1.APIVersion).To(Equal("policy/v1"))
				Expect(pdb1.Spec.MinAvailable.IntVal).To(BeNumerically("==", 3))
				checkV1PDB(pdb1, &values)

				Expect(cl.Delete(ctx, pdb1)).To(Succeed())
				Expect(cl.Delete(ctx, pdb)).To(Succeed())

			})
		})
	})

	Describe("#PDB APIVersion", func() {
		Context("With k8s version less than 1.21", func() {
			It("Should create a PDB with the policy/v1beta1 API", func() {
				values = GenerateValues(etcd)
				pdbDeployer = New(cl, namespace, &values, *k8sVersion_1_20_0)
				Expect(pdbDeployer.Deploy(ctx)).To(Succeed())

				pdb := &policyv1beta1.PodDisruptionBudget{}
				Expect(cl.Get(ctx, kutil.Key(namespace, values.EtcdName), pdb)).To(Succeed())
				Expect(pdb.APIVersion).To(Equal("policy/v1beta1"))
				checkV1beta1PDB(pdb, &values)

				Expect(cl.Delete(ctx, pdb)).To(Succeed())
			})
		})
		Context("With k8s version greater than 1.21", func() {
			It("Should create a PDB with the policyv1 API", func() {
				values = GenerateValues(etcd)
				pdbDeployer = New(cl, namespace, &values, *k8sVersion_1_25_0)
				Expect(pdbDeployer.Deploy(ctx)).To(Succeed())

				pdb := &policyv1.PodDisruptionBudget{}
				Expect(cl.Get(ctx, kutil.Key(namespace, values.EtcdName), pdb)).To(Succeed())
				Expect(pdb.APIVersion).To(Equal("policy/v1"))
				checkV1PDB(pdb, &values)

				Expect(cl.Delete(ctx, pdb)).To(Succeed())
			})
		})
	})
	Describe("#PDB APIVersion with suffix", func() {
		Context("With k8s version 1.20.0-dev", func() {
			It("Should consider a case of k8s v1.20.0 create a PDB with the policy/v1beta1 API", func() {
				values = GenerateValues(etcd)
				pdbDeployer = New(cl, namespace, &values, *k8sVersion_1_20_0_dev)
				Expect(pdbDeployer.Deploy(ctx)).To(Succeed())

				pdb := &policyv1beta1.PodDisruptionBudget{}
				Expect(cl.Get(ctx, kutil.Key(namespace, values.EtcdName), pdb)).To(Succeed())
				Expect(pdb.APIVersion).To(Equal("policy/v1beta1"))
				checkV1beta1PDB(pdb, &values)

				Expect(cl.Delete(ctx, pdb)).To(Succeed())
			})
		})
		Context("With k8s version 1.25.0-dev", func() {
			It("Should consider a case of k8s v1.25.0 create a PDB with the policyv1 API", func() {
				values = GenerateValues(etcd)
				pdbDeployer = New(cl, namespace, &values, *k8sVersion_1_25_0_dev)
				Expect(pdbDeployer.Deploy(ctx)).To(Succeed())

				pdb := &policyv1.PodDisruptionBudget{}
				Expect(cl.Get(ctx, kutil.Key(namespace, values.EtcdName), pdb)).To(Succeed())
				Expect(pdb.APIVersion).To(Equal("policy/v1"))
				checkV1PDB(pdb, &values)

				Expect(cl.Delete(ctx, pdb)).To(Succeed())
			})
		})
	})
})

func checkV1beta1PDB(pdb *policyv1beta1.PodDisruptionBudget, values *Values) {
	checkPDBMetadata(&pdb.ObjectMeta, values)

	Expect(pdb.Spec.MinAvailable.IntVal).To(BeNumerically("==", values.MinAvailable))
	Expect(pdb.Spec.Selector).To(Equal(pdbSelectorLabels(values)))
}

func checkV1PDB(pdb *policyv1.PodDisruptionBudget, values *Values) {
	checkPDBMetadata(&pdb.ObjectMeta, values)

	Expect(pdb.Spec.MinAvailable.IntVal).To(BeNumerically("==", values.MinAvailable))
	Expect(pdb.Spec.Selector).To(Equal(pdbSelectorLabels(values)))
}

func checkPDBMetadata(meta *metav1.ObjectMeta, values *Values) {
	Expect(meta.Name).To(Equal(values.EtcdName))
	Expect(meta.Namespace).To(Equal(values.EtcdNameSpace))
	Expect(meta.OwnerReferences).To(ConsistOf(Equal(metav1.OwnerReference{
		APIVersion:         druidv1alpha1.GroupVersion.String(),
		Kind:               "Etcd",
		Name:               values.EtcdName,
		UID:                values.EtcdUID,
		Controller:         pointer.BoolPtr(true),
		BlockOwnerDeletion: pointer.BoolPtr(true),
	})))
	Expect(meta.Labels).To(Equal(pdbLabels(values)))
}

func pdbLabels(val *Values) map[string]string {
	labels := map[string]string{
		"name":     "etcd",
		"instance": val.EtcdName,
		"app":      "etcd-statefulset",
		"role":     "main",
	}

	return labels
}

func pdbSelectorLabels(val *Values) *metav1.LabelSelector {
	labels := metav1.LabelSelector{
		MatchLabels: map[string]string{
			"instance": val.EtcdName,
			"name":     "etcd",
		},
	}

	return &labels
}
