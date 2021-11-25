// Copyright (c) 2021 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package service_test

import (
	"context"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/pkg/client/kubernetes"
	. "github.com/gardener/etcd-druid/pkg/component/etcd/service"

	"github.com/gardener/gardener/pkg/operation/botanist/component"
	kutil "github.com/gardener/gardener/pkg/utils/kubernetes"
	. "github.com/gardener/gardener/pkg/utils/test/matchers"
	. "github.com/onsi/ginkgo"
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
		labels                             map[string]string

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
		labels = map[string]string{
			"foo": "bar",
		}

		etcd = &druidv1alpha1.Etcd{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				UID:       uid,
			},
			Spec: druidv1alpha1.EtcdSpec{
				Selector: metav1.SetAsLabelSelector(labels),
				Backup: druidv1alpha1.BackupSpec{
					Port: pointer.Int32Ptr(backupPort),
				},
				Etcd: druidv1alpha1.EtcdConfig{
					ClientPort: pointer.Int32Ptr(clientPort),
					ServerPort: pointer.Int32Ptr(serverPort),
				},
			},
		}

		values = GenerateValues(etcd)
		serviceDeployer = New(cl, namespace, values)
	})

	Describe("#Deploy", func() {
		Context("when service does not exist", func() {
			It("should create the service successfully", func() {
				Expect(serviceDeployer.Deploy(ctx)).To(Succeed())

				svc := &corev1.Service{}
				Expect(cl.Get(ctx, kutil.Key(namespace, values.ClientServiceName), svc)).To(Succeed())
				checkService(svc, values)
			})
		})

		Context("when service exists", func() {
			It("should update the service successfully", func() {
				existingService := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      values.ClientServiceName,
						Namespace: values.EtcdName,
					},
				}
				Expect(cl.Create(ctx, existingService)).To(Succeed())
				Expect(serviceDeployer.Deploy(ctx)).To(Succeed())

				svc := &corev1.Service{}
				Expect(cl.Get(ctx, kutil.Key(namespace, values.ClientServiceName), svc)).To(Succeed())
				checkService(svc, values)
			})
		})
	})

	Describe("#Deploy", func() {
		Context("when service does not exist", func() {
			It("should destroy successfully", func() {
				Expect(serviceDeployer.Destroy(ctx)).To(Succeed())
				Expect(cl.Get(ctx, kutil.Key(namespace, values.ClientServiceName), &corev1.Service{})).To(BeNotFoundError())
			})
		})

		Context("when service exists", func() {
			It("should destroy successfully", func() {
				existingService := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      values.ClientServiceName,
						Namespace: values.EtcdName,
					},
				}
				Expect(cl.Create(ctx, existingService)).To(Succeed())
				Expect(serviceDeployer.Destroy(ctx)).To(Succeed())
				Expect(cl.Get(ctx, kutil.Key(namespace, values.ClientServiceName), &corev1.Service{})).To(BeNotFoundError())
			})
		})
	})
})

func checkService(svc *corev1.Service, values Values) {
	Expect(svc.OwnerReferences).To(ConsistOf(Equal(metav1.OwnerReference{
		APIVersion:         druidv1alpha1.GroupVersion.String(),
		Kind:               "Etcd",
		Name:               values.EtcdName,
		UID:                values.EtcdUID,
		Controller:         pointer.BoolPtr(true),
		BlockOwnerDeletion: pointer.BoolPtr(true),
	})))
	Expect(svc.Labels).To(Equal(serviceLabels(values)))
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
			Port:       values.ServerPort,
			TargetPort: intstr.FromInt(int(values.ServerPort)),
		}),
		Equal(corev1.ServicePort{
			Name:       "backuprestore",
			Protocol:   corev1.ProtocolTCP,
			Port:       values.BackupPort,
			TargetPort: intstr.FromInt(int(values.BackupPort)),
		}),
	))
}

func serviceLabels(val Values) map[string]string {
	labels := map[string]string{
		"instance": val.EtcdName,
	}

	for k, v := range val.Labels {
		labels[k] = v
	}

	return labels
}
