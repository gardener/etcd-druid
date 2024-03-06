// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package poddisruptionbudget_test

import (
	"context"

	. "github.com/gardener/etcd-druid/pkg/component/etcd/poddisruptionbudget"
	. "github.com/gardener/gardener/pkg/utils/test/matchers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/gardener/gardener/pkg/component"
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

		defaultPDB *policyv1.PodDisruptionBudget
		etcd       *druidv1alpha1.Etcd
		namespace  string
		name       string
		uid        types.UID

		metricsLevel druidv1alpha1.MetricsLevel
		quota        resource.Quantity
		labels       map[string]string

		values      Values
		pdbDeployer component.Deployer
	)

	BeforeEach(func() {
		ctx = context.Background()
		cl = GetFakeKubernetesClientSet()

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

		defaultPDB = &policyv1.PodDisruptionBudget{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: policyv1.PodDisruptionBudgetSpec{
				MinAvailable: &intstr.IntOrString{
					Type:   0,
					IntVal: 0,
				},
			},
		}
	})
	Describe("#Deploy", func() {
		var (
			pdb *policyv1.PodDisruptionBudget
		)
		BeforeEach(func() {
			pdb = &policyv1.PodDisruptionBudget{}
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
					pdbDeployer = New(cl, namespace, &values)
					Expect(pdbDeployer.Deploy(ctx)).To(Succeed())

					Expect(cl.Get(ctx, kutil.Key(namespace, values.Name), pdb)).To(Succeed())
					Expect(pdb.Spec.MinAvailable.IntVal).To(BeNumerically("==", 0))
					checkPDB(pdb, &values, namespace)
				})
			})
			Context("when etcd replicas are 5", func() {
				It("should create the PDB successfully", func() {
					etcd.Spec.Replicas = 5
					values = GenerateValues(etcd)
					pdbDeployer = New(cl, namespace, &values)
					Expect(pdbDeployer.Deploy(ctx)).To(Succeed())

					Expect(cl.Get(ctx, kutil.Key(namespace, values.Name), pdb)).To(Succeed())
					Expect(pdb.Spec.MinAvailable.IntVal).To(BeNumerically("==", 3))
					checkPDB(pdb, &values, namespace)
				})
			})
		})

		Context("when pdb already exists", func() {
			It("should update the pdb successfully when etcd replicas change", func() {
				// existing PDB with min = 0
				values = GenerateValues(etcd)
				Expect(cl.Create(ctx, defaultPDB)).To(Succeed())
				Expect(cl.Get(ctx, kutil.Key(namespace, values.Name), pdb)).To(Succeed())
				Expect(pdb.Spec.MinAvailable.IntVal).To(BeNumerically("==", 0))

				// Post updating the etcd replicas
				etcd.Spec.Replicas = 5
				values = GenerateValues(etcd)
				pdbDeployer = New(cl, namespace, &values)
				Expect(pdbDeployer.Deploy(ctx)).To(Succeed())

				Expect(cl.Get(ctx, kutil.Key(namespace, values.Name), pdb)).To(Succeed())
				Expect(pdb.Spec.MinAvailable.IntVal).To(BeNumerically("==", 3))
				checkPDB(pdb, &values, namespace)
			})
		})
	})

	Describe("#Destroy", func() {
		var (
			pdb    *policyv1.PodDisruptionBudget
			values Values
		)
		BeforeEach(func() {
			pdb = &policyv1.PodDisruptionBudget{}
			values = GenerateValues(etcd)
		})
		Context("when PDB does not exist", func() {
			It("should destroy successfully ignoring the not-found error", func() {
				pdbDeployer = New(cl, namespace, &values)
				Expect(pdbDeployer.Destroy(ctx)).To(Succeed())
				Expect(cl.Get(ctx, kutil.Key(namespace, values.Name), pdb)).To(BeNotFoundError())
			})
		})

		Context("when PDB exists", func() {
			It("should destroy successfully", func() {
				Expect(cl.Create(ctx, defaultPDB)).To(Succeed())
				pdbDeployer = New(cl, namespace, &values)
				Expect(pdbDeployer.Destroy(ctx)).To(Succeed())
				Expect(cl.Get(ctx, kutil.Key(namespace, values.Name), pdb)).To(BeNotFoundError())
			})
		})
	})
})

func checkPDB(pdb *policyv1.PodDisruptionBudget, values *Values, expectedNamespace string) {
	Expect(pdb.Name).To(Equal(values.Name))
	Expect(pdb.Namespace).To(Equal(expectedNamespace))
	Expect(pdb.OwnerReferences).To(Equal([]metav1.OwnerReference{values.OwnerReference}))
	Expect(pdb.Labels).To(Equal(pdbLabels(values)))
	Expect(pdb.Spec.MinAvailable.IntVal).To(BeNumerically("==", values.MinAvailable))
	Expect(pdb.Spec.Selector).To(Equal(pdbSelectorLabels(values)))
}

func pdbLabels(val *Values) map[string]string {
	return map[string]string{
		"name":     "etcd",
		"instance": val.Name,
	}
}

func pdbSelectorLabels(val *Values) *metav1.LabelSelector {
	return &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"instance": val.Name,
			"name":     "etcd",
		},
	}
}
