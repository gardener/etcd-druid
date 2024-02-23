// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package service_test

import (
	"context"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/pkg/client/kubernetes"
	. "github.com/gardener/etcd-druid/pkg/component/etcd/service"
	"github.com/gardener/etcd-druid/pkg/utils"

	"github.com/gardener/gardener/pkg/component"
	kutil "github.com/gardener/gardener/pkg/utils/kubernetes"
	. "github.com/gardener/gardener/pkg/utils/test/matchers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("Service", func() {
	var (
		ctx context.Context
		cl  client.Client

		etcd                               *druidv1alpha1.Etcd
		backupPort, clientPort, serverPort int32
		namespace                          string
		name                               string
		uid                                types.UID
		selectors                          map[string]string
		services                           []*corev1.Service

		values          Values
		serviceDeployer component.Deployer
	)

	BeforeEach(func() {
		ctx = context.Background()
		namespace = "default"
		cl = fakeclient.NewClientBuilder().WithScheme(kubernetes.Scheme).Build()

		backupPort = 1111
		clientPort = 2222
		serverPort = 3333

		selectors = map[string]string{
			"foo": "bar",
			"baz": "qux",
		}

		etcd = &druidv1alpha1.Etcd{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				UID:       uid,
			},
			Spec: druidv1alpha1.EtcdSpec{
				Selector: metav1.SetAsLabelSelector(selectors),
				Backup: druidv1alpha1.BackupSpec{
					Port: pointer.Int32(backupPort),
				},
				Etcd: druidv1alpha1.EtcdConfig{
					ClientPort: pointer.Int32(clientPort),
					ServerPort: pointer.Int32(serverPort),
				},
			},
		}

		values = GenerateValues(etcd)

		services = []*corev1.Service{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      values.ClientServiceName,
					Namespace: namespace,
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      values.PeerServiceName,
					Namespace: namespace,
				},
			},
		}

		serviceDeployer = New(cl, namespace, values)
	})

	Describe("#Deploy", func() {
		Context("when services do not exist", func() {
			It("should create the service successfully", func() {
				Expect(serviceDeployer.Deploy(ctx)).To(Succeed())

				svc := &corev1.Service{}

				Expect(cl.Get(ctx, kutil.Key(namespace, values.ClientServiceName), svc)).To(Succeed())
				checkClientService(svc, values)

				Expect(cl.Get(ctx, kutil.Key(namespace, values.PeerServiceName), svc)).To(Succeed())
				checkPeerService(svc, values)
			})
		})

		Context("when services exist", func() {
			It("should update the service successfully", func() {
				for _, svc := range services {
					Expect(cl.Create(ctx, svc)).To(Succeed())
				}

				Expect(serviceDeployer.Deploy(ctx)).To(Succeed())

				svc := &corev1.Service{}

				Expect(cl.Get(ctx, kutil.Key(namespace, values.ClientServiceName), svc)).To(Succeed())
				checkClientService(svc, values)

				Expect(cl.Get(ctx, kutil.Key(namespace, values.PeerServiceName), svc)).To(Succeed())
				checkPeerService(svc, values)
			})
		})
	})

	Describe("#Destroy", func() {
		Context("when services do not exist", func() {
			It("should destroy successfully", func() {
				Expect(serviceDeployer.Destroy(ctx)).To(Succeed())
				for _, svc := range services {
					Expect(cl.Get(ctx, client.ObjectKeyFromObject(svc), &corev1.Service{})).To(BeNotFoundError())
				}
			})
		})

		Context("when services exist", func() {
			It("should destroy successfully", func() {
				for _, svc := range services {
					Expect(cl.Create(ctx, svc)).To(Succeed())
				}

				Expect(serviceDeployer.Destroy(ctx)).To(Succeed())

				for _, svc := range services {
					Expect(cl.Get(ctx, kutil.Key(namespace, svc.Name), &corev1.Service{})).To(BeNotFoundError())
				}
			})
		})
	})
})

func checkClientService(svc *corev1.Service, values Values) {
	Expect(svc.OwnerReferences).To(Equal([]metav1.OwnerReference{values.OwnerReference}))
	Expect(svc.Labels).To(Equal(utils.MergeStringMaps(values.Labels, values.ClientServiceLabels)))
	Expect(svc.Spec.Selector).To(Equal(values.SelectorLabels))
	Expect(svc.Spec.Type).To(Equal(corev1.ServiceType("ClusterIP")))
	Expect(svc.Spec.Ports).To(ConsistOf(
		Equal(corev1.ServicePort{
			Name:       "client",
			Protocol:   corev1.ProtocolTCP,
			Port:       values.ClientPort,
			TargetPort: intstr.FromInt(int(values.ClientPort)),
		}),
		Equal(corev1.ServicePort{
			Name:       "server",
			Protocol:   corev1.ProtocolTCP,
			Port:       values.PeerPort,
			TargetPort: intstr.FromInt(int(values.PeerPort)),
		}),
		Equal(corev1.ServicePort{
			Name:       "backuprestore",
			Protocol:   corev1.ProtocolTCP,
			Port:       values.BackupPort,
			TargetPort: intstr.FromInt(int(values.BackupPort)),
		}),
	))
}

func checkPeerService(svc *corev1.Service, values Values) {
	Expect(svc.OwnerReferences).To(Equal([]metav1.OwnerReference{values.OwnerReference}))
	Expect(svc.Labels).To(Equal(values.Labels))
	Expect(svc.Spec.PublishNotReadyAddresses).To(BeTrue())
	Expect(svc.Spec.Type).To(Equal(corev1.ServiceType("ClusterIP")))
	Expect(svc.Spec.ClusterIP).To(Equal(("None")))
	Expect(svc.Spec.Selector).To(Equal(values.SelectorLabels))
	Expect(svc.Spec.Ports).To(ConsistOf(
		Equal(corev1.ServicePort{
			Name:       "peer",
			Protocol:   corev1.ProtocolTCP,
			Port:       values.PeerPort,
			TargetPort: intstr.FromInt(int(values.PeerPort)),
		}),
	))
}
