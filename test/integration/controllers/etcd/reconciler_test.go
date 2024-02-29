// Copyright 2023 SAP SE or an SAP affiliate company
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package etcd

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/controllers/etcd"
	"github.com/gardener/etcd-druid/pkg/common"
	"github.com/gardener/etcd-druid/pkg/utils"
	"github.com/gardener/etcd-druid/test/integration/controllers/assets"
	testutils "github.com/gardener/etcd-druid/test/utils"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	gardenerUtils "github.com/gardener/gardener/pkg/utils"
	"github.com/gardener/gardener/pkg/utils/imagevector"
	"github.com/gardener/gardener/pkg/utils/test/matchers"
	"github.com/ghodss/yaml"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	timeout         = time.Minute * 5
	pollingInterval = time.Second * 2
	etcdConfig      = "etcd.conf.yaml"
	backupRestore   = "backup-restore"
	metricsKey      = "metrics"
)

var (
	clientPort              int32 = 2379
	serverPort              int32 = 2380
	backupPort              int32 = 8080
	defaultStorageCapacity        = resource.MustParse("16Gi")
	deltaSnapShotMemLimit         = resource.MustParse("100Mi")
	autoCompactionMode            = druidv1alpha1.Periodic
	autoCompactionRetention       = "2m"
	quota                         = resource.MustParse("8Gi")
	maxBackups                    = 7
	imageNames                    = []string{
		common.Etcd,
		common.BackupRestore,
		common.EtcdWrapper,
		common.BackupRestoreDistroless,
	}
)

var _ = Describe("Etcd Controller", func() {
	//Reconciliation of new etcd resource deployment without any existing statefulsets.
	Context("when adding etcd resources", func() {
		var (
			err      error
			instance *druidv1alpha1.Etcd
			sts      *appsv1.StatefulSet
			svc      *corev1.Service
			ctx      = context.TODO()
		)

		BeforeEach(func() {
			instance = testutils.EtcdBuilderWithDefaults("foo1", namespace).Build()

			storeSecret := instance.Spec.Backup.Store.SecretRef.Name
			errors := testutils.CreateSecrets(ctx, k8sClient, instance.Namespace, storeSecret)
			Expect(len(errors)).Should(BeZero())
			Expect(k8sClient.Create(context.TODO(), instance)).To(Succeed())

			sts = &appsv1.StatefulSet{}
			// Wait until StatefulSet has been created by controller
			Eventually(func() error {
				return k8sClient.Get(context.TODO(), types.NamespacedName{
					Name:      instance.Name,
					Namespace: instance.Namespace,
				}, sts)
			}, timeout, pollingInterval).Should(BeNil())

			svc = &corev1.Service{}
			// Wait until Service has been created by controller
			Eventually(func() error {
				return k8sClient.Get(context.TODO(), types.NamespacedName{
					Name:      instance.GetClientServiceName(),
					Namespace: instance.Namespace,
				}, svc)
			}, timeout, pollingInterval).Should(BeNil())

		})
		It("should create and adopt statefulset", func() {
			ctx := context.TODO()

			testutils.SetStatefulSetReady(sts)
			err = k8sClient.Status().Update(ctx, sts)
			Eventually(func() (bool, error) { return testutils.IsStatefulSetCorrectlyReconciled(ctx, k8sClient, instance, sts) }, timeout, pollingInterval).Should(BeTrue())
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() (*bool, error) {
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(instance), instance); err != nil {
					return nil, err
				}
				return instance.Status.Ready, nil
			}, timeout, pollingInterval).Should(Equal(pointer.Bool(true)))
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
			Expect(k8sClient.Create(context.TODO(), pvc)).To(Succeed())

			// Create PVC warning Event
			pvcMessage := "Failed to provision volume"
			Expect(k8sClient.Create(context.TODO(), &corev1.Event{
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
				if err := k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(instance), instance); err != nil {
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
			Expect(k8sClient.Delete(context.TODO(), instance)).To(Succeed())
			Eventually(func() error {
				return k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(instance), &druidv1alpha1.Etcd{})
			}, timeout, pollingInterval).Should(matchers.BeNotFoundError())
			// Delete service manually because garbage collection is not available in `envtest`
			Expect(k8sClient.Delete(context.TODO(), svc)).To(Succeed())
			Eventually(func() error {
				return k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(svc), &corev1.Service{})
			}, timeout, pollingInterval).Should(matchers.BeNotFoundError())

		})
	})

	DescribeTable("when etcd resource is created",
		func(etcdName string, withTLS bool, provider druidv1alpha1.StorageProvider, etcdWithDefaults bool, validate func(*druidv1alpha1.Etcd, *appsv1.StatefulSet, *corev1.ConfigMap, *corev1.Service, *corev1.Service)) {
			var (
				err             error
				s               *appsv1.StatefulSet
				cm              *corev1.ConfigMap
				clSvc, prSvc    *corev1.Service
				sa              *corev1.ServiceAccount
				role            *rbac.Role
				rb              *rbac.RoleBinding
				ctx             = context.TODO()
				instance        *druidv1alpha1.Etcd
				instanceBuilder *testutils.EtcdBuilder
			)
			if etcdWithDefaults {
				instanceBuilder = testutils.EtcdBuilderWithDefaults(etcdName, namespace)
			} else {
				instanceBuilder = testutils.EtcdBuilderWithoutDefaults(etcdName, namespace)
			}
			if withTLS {
				instanceBuilder.WithTLS()
			}
			instance = instanceBuilder.WithStorageProvider(provider).Build()

			if instance.Spec.Backup.Store != nil && instance.Spec.Backup.Store.SecretRef != nil {
				By("create backup-store secrets")
				storeSecret := instance.Spec.Backup.Store.SecretRef.Name
				errors := testutils.CreateSecrets(ctx, k8sClient, instance.Namespace, storeSecret)
				Expect(len(errors)).Should(BeZero())
			}

			By("create etcd instance and check if it has been created")
			err = k8sClient.Create(context.TODO(), instance)
			Expect(err).NotTo(HaveOccurred())
			Eventually(func() error {
				return k8sClient.Get(ctx, client.ObjectKeyFromObject(instance), instance)
			}).Should(Not(HaveOccurred()))

			By("check if statefulset has reconciled")
			s = &appsv1.StatefulSet{}
			Eventually(func() (bool, error) { return testutils.IsStatefulSetCorrectlyReconciled(ctx, k8sClient, instance, s) }, timeout, pollingInterval).Should(BeTrue())

			By("check if configmap has reconciled")
			cm = &corev1.ConfigMap{}
			Eventually(func() error { return testutils.ConfigMapIsCorrectlyReconciled(k8sClient, timeout, instance, cm) }, timeout, pollingInterval).Should(BeNil())

			By("check if client service has reconciled")
			clSvc = &corev1.Service{}
			Eventually(func() error { return testutils.ClientServiceIsCorrectlyReconciled(k8sClient, timeout, instance, clSvc) }, timeout, pollingInterval).Should(BeNil())

			By("check if peer service has reconciled")
			prSvc = &corev1.Service{}
			Eventually(func() error { return testutils.PeerServiceIsCorrectlyReconciled(k8sClient, timeout, instance, prSvc) }, timeout, pollingInterval).Should(BeNil())

			By("check if service account has reconciled")
			sa = &corev1.ServiceAccount{}
			Eventually(func() error { return testutils.ServiceAccountIsCorrectlyReconciled(k8sClient, timeout, instance, sa) }, timeout, pollingInterval).Should(BeNil())

			By("check if role has reconciled")
			role = &rbac.Role{}
			Eventually(func() error { return testutils.RoleIsCorrectlyReconciled(k8sClient, timeout, instance, role) }, timeout, pollingInterval).Should(BeNil())

			By("check if rolebinding has reconciled")
			rb = &rbac.RoleBinding{}
			Eventually(func() error { return testutils.RoleBindingIsCorrectlyReconciled(k8sClient, timeout, instance, rb) }, timeout, pollingInterval).Should(BeNil())

			validate(instance, s, cm, clSvc, prSvc)
			validateRole(instance, role)

			testutils.SetStatefulSetReady(s)
			err = k8sClient.Status().Update(context.TODO(), s)
			Expect(err).NotTo(HaveOccurred())
		},
		Entry("if fields are not set in etcd.Spec, the statefulset should reflect the spec changes", "foo28", false, druidv1alpha1.StorageProvider("Local"), false, validateDefaultValuesForEtcd),
		Entry("if fields are set in etcd.Spec and TLS enabled, the resources should reflect the spec changes", "foo29", true, druidv1alpha1.StorageProvider("Local"), true, validateEtcd),
		Entry("if the store is GCS, the statefulset should reflect the spec changes", "foo30", true, druidv1alpha1.StorageProvider("gcp"), true, validateStoreGCP),
		Entry("if the store is S3, the statefulset should reflect the spec changes", "foo31", true, druidv1alpha1.StorageProvider("aws"), true, validateStoreAWS),
		Entry("if the store is ABS, the statefulset should reflect the spec changes", "foo32", true, druidv1alpha1.StorageProvider("azure"), true, validateStoreAzure),
		Entry("if the store is Swift, the statefulset should reflect the spec changes", "foo33", true, druidv1alpha1.StorageProvider("openstack"), true, validateStoreOpenstack),
		Entry("if the store is OSS, the statefulset should reflect the spec changes", "foo34", true, druidv1alpha1.StorageProvider("alicloud"), true, validateStoreAlicloud),
	)
})

var _ = Describe("Multinode ETCD", func() {
	//Reconciliation of new etcd resource deployment without any existing statefulsets.
	Context("when adding etcd resources", func() {
		var (
			instance *druidv1alpha1.Etcd
			sts      *appsv1.StatefulSet
			svc      *corev1.Service
			ctx      = context.TODO()
		)

		BeforeEach(func() {
			instance = testutils.EtcdBuilderWithDefaults("foo82", namespace).Build()
			storeSecret := instance.Spec.Backup.Store.SecretRef.Name
			errors := testutils.CreateSecrets(ctx, k8sClient, instance.Namespace, storeSecret)
			Expect(len(errors)).Should(BeZero())
		})
		It("should create the statefulset based on the replicas in ETCD CR", func() {
			// First delete existing statefulset if any.
			// This is required due to a bug
			sts = &appsv1.StatefulSet{}
			sts.Name = instance.Name
			sts.Namespace = instance.Namespace
			Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, sts))).To(Succeed())

			By("update replicas in ETCD resource with 0")
			instance.Spec.Replicas = 4
			Expect(k8sClient.Create(ctx, instance)).To(Succeed())

			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      instance.Name,
					Namespace: instance.Namespace,
				}, instance)
			}, timeout, pollingInterval).Should(BeNil())

			By("no StatefulSet has been created by controller as even number of replicas are not allowed")
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      instance.Name,
					Namespace: instance.Namespace,
				}, sts)
			}, timeout, pollingInterval).Should(matchers.BeNotFoundError())

			By("update replicas in ETCD resource with 3")
			patch := client.MergeFrom(instance.DeepCopy())
			instance.Spec.Replicas = 3
			Expect(k8sClient.Patch(ctx, instance, patch)).To(Succeed())

			By("statefulsets are created when ETCD replicas are odd number")
			Eventually(func() (bool, error) { return testutils.IsStatefulSetCorrectlyReconciled(ctx, k8sClient, instance, sts) }, timeout, pollingInterval).Should(BeTrue())
			Expect(int(*sts.Spec.Replicas)).To(Equal(3))

			By("client Service has been created by controller")
			svc = &corev1.Service{}
			Eventually(func() error { return testutils.ClientServiceIsCorrectlyReconciled(k8sClient, timeout, instance, svc) }, timeout, pollingInterval).Should(BeNil())

			By("should raise an event if annotation to ignore reconciliation is applied on ETCD CR")
			patch = client.MergeFrom(instance.DeepCopy())
			annotations := utils.MergeStringMaps(
				map[string]string{
					etcd.IgnoreReconciliationAnnotation: "true",
				},
				instance.Annotations,
			)
			instance.Annotations = annotations
			Expect(k8sClient.Patch(ctx, instance, patch)).To(Succeed())

			clientset, _ := kubernetes.NewForConfig(restConfig)
			Eventually(func() error {
				events, err := clientset.CoreV1().Events(instance.Namespace).List(ctx, metav1.ListOptions{})
				if err != nil {
					fmt.Printf("The error is : %v", err)
					return err
				}

				if events == nil || len(events.Items) == 0 {
					return fmt.Errorf("no events generated for annotation to ignore reconciliation")
				}

				for _, event := range events.Items {
					if event.Reason == "ReconciliationIgnored" {
						return nil
					}
				}
				return nil

			}, timeout, pollingInterval).Should(BeNil())

			By("delete `etcd` instance")
			Expect(client.IgnoreNotFound(k8sClient.Delete(context.TODO(), instance))).To(Succeed())
			Eventually(func() error {
				return k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(instance), &druidv1alpha1.Etcd{})
			}, timeout, pollingInterval).Should(matchers.BeNotFoundError())

			By("delete service manually because garbage collection is not available in `envtest`")
			if svc != nil {
				Expect(k8sClient.Delete(context.TODO(), svc)).To(Succeed())
				Eventually(func() error {
					return k8sClient.Get(context.TODO(), client.ObjectKeyFromObject(svc), &corev1.Service{})
				}, timeout, pollingInterval).Should(matchers.BeNotFoundError())
			}
		})
	})
	DescribeTable("configmaps are mounted properly when ETCD replicas are odd number", func(etcdName string, replicas int) {
		var (
			err      error
			sts      *appsv1.StatefulSet
			cm       *corev1.ConfigMap
			svc      *corev1.Service
			ctx      = context.TODO()
			instance *druidv1alpha1.Etcd
		)
		instance = testutils.EtcdBuilderWithDefaults(etcdName, namespace).WithReplicas(int32(replicas)).Build()

		if instance.Spec.Backup.Store != nil && instance.Spec.Backup.Store.SecretRef != nil {
			storeSecret := instance.Spec.Backup.Store.SecretRef.Name
			errors := testutils.CreateSecrets(ctx, k8sClient, instance.Namespace, storeSecret)
			Expect(len(errors)).Should(BeZero())
		}
		err = k8sClient.Create(context.TODO(), instance)
		Expect(err).NotTo(HaveOccurred())
		sts = &appsv1.StatefulSet{}
		Eventually(func() (bool, error) { return testutils.IsStatefulSetCorrectlyReconciled(ctx, k8sClient, instance, sts) }, timeout, pollingInterval).Should(BeTrue())
		cm = &corev1.ConfigMap{}
		Eventually(func() error { return testutils.ConfigMapIsCorrectlyReconciled(k8sClient, timeout, instance, cm) }, timeout, pollingInterval).Should(BeNil())
		svc = &corev1.Service{}
		Eventually(func() error { return testutils.ClientServiceIsCorrectlyReconciled(k8sClient, timeout, instance, svc) }, timeout, pollingInterval).Should(BeNil())

		// Validate statefulset
		Expect(*sts.Spec.Replicas).To(Equal(instance.Spec.Replicas))

		if instance.Spec.Replicas == 1 {
			matcher := fmt.Sprintf("initial-cluster: foo83-0=http://foo83-0.foo83-peer.%s.svc:2380", namespace)
			Expect(strings.Contains(cm.Data["etcd.conf.yaml"], matcher)).To(BeTrue())
		}

		if instance.Spec.Replicas > 1 {
			matcher := fmt.Sprintf("initial-cluster: foo84-0=http://foo84-0.foo84-peer.%s.svc:2380,foo84-1=http://foo84-1.foo84-peer.%s.svc:2380,foo84-2=http://foo84-2.foo84-peer.%s.svc:2380", namespace, namespace, namespace)
			Expect(strings.Contains(cm.Data["etcd.conf.yaml"], matcher)).To(BeTrue())
		}
	},
		Entry("verify configmap mount path and etcd.conf.yaml when replica is 1 ", "foo83", 1),
		Entry("verify configmap mount path and etcd.conf.yaml when replica is 3 ", "foo84", 3),
	)
})

func validateRole(instance *druidv1alpha1.Etcd, role *rbac.Role) {
	Expect(*role).To(MatchFields(IgnoreExtras, Fields{
		"ObjectMeta": MatchFields(IgnoreExtras, Fields{
			"Name":      Equal(instance.GetRoleName()),
			"Namespace": Equal(instance.Namespace),
			"Labels": MatchKeys(IgnoreExtras, Keys{
				"name":     Equal("etcd"),
				"instance": Equal(instance.Name),
			}),
			"OwnerReferences": MatchElements(testutils.OwnerRefIterator, IgnoreExtras, Elements{
				instance.Name: MatchFields(IgnoreExtras, Fields{
					"APIVersion":         Equal(druidv1alpha1.GroupVersion.String()),
					"Kind":               Equal("Etcd"),
					"Name":               Equal(instance.Name),
					"UID":                Equal(instance.UID),
					"Controller":         PointTo(Equal(true)),
					"BlockOwnerDeletion": PointTo(Equal(true)),
				}),
			}),
		}),
		"Rules": MatchAllElements(testutils.RuleIterator, Elements{
			"coordination.k8s.io": MatchFields(IgnoreExtras, Fields{
				"APIGroups": MatchAllElements(testutils.StringArrayIterator, Elements{
					"coordination.k8s.io": Equal("coordination.k8s.io"),
				}),
				"Resources": MatchAllElements(testutils.StringArrayIterator, Elements{
					"leases": Equal("leases"),
				}),
				"Verbs": MatchAllElements(testutils.StringArrayIterator, Elements{
					"list":   Equal("list"),
					"get":    Equal("get"),
					"update": Equal("update"),
					"patch":  Equal("patch"),
					"watch":  Equal("watch"),
				}),
			}),
			"apps": MatchFields(IgnoreExtras, Fields{
				"APIGroups": MatchAllElements(testutils.StringArrayIterator, Elements{
					"apps": Equal("apps"),
				}),
				"Resources": MatchAllElements(testutils.StringArrayIterator, Elements{
					"statefulsets": Equal("statefulsets"),
				}),
				"Verbs": MatchAllElements(testutils.StringArrayIterator, Elements{
					"list":   Equal("list"),
					"get":    Equal("get"),
					"update": Equal("update"),
					"patch":  Equal("patch"),
					"watch":  Equal("watch"),
				}),
			}),
			"": MatchFields(IgnoreExtras, Fields{
				"APIGroups": MatchAllElements(testutils.StringArrayIterator, Elements{
					"": Equal(""),
				}),
				"Resources": MatchAllElements(testutils.StringArrayIterator, Elements{
					"pods": Equal("pods"),
				}),
				"Verbs": MatchAllElements(testutils.StringArrayIterator, Elements{
					"list":  Equal("list"),
					"get":   Equal("get"),
					"watch": Equal("watch"),
				}),
			}),
		}),
	}))
}

func validateDefaultValuesForEtcd(instance *druidv1alpha1.Etcd, s *appsv1.StatefulSet, cm *corev1.ConfigMap, clSvc *corev1.Service, prSvc *corev1.Service) {
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
	imageVector := assets.CreateImageVector()
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
		"auto-compaction-retention":   Equal("30m"),
	}))

	Expect(*clSvc).To(MatchFields(IgnoreExtras, Fields{
		"ObjectMeta": MatchFields(IgnoreExtras, Fields{
			"Name":      Equal(instance.GetClientServiceName()),
			"Namespace": Equal(instance.Namespace),
			"Labels": MatchAllKeys(Keys{
				"name":     Equal("etcd"),
				"instance": Equal(instance.Name),
			}),
			"OwnerReferences": MatchElements(testutils.OwnerRefIterator, IgnoreExtras, Elements{
				instance.Name: MatchFields(IgnoreExtras, Fields{
					"APIVersion":         Equal(druidv1alpha1.GroupVersion.String()),
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
			"Ports": MatchElements(testutils.ServicePortIterator, IgnoreExtras, Elements{
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
				"name":                      Equal("etcd"),
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
			"ServiceName": Equal(instance.GetPeerServiceName()),
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
						"app":      Equal("etcd-statefulset"),
						"name":     Equal("etcd"),
						"instance": Equal(instance.Name),
					}),
				}),
				"Spec": MatchFields(IgnoreExtras, Fields{
					"HostAliases": MatchAllElements(testutils.HostAliasIterator, Elements{
						"127.0.0.1": MatchFields(IgnoreExtras, Fields{
							"IP": Equal("127.0.0.1"),
							"Hostnames": MatchAllElements(testutils.CmdIterator, Elements{
								fmt.Sprintf("%s-local", instance.Name): Equal(fmt.Sprintf("%s-local", instance.Name)),
							}),
						}),
					}),
					"PriorityClassName": Equal(""),
					"Containers": MatchAllElements(testutils.ContainerIterator, Elements{
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
							"Args": MatchAllElements(testutils.CmdIterator, Elements{
								"start-etcd":                         Equal("start-etcd"),
								"--backup-restore-tls-enabled=false": Equal("--backup-restore-tls-enabled=false"),
								fmt.Sprintf("--backup-restore-host-port=%s-local:%d", instance.Name, backupPort): Equal(fmt.Sprintf("--backup-restore-host-port=%s-local:%d", instance.Name, backupPort)),
								fmt.Sprintf("--etcd-server-name=%s-local", instance.Name):                        Equal(fmt.Sprintf("--etcd-server-name=%s-local", instance.Name)),
							}),
							"ImagePullPolicy": Equal(corev1.PullIfNotPresent),
							"Image":           Equal(fmt.Sprintf("%s:%s", images[common.EtcdWrapper].Repository, *images[common.EtcdWrapper].Tag)),
							"Resources": MatchFields(IgnoreExtras, Fields{
								"Requests": MatchKeys(IgnoreExtras, Keys{
									corev1.ResourceCPU:    Equal(resource.MustParse("50m")),
									corev1.ResourceMemory: Equal(resource.MustParse("128Mi")),
								}),
							}),
							"ReadinessProbe": PointTo(MatchFields(IgnoreExtras, Fields{
								"ProbeHandler": MatchFields(IgnoreExtras, Fields{
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
							"VolumeMounts": MatchAllElements(testutils.VolumeMountIterator, Elements{
								instance.Name: MatchFields(IgnoreExtras, Fields{
									"Name":      Equal(instance.Name),
									"MountPath": Equal("/var/etcd/data/"),
								}),
							}),
						}),

						backupRestore: MatchFields(IgnoreExtras, Fields{
							"Args": MatchAllElements(testutils.CmdIterator, Elements{
								"server":                             Equal("server"),
								"--data-dir=/var/etcd/data/new.etcd": Equal("--data-dir=/var/etcd/data/new.etcd"),
								"--restoration-temp-snapshots-dir=/var/etcd/data/restoration.temp": Equal("--restoration-temp-snapshots-dir=/var/etcd/data/restoration.temp"),
								"--insecure-transport=true":                                        Equal("--insecure-transport=true"),
								"--insecure-skip-tls-verify=true":                                  Equal("--insecure-skip-tls-verify=true"),
								"--etcd-connection-timeout=5m":                                     Equal("--etcd-connection-timeout=5m"),
								"--snapstore-temp-directory=/var/etcd/data/temp":                   Equal("--snapstore-temp-directory=/var/etcd/data/temp"),
								"--enable-member-lease-renewal=true":                               Equal("--enable-member-lease-renewal=true"),
								"--k8s-heartbeat-duration=10s":                                     Equal("--k8s-heartbeat-duration=10s"),

								fmt.Sprintf("--delta-snapshot-memory-limit=%d", deltaSnapShotMemLimit.Value()):                 Equal(fmt.Sprintf("--delta-snapshot-memory-limit=%d", deltaSnapShotMemLimit.Value())),
								fmt.Sprintf("--garbage-collection-policy=%s", druidv1alpha1.GarbageCollectionPolicyLimitBased): Equal(fmt.Sprintf("--garbage-collection-policy=%s", druidv1alpha1.GarbageCollectionPolicyLimitBased)),
								fmt.Sprintf("--endpoints=http://%s-local:%d", instance.Name, clientPort):                       Equal(fmt.Sprintf("--endpoints=http://%s-local:%d", instance.Name, clientPort)),
								fmt.Sprintf("--service-endpoints=http://%s:%d", instance.GetClientServiceName(), clientPort):   Equal(fmt.Sprintf("--service-endpoints=http://%s:%d", instance.GetClientServiceName(), clientPort)),
								fmt.Sprintf("--embedded-etcd-quota-bytes=%d", quota.Value()):                                   Equal(fmt.Sprintf("--embedded-etcd-quota-bytes=%d", quota.Value())),
								fmt.Sprintf("--max-backups=%d", maxBackups):                                                    Equal(fmt.Sprintf("--max-backups=%d", maxBackups)),
								fmt.Sprintf("--auto-compaction-mode=%s", druidv1alpha1.Periodic):                               Equal(fmt.Sprintf("--auto-compaction-mode=%s", druidv1alpha1.Periodic)),
								fmt.Sprintf("--auto-compaction-retention=%s", "30m"):                                           Equal(fmt.Sprintf("--auto-compaction-retention=%s", "30m")),
								fmt.Sprintf("%s=%s", "--etcd-snapshot-timeout", "15m"):                                         Equal(fmt.Sprintf("%s=%s", "--etcd-snapshot-timeout", "15m")),
								fmt.Sprintf("%s=%s", "--etcd-defrag-timeout", "15m"):                                           Equal(fmt.Sprintf("%s=%s", "--etcd-defrag-timeout", "15m")),
							}),
							"Ports": ConsistOf([]corev1.ContainerPort{
								{
									Name:          "server",
									Protocol:      corev1.ProtocolTCP,
									HostPort:      0,
									ContainerPort: backupPort,
								},
							}),
							"Image":           Equal(fmt.Sprintf("%s:%s", images[common.BackupRestoreDistroless].Repository, *images[common.BackupRestoreDistroless].Tag)),
							"ImagePullPolicy": Equal(corev1.PullIfNotPresent),
							"VolumeMounts": MatchAllElements(testutils.VolumeMountIterator, Elements{
								instance.Name: MatchFields(IgnoreExtras, Fields{
									"Name":      Equal(instance.Name),
									"MountPath": Equal("/var/etcd/data"),
								}),
								"etcd-config-file": MatchFields(IgnoreExtras, Fields{
									"Name":      Equal("etcd-config-file"),
									"MountPath": Equal("/var/etcd/config/"),
								}),
							}),
							"Env": MatchAllElements(testutils.EnvIterator, Elements{
								common.EnvPodName: MatchFields(IgnoreExtras, Fields{
									"Name": Equal(common.EnvPodName),
									"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
										"FieldRef": PointTo(MatchFields(IgnoreExtras, Fields{
											"FieldPath": Equal("metadata.name"),
										})),
									})),
								}),
								common.EnvPodNamespace: MatchFields(IgnoreExtras, Fields{
									"Name": Equal(common.EnvPodNamespace),
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
					"ShareProcessNamespace": Equal(pointer.Bool(true)),
					"Volumes": MatchAllElements(testutils.VolumeIterator, Elements{
						"etcd-config-file": MatchFields(IgnoreExtras, Fields{
							"Name": Equal("etcd-config-file"),
							"VolumeSource": MatchFields(IgnoreExtras, Fields{
								"ConfigMap": PointTo(MatchFields(IgnoreExtras, Fields{
									"LocalObjectReference": MatchFields(IgnoreExtras, Fields{
										"Name": Equal(fmt.Sprintf("etcd-bootstrap-%s", string(instance.UID[:6]))),
									}),
									"DefaultMode": PointTo(Equal(int32(0640))),
									"Items": MatchAllElements(testutils.KeyIterator, Elements{
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
			"VolumeClaimTemplates": MatchAllElements(testutils.PVCIterator, Elements{
				instance.Name: MatchFields(IgnoreExtras, Fields{
					"ObjectMeta": MatchFields(IgnoreExtras, Fields{
						"Name": Equal(instance.Name),
					}),
					"Spec": MatchFields(IgnoreExtras, Fields{
						"AccessModes": MatchAllElements(testutils.AccessModeIterator, Elements{
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
			"OwnerReferences": MatchElements(testutils.OwnerRefIterator, IgnoreExtras, Elements{
				instance.Name: MatchFields(IgnoreExtras, Fields{
					"APIVersion":         Equal(druidv1alpha1.GroupVersion.String()),
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
			"Name":      Equal(instance.GetClientServiceName()),
			"Namespace": Equal(instance.Namespace),
			"Labels": MatchAllKeys(Keys{
				"name":     Equal("etcd"),
				"instance": Equal(instance.Name),
			}),
			"OwnerReferences": MatchElements(testutils.OwnerRefIterator, IgnoreExtras, Elements{
				instance.Name: MatchFields(IgnoreExtras, Fields{
					"APIVersion":         Equal(druidv1alpha1.GroupVersion.String()),
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
			"Ports": MatchElements(testutils.ServicePortIterator, IgnoreExtras, Elements{
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
				"name":                      Equal("etcd"),
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
			"ServiceName": Equal(instance.GetPeerServiceName()),
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
						"app":      Equal("etcd-statefulset"),
						"name":     Equal("etcd"),
						"instance": Equal(instance.Name),
					}),
				}),
				//s.Spec.Template.Spec.HostAliases
				"Spec": MatchFields(IgnoreExtras, Fields{
					"HostAliases": MatchAllElements(testutils.HostAliasIterator, Elements{
						"127.0.0.1": MatchFields(IgnoreExtras, Fields{
							"IP": Equal("127.0.0.1"),
							"Hostnames": MatchAllElements(testutils.CmdIterator, Elements{
								fmt.Sprintf("%s-local", instance.Name): Equal(fmt.Sprintf("%s-local", instance.Name)),
							}),
						}),
					}),
					"PriorityClassName": Equal(*instance.Spec.PriorityClassName),
					"Containers": MatchAllElements(testutils.ContainerIterator, Elements{
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
							"Args": MatchAllElements(testutils.CmdIterator, Elements{
								"start-etcd":                        Equal("start-etcd"),
								"--backup-restore-tls-enabled=true": Equal("--backup-restore-tls-enabled=true"),
								"--etcd-client-cert-path=/var/etcd/ssl/client/client/tls.crt":                    Equal("--etcd-client-cert-path=/var/etcd/ssl/client/client/tls.crt"),
								"--etcd-client-key-path=/var/etcd/ssl/client/client/tls.key":                     Equal("--etcd-client-key-path=/var/etcd/ssl/client/client/tls.key"),
								"--backup-restore-ca-cert-bundle-path=/var/etcd/ssl/client/ca/ca.crt":            Equal("--backup-restore-ca-cert-bundle-path=/var/etcd/ssl/client/ca/ca.crt"),
								fmt.Sprintf("--backup-restore-host-port=%s-local:%d", instance.Name, backupPort): Equal(fmt.Sprintf("--backup-restore-host-port=%s-local:%d", instance.Name, backupPort)),
								fmt.Sprintf("--etcd-server-name=%s-local", instance.Name):                        Equal(fmt.Sprintf("--etcd-server-name=%s-local", instance.Name)),
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
								"ProbeHandler": MatchFields(IgnoreExtras, Fields{
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
							"VolumeMounts": MatchAllElements(testutils.VolumeMountIterator, Elements{
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
							"Args": MatchAllElements(testutils.CmdIterator, Elements{
								"server": Equal("server"),
								"--cert=/var/etcd/ssl/client/client/tls.crt":                                                                        Equal("--cert=/var/etcd/ssl/client/client/tls.crt"),
								"--key=/var/etcd/ssl/client/client/tls.key":                                                                         Equal("--key=/var/etcd/ssl/client/client/tls.key"),
								"--cacert=/var/etcd/ssl/client/ca/ca.crt":                                                                           Equal("--cacert=/var/etcd/ssl/client/ca/ca.crt"),
								"--server-cert=/var/etcd/ssl/client/server/tls.crt":                                                                 Equal("--server-cert=/var/etcd/ssl/client/server/tls.crt"),
								"--server-key=/var/etcd/ssl/client/server/tls.key":                                                                  Equal("--server-key=/var/etcd/ssl/client/server/tls.key"),
								"--data-dir=/var/etcd/data/new.etcd":                                                                                Equal("--data-dir=/var/etcd/data/new.etcd"),
								"--restoration-temp-snapshots-dir=/var/etcd/data/restoration.temp":                                                  Equal("--restoration-temp-snapshots-dir=/var/etcd/data/restoration.temp"),
								"--insecure-transport=false":                                                                                        Equal("--insecure-transport=false"),
								"--insecure-skip-tls-verify=false":                                                                                  Equal("--insecure-skip-tls-verify=false"),
								"--snapstore-temp-directory=/var/etcd/data/temp":                                                                    Equal("--snapstore-temp-directory=/var/etcd/data/temp"),
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
								fmt.Sprintf("--service-endpoints=https://%s:%d", instance.GetClientServiceName(), clientPort):                       Equal(fmt.Sprintf("--service-endpoints=https://%s:%d", instance.GetClientServiceName(), clientPort)),
								fmt.Sprintf("--embedded-etcd-quota-bytes=%d", instance.Spec.Etcd.Quota.Value()):                                     Equal(fmt.Sprintf("--embedded-etcd-quota-bytes=%d", instance.Spec.Etcd.Quota.Value())),
								fmt.Sprintf("%s=%s", "--delta-snapshot-period", instance.Spec.Backup.DeltaSnapshotPeriod.Duration.String()):         Equal(fmt.Sprintf("%s=%s", "--delta-snapshot-period", instance.Spec.Backup.DeltaSnapshotPeriod.Duration.String())),
								fmt.Sprintf("%s=%s", "--garbage-collection-period", instance.Spec.Backup.GarbageCollectionPeriod.Duration.String()): Equal(fmt.Sprintf("%s=%s", "--garbage-collection-period", instance.Spec.Backup.GarbageCollectionPeriod.Duration.String())),
								fmt.Sprintf("%s=%s", "--auto-compaction-mode", *instance.Spec.Common.AutoCompactionMode):                            Equal(fmt.Sprintf("%s=%s", "--auto-compaction-mode", autoCompactionMode)),
								fmt.Sprintf("%s=%s", "--auto-compaction-retention", *instance.Spec.Common.AutoCompactionRetention):                  Equal(fmt.Sprintf("%s=%s", "--auto-compaction-retention", autoCompactionRetention)),
								fmt.Sprintf("%s=%s", "--etcd-snapshot-timeout", instance.Spec.Backup.EtcdSnapshotTimeout.Duration.String()):         Equal(fmt.Sprintf("%s=%s", "--etcd-snapshot-timeout", instance.Spec.Backup.EtcdSnapshotTimeout.Duration.String())),
								fmt.Sprintf("%s=%s", "--etcd-defrag-timeout", instance.Spec.Etcd.EtcdDefragTimeout.Duration.String()):               Equal(fmt.Sprintf("%s=%s", "--etcd-defrag-timeout", instance.Spec.Etcd.EtcdDefragTimeout.Duration.String())),
								fmt.Sprintf("%s=%s", "--delta-snapshot-lease-name", instance.GetDeltaSnapshotLeaseName()):                           Equal(fmt.Sprintf("%s=%s", "--delta-snapshot-lease-name", instance.GetDeltaSnapshotLeaseName())),
								fmt.Sprintf("%s=%s", "--full-snapshot-lease-name", instance.GetFullSnapshotLeaseName()):                             Equal(fmt.Sprintf("%s=%s", "--full-snapshot-lease-name", instance.GetFullSnapshotLeaseName())),
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
							"VolumeMounts": MatchElements(testutils.VolumeMountIterator, IgnoreExtras, Elements{
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
									"MountPath": Equal("/home/nonroot/" + *instance.Spec.Backup.Store.Container),
								}),
							}),
							"Env": MatchElements(testutils.EnvIterator, IgnoreExtras, Elements{
								common.EnvPodName: MatchFields(IgnoreExtras, Fields{
									"Name": Equal(common.EnvPodName),
									"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
										"FieldRef": PointTo(MatchFields(IgnoreExtras, Fields{
											"FieldPath": Equal("metadata.name"),
										})),
									})),
								}),
								common.EnvPodNamespace: MatchFields(IgnoreExtras, Fields{
									"Name": Equal(common.EnvPodNamespace),
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
					"ShareProcessNamespace": Equal(pointer.Bool(true)),
					"Volumes": MatchAllElements(testutils.VolumeIterator, Elements{
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
									"DefaultMode": PointTo(Equal(int32(0640))),
									"Items": MatchAllElements(testutils.KeyIterator, Elements{
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
									"SecretName":  Equal(instance.Spec.Etcd.ClientUrlTLS.ServerTLSSecretRef.Name),
									"DefaultMode": PointTo(Equal(int32(0640))),
								})),
							}),
						}),
						"client-url-etcd-client-tls": MatchFields(IgnoreExtras, Fields{
							"Name": Equal("client-url-etcd-client-tls"),
							"VolumeSource": MatchFields(IgnoreExtras, Fields{
								"Secret": PointTo(MatchFields(IgnoreExtras, Fields{
									"SecretName":  Equal(instance.Spec.Etcd.ClientUrlTLS.ClientTLSSecretRef.Name),
									"DefaultMode": PointTo(Equal(int32(0640))),
								})),
							}),
						}),
						"client-url-ca-etcd": MatchFields(IgnoreExtras, Fields{
							"Name": Equal("client-url-ca-etcd"),
							"VolumeSource": MatchFields(IgnoreExtras, Fields{
								"Secret": PointTo(MatchFields(IgnoreExtras, Fields{
									"SecretName":  Equal(instance.Spec.Etcd.ClientUrlTLS.TLSCASecretRef.Name),
									"DefaultMode": PointTo(Equal(int32(0640))),
								})),
							}),
						}),
						"peer-url-etcd-server-tls": MatchFields(IgnoreExtras, Fields{
							"Name": Equal("peer-url-etcd-server-tls"),
							"VolumeSource": MatchFields(IgnoreExtras, Fields{
								"Secret": PointTo(MatchFields(IgnoreExtras, Fields{
									"SecretName":  Equal(instance.Spec.Etcd.PeerUrlTLS.ServerTLSSecretRef.Name),
									"DefaultMode": PointTo(Equal(int32(0640))),
								})),
							}),
						}),
						"peer-url-ca-etcd": MatchFields(IgnoreExtras, Fields{
							"Name": Equal("peer-url-ca-etcd"),
							"VolumeSource": MatchFields(IgnoreExtras, Fields{
								"Secret": PointTo(MatchFields(IgnoreExtras, Fields{
									"SecretName":  Equal(instance.Spec.Etcd.PeerUrlTLS.TLSCASecretRef.Name),
									"DefaultMode": PointTo(Equal(int32(0640))),
								})),
							}),
						}),
					}),
				}),
			}),
			"VolumeClaimTemplates": MatchAllElements(testutils.PVCIterator, Elements{
				*instance.Spec.VolumeClaimTemplate: MatchFields(IgnoreExtras, Fields{
					"ObjectMeta": MatchFields(IgnoreExtras, Fields{
						"Name": Equal(*instance.Spec.VolumeClaimTemplate),
					}),
					"Spec": MatchFields(IgnoreExtras, Fields{
						"StorageClassName": PointTo(Equal(*instance.Spec.StorageClass)),
						"AccessModes": MatchAllElements(testutils.AccessModeIterator, Elements{
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

func validateStoreGCP(instance *druidv1alpha1.Etcd, s *appsv1.StatefulSet, _ *corev1.ConfigMap, _ *corev1.Service, _ *corev1.Service) {

	Expect(*s).To(MatchFields(IgnoreExtras, Fields{
		"Spec": MatchFields(IgnoreExtras, Fields{
			"Template": MatchFields(IgnoreExtras, Fields{
				//s.Spec.Template.Spec.HostAliases
				"Spec": MatchFields(IgnoreExtras, Fields{
					"Containers": MatchElements(testutils.ContainerIterator, IgnoreExtras, Elements{
						backupRestore: MatchFields(IgnoreExtras, Fields{
							"Args": MatchElements(testutils.CmdIterator, IgnoreExtras, Elements{
								"--storage-provider=GCS": Equal("--storage-provider=GCS"),
								fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix): Equal(fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix)),
							}),
							"VolumeMounts": MatchElements(testutils.VolumeMountIterator, IgnoreExtras, Elements{
								"etcd-backup": MatchFields(IgnoreExtras, Fields{
									"Name":      Equal("etcd-backup"),
									"MountPath": Equal("/var/.gcp/"),
								}),
							}),
							"Env": MatchAllElements(testutils.EnvIterator, Elements{
								common.EnvStorageContainer: MatchFields(IgnoreExtras, Fields{
									"Name":  Equal(common.EnvStorageContainer),
									"Value": Equal(*instance.Spec.Backup.Store.Container),
								}),
								common.EnvPodName: MatchFields(IgnoreExtras, Fields{
									"Name": Equal(common.EnvPodName),
									"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
										"FieldRef": PointTo(MatchFields(IgnoreExtras, Fields{
											"FieldPath": Equal("metadata.name"),
										})),
									})),
								}),
								common.EnvPodNamespace: MatchFields(IgnoreExtras, Fields{
									"Name": Equal(common.EnvPodNamespace),
									"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
										"FieldRef": PointTo(MatchFields(IgnoreExtras, Fields{
											"FieldPath": Equal("metadata.namespace"),
										})),
									})),
								}),
								common.EnvGoogleApplicationCredentials: MatchFields(IgnoreExtras, Fields{
									"Name":  Equal(common.EnvGoogleApplicationCredentials),
									"Value": Equal("/var/.gcp/serviceaccount.json"),
								}),
								common.EnvGoogleStorageAPIEndpoint: MatchFields(IgnoreExtras, Fields{
									"Name": Equal(common.EnvGoogleStorageAPIEndpoint),
									"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
										"SecretKeyRef": PointTo(MatchFields(IgnoreExtras, Fields{
											"LocalObjectReference": MatchFields(IgnoreExtras, Fields{
												"Name": Equal(instance.Spec.Backup.Store.SecretRef.Name),
											}),
											"Key":      Equal("storageAPIEndpoint"),
											"Optional": Equal(pointer.Bool(true)),
										})),
									})),
								}),
							}),
						}),
					}),
					"Volumes": MatchElements(testutils.VolumeIterator, IgnoreExtras, Elements{
						"etcd-backup": MatchFields(IgnoreExtras, Fields{
							"Name": Equal("etcd-backup"),
							"VolumeSource": MatchFields(IgnoreExtras, Fields{
								"Secret": PointTo(MatchFields(IgnoreExtras, Fields{
									"SecretName":  Equal(instance.Spec.Backup.Store.SecretRef.Name),
									"DefaultMode": PointTo(Equal(int32(0640))),
								})),
							}),
						}),
					}),
				}),
			}),
		}),
	}))

}

func validateStoreAzure(instance *druidv1alpha1.Etcd, s *appsv1.StatefulSet, _ *corev1.ConfigMap, _ *corev1.Service, _ *corev1.Service) {
	Expect(*s).To(MatchFields(IgnoreExtras, Fields{
		"Spec": MatchFields(IgnoreExtras, Fields{
			"Template": MatchFields(IgnoreExtras, Fields{
				//s.Spec.Template.Spec.HostAliases
				"Spec": MatchFields(IgnoreExtras, Fields{
					"Containers": MatchElements(testutils.ContainerIterator, IgnoreExtras, Elements{
						backupRestore: MatchFields(IgnoreExtras, Fields{
							"Args": MatchElements(testutils.CmdIterator, IgnoreExtras, Elements{
								"--storage-provider=ABS": Equal("--storage-provider=ABS"),
								fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix): Equal(fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix)),
							}),
							"Env": MatchAllElements(testutils.EnvIterator, Elements{
								common.EnvStorageContainer: MatchFields(IgnoreExtras, Fields{
									"Name":  Equal(common.EnvStorageContainer),
									"Value": Equal(*instance.Spec.Backup.Store.Container),
								}),
								common.EnvPodName: MatchFields(IgnoreExtras, Fields{
									"Name": Equal(common.EnvPodName),
									"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
										"FieldRef": PointTo(MatchFields(IgnoreExtras, Fields{
											"FieldPath": Equal("metadata.name"),
										})),
									})),
								}),
								common.EnvPodNamespace: MatchFields(IgnoreExtras, Fields{
									"Name": Equal(common.EnvPodNamespace),
									"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
										"FieldRef": PointTo(MatchFields(IgnoreExtras, Fields{
											"FieldPath": Equal("metadata.namespace"),
										})),
									})),
								}),
								common.EnvAzureApplicationCredentials: MatchFields(IgnoreExtras, Fields{
									"Name":  Equal(common.EnvAzureApplicationCredentials),
									"Value": Equal("/var/etcd-backup"),
								}),
							}),
						}),
					}),
					"Volumes": MatchElements(testutils.VolumeIterator, IgnoreExtras, Elements{
						"etcd-backup": MatchFields(IgnoreExtras, Fields{
							"Name": Equal("etcd-backup"),
							"VolumeSource": MatchFields(IgnoreExtras, Fields{
								"Secret": PointTo(MatchFields(IgnoreExtras, Fields{
									"SecretName":  Equal(instance.Spec.Backup.Store.SecretRef.Name),
									"DefaultMode": PointTo(Equal(int32(0640))),
								})),
							}),
						}),
					}),
				}),
			}),
		}),
	}))
}

func validateStoreOpenstack(instance *druidv1alpha1.Etcd, s *appsv1.StatefulSet, _ *corev1.ConfigMap, _ *corev1.Service, _ *corev1.Service) {
	Expect(*s).To(MatchFields(IgnoreExtras, Fields{
		"Spec": MatchFields(IgnoreExtras, Fields{
			"Template": MatchFields(IgnoreExtras, Fields{
				//s.Spec.Template.Spec.HostAliases
				"Spec": MatchFields(IgnoreExtras, Fields{
					"Containers": MatchElements(testutils.ContainerIterator, IgnoreExtras, Elements{
						backupRestore: MatchFields(IgnoreExtras, Fields{
							"Args": MatchElements(testutils.CmdIterator, IgnoreExtras, Elements{
								"--storage-provider=Swift": Equal("--storage-provider=Swift"),
								fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix): Equal(fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix)),
							}),
							"Env": MatchAllElements(testutils.EnvIterator, Elements{
								common.EnvStorageContainer: MatchFields(IgnoreExtras, Fields{
									"Name":  Equal(common.EnvStorageContainer),
									"Value": Equal(*instance.Spec.Backup.Store.Container),
								}),
								common.EnvPodName: MatchFields(IgnoreExtras, Fields{
									"Name": Equal(common.EnvPodName),
									"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
										"FieldRef": PointTo(MatchFields(IgnoreExtras, Fields{
											"FieldPath": Equal("metadata.name"),
										})),
									})),
								}),
								common.EnvPodNamespace: MatchFields(IgnoreExtras, Fields{
									"Name": Equal(common.EnvPodNamespace),
									"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
										"FieldRef": PointTo(MatchFields(IgnoreExtras, Fields{
											"FieldPath": Equal("metadata.namespace"),
										})),
									})),
								}),
								common.EnvOpenstackApplicationCredentials: MatchFields(IgnoreExtras, Fields{
									"Name":  Equal(common.EnvOpenstackApplicationCredentials),
									"Value": Equal("/var/etcd-backup"),
								}),
							}),
						}),
					}),
					"Volumes": MatchElements(testutils.VolumeIterator, IgnoreExtras, Elements{
						"etcd-backup": MatchFields(IgnoreExtras, Fields{
							"Name": Equal("etcd-backup"),
							"VolumeSource": MatchFields(IgnoreExtras, Fields{
								"Secret": PointTo(MatchFields(IgnoreExtras, Fields{
									"SecretName":  Equal(instance.Spec.Backup.Store.SecretRef.Name),
									"DefaultMode": PointTo(Equal(int32(0640))),
								})),
							}),
						}),
					}),
				}),
			}),
		}),
	}))
}

func validateStoreAlicloud(instance *druidv1alpha1.Etcd, s *appsv1.StatefulSet, _ *corev1.ConfigMap, _ *corev1.Service, _ *corev1.Service) {
	Expect(*s).To(MatchFields(IgnoreExtras, Fields{
		"Spec": MatchFields(IgnoreExtras, Fields{
			"Template": MatchFields(IgnoreExtras, Fields{
				//s.Spec.Template.Spec.HostAliases
				"Spec": MatchFields(IgnoreExtras, Fields{
					"Containers": MatchElements(testutils.ContainerIterator, IgnoreExtras, Elements{

						backupRestore: MatchFields(IgnoreExtras, Fields{
							"Args": MatchElements(testutils.CmdIterator, IgnoreExtras, Elements{
								"--storage-provider=OSS": Equal("--storage-provider=OSS"),
								fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix): Equal(fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix)),
							}),
							"ImagePullPolicy": Equal(corev1.PullIfNotPresent),
							"Env": MatchAllElements(testutils.EnvIterator, Elements{
								common.EnvStorageContainer: MatchFields(IgnoreExtras, Fields{
									"Name":  Equal(common.EnvStorageContainer),
									"Value": Equal(*instance.Spec.Backup.Store.Container),
								}),
								common.EnvPodName: MatchFields(IgnoreExtras, Fields{
									"Name": Equal(common.EnvPodName),
									"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
										"FieldRef": PointTo(MatchFields(IgnoreExtras, Fields{
											"FieldPath": Equal("metadata.name"),
										})),
									})),
								}),
								common.EnvPodNamespace: MatchFields(IgnoreExtras, Fields{
									"Name": Equal(common.EnvPodNamespace),
									"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
										"FieldRef": PointTo(MatchFields(IgnoreExtras, Fields{
											"FieldPath": Equal("metadata.namespace"),
										})),
									})),
								}),
								common.EnvAlicloudApplicationCredentials: MatchFields(IgnoreExtras, Fields{
									"Name":  Equal(common.EnvAlicloudApplicationCredentials),
									"Value": Equal("/var/etcd-backup"),
								}),
							}),
						}),
					}),
					"Volumes": MatchElements(testutils.VolumeIterator, IgnoreExtras, Elements{
						"etcd-backup": MatchFields(IgnoreExtras, Fields{
							"Name": Equal("etcd-backup"),
							"VolumeSource": MatchFields(IgnoreExtras, Fields{
								"Secret": PointTo(MatchFields(IgnoreExtras, Fields{
									"SecretName":  Equal(instance.Spec.Backup.Store.SecretRef.Name),
									"DefaultMode": PointTo(Equal(int32(0640))),
								})),
							}),
						}),
					}),
				}),
			}),
		}),
	}))
}

func validateStoreAWS(instance *druidv1alpha1.Etcd, s *appsv1.StatefulSet, _ *corev1.ConfigMap, _ *corev1.Service, _ *corev1.Service) {
	Expect(*s).To(MatchFields(IgnoreExtras, Fields{
		"Spec": MatchFields(IgnoreExtras, Fields{
			"Template": MatchFields(IgnoreExtras, Fields{
				//s.Spec.Template.Spec.HostAliases
				"Spec": MatchFields(IgnoreExtras, Fields{
					"Containers": MatchElements(testutils.ContainerIterator, IgnoreExtras, Elements{

						backupRestore: MatchFields(IgnoreExtras, Fields{
							"Args": MatchElements(testutils.CmdIterator, IgnoreExtras, Elements{
								"--storage-provider=S3": Equal("--storage-provider=S3"),
								fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix): Equal(fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix)),
							}),
							"ImagePullPolicy": Equal(corev1.PullIfNotPresent),
							"Env": MatchAllElements(testutils.EnvIterator, Elements{
								common.EnvStorageContainer: MatchFields(IgnoreExtras, Fields{
									"Name":  Equal(common.EnvStorageContainer),
									"Value": Equal(*instance.Spec.Backup.Store.Container),
								}),
								common.EnvPodName: MatchFields(IgnoreExtras, Fields{
									"Name": Equal(common.EnvPodName),
									"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
										"FieldRef": PointTo(MatchFields(IgnoreExtras, Fields{
											"FieldPath": Equal("metadata.name"),
										})),
									})),
								}),
								common.EnvPodNamespace: MatchFields(IgnoreExtras, Fields{
									"Name": Equal(common.EnvPodNamespace),
									"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
										"FieldRef": PointTo(MatchFields(IgnoreExtras, Fields{
											"FieldPath": Equal("metadata.namespace"),
										})),
									})),
								}),
								common.EnvAWSApplicationCredentials: MatchFields(IgnoreExtras, Fields{
									"Name":  Equal(common.EnvAWSApplicationCredentials),
									"Value": Equal("/var/etcd-backup"),
								}),
							}),
						}),
					}),
					"Volumes": MatchElements(testutils.VolumeIterator, IgnoreExtras, Elements{
						"etcd-backup": MatchFields(IgnoreExtras, Fields{
							"Name": Equal("etcd-backup"),
							"VolumeSource": MatchFields(IgnoreExtras, Fields{
								"Secret": PointTo(MatchFields(IgnoreExtras, Fields{
									"SecretName":  Equal(instance.Spec.Backup.Store.SecretRef.Name),
									"DefaultMode": PointTo(Equal(int32(0640))),
								})),
							}),
						}),
					}),
				}),
			}),
		}),
	}))
}

var _ = Describe("buildPredicate", func() {
	var (
		instance                          *druidv1alpha1.Etcd
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
		instance = &druidv1alpha1.Etcd{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{},
			},
		}
	})

	DescribeTable(
		"with ignoreOperationAnnotation true",
		func(evalFn func(p predicate.Predicate, obj client.Object) bool, expect bool) {
			Expect(evalFn(etcd.BuildPredicate(true), instance)).To(Equal(expect))
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
				Expect(evalFn(etcd.BuildPredicate(false), instance)).To(Equal(expect))
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
				instance.Annotations[v1beta1constants.GardenerOperation] = v1beta1constants.GardenerOperationReconcile
				Expect(evalFn(etcd.BuildPredicate(false), instance)).To(Equal(expect))
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
				instance.Status.LastError = pointer.String("error")
				Expect(evalFn(etcd.BuildPredicate(false), instance)).To(Equal(expect))
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
				instance.DeletionTimestamp = &now
				Expect(evalFn(etcd.BuildPredicate(false), instance)).To(Equal(expect))
			},
			Entry("Create should match", evalCreate, true),
			Entry("Delete should match", evalDelete, true),
			Entry("Generic should match", evalGeneric, true),
			Entry("Update without generation change should match", evalUpdateWithoutGenerationChange, true),
			Entry("Update with generation change should match", evalUpdateWithGenerationChange, true),
		)
	})
})
