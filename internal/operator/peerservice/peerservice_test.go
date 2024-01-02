// Copyright 2023 SAP SE or an SAP affiliate company
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package peerservice

import (
	"context"
	"fmt"
	"testing"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/operator/resource"
	"github.com/gardener/etcd-druid/internal/utils"
	mockclient "github.com/gardener/etcd-druid/pkg/mock/controller-runtime/client"
	"github.com/go-logr/logr"
	"github.com/golang/mock/gomock"
	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// ------------------------ GetExistingResourceNames ------------------------
func TestGetExistingResourceNames_WithExistingService(t *testing.T) {
	g, ctx, etcd, _, op := setupWithFakeClient(t, true)

	err := op.Sync(ctx, etcd)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	names, err := op.GetExistingResourceNames(ctx, etcd)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(names).To(gomega.ContainElement(etcd.GetPeerServiceName()))

}

func TestGetExistingResourceNames_ServiceNotFound(t *testing.T) {
	g, ctx, etcd, _, op := setupWithFakeClient(t, false)

	names, err := op.GetExistingResourceNames(ctx, etcd)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(names).To(gomega.BeEmpty())
}

func TestGetExistingResourceNames_WithError(t *testing.T) {
	g, ctx, etcd, cl, op := setupWithMockClient(t)
	cl.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(fmt.Errorf("fake get error")).AnyTimes()

	names, err := op.GetExistingResourceNames(ctx, etcd)
	g.Expect(err).To(gomega.HaveOccurred())
	g.Expect(names).To(gomega.BeEmpty())
}

// ----------------------------------- Sync -----------------------------------

func TestSync_CreateNewService(t *testing.T) {
	g, ctx, etcd, cl, op := setupWithFakeClient(t, false)
	err := op.Sync(ctx, etcd)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      etcd.GetPeerServiceName(),
			Namespace: etcd.Namespace,
		},
	}
	err = cl.Get(ctx, client.ObjectKeyFromObject(service), service)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	checkPeerService(g, service, etcd)

}

func TestSync_UpdateExistingService(t *testing.T) {
	g, ctx, etcd, cl, op := setupWithFakeClient(t, true)
	etcd.Spec.Etcd.ServerPort = nil

	err := op.Sync(ctx, etcd)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      etcd.GetPeerServiceName(),
			Namespace: etcd.Namespace,
		},
	}
	err = cl.Get(ctx, client.ObjectKeyFromObject(service), service)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(service).ToNot(gomega.BeNil())
	checkPeerService(g, service, etcd)
}

func TestSync_WithClientError(t *testing.T) {
	g, ctx, etcd, cl, op := setupWithMockClient(t)
	cl.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(fmt.Errorf("fake get error")).AnyTimes()
	cl.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any()).Return(fmt.Errorf("fake create error")).AnyTimes()
	cl.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any()).Return(fmt.Errorf("fake update error")).AnyTimes()

	err := op.Sync(ctx, etcd)
	g.Expect(err).To(gomega.HaveOccurred())
}

// ----------------------------- TriggerDelete -------------------------------

func TestTriggerDelete_ExistingService(t *testing.T) {
	g, ctx, etcd, cl, op := setupWithFakeClient(t, true)
	err := op.TriggerDelete(ctx, etcd)
	g.Expect(err).NotTo(gomega.HaveOccurred())

	serviceList := &corev1.List{}
	err = cl.List(ctx, serviceList)
	g.Expect(err).NotTo(gomega.HaveOccurred())
	g.Expect(serviceList.Items).To(gomega.BeEmpty())
}

func TestTriggerDelete_ServiceNotFound(t *testing.T) {
	g, ctx, etcd, _, op := setupWithFakeClient(t, false)

	err := op.TriggerDelete(ctx, etcd)
	g.Expect(err).NotTo(gomega.HaveOccurred())
}

func TestTriggerDelete_WithClientError(t *testing.T) {
	g, ctx, etcd, cl, op := setupWithMockClient(t)
	// Configure the mock client to return an error on delete operation
	cl.EXPECT().Delete(gomock.Any(), gomock.Any(), gomock.Any()).Return(fmt.Errorf("fake delete error")).AnyTimes()

	err := op.TriggerDelete(ctx, etcd)
	g.Expect(err).To(gomega.HaveOccurred())
}

// ---------------------------- Helper Functions -----------------------------

func sampleEtcd() *druidv1alpha1.Etcd {
	return &druidv1alpha1.Etcd{

		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-etcd",
			Namespace: "test-namespace",
			UID:       "xxx-yyy-zzz",
		},
		Spec: druidv1alpha1.EtcdSpec{

			Backup: druidv1alpha1.BackupSpec{
				Port: pointer.Int32(1111),
			},
			Etcd: druidv1alpha1.EtcdConfig{
				ClientPort: pointer.Int32(2222),
				ServerPort: pointer.Int32(3333),
			},
		},
	}
}

func checkPeerService(g *gomega.WithT, svc *corev1.Service, etcd *druidv1alpha1.Etcd) {
	peerPort := utils.TypeDeref[int32](etcd.Spec.Etcd.ServerPort, defaultServerPort)
	g.Expect(svc.OwnerReferences).To(gomega.Equal([]metav1.OwnerReference{etcd.GetAsOwnerReference()}))
	g.Expect(svc.Labels).To(gomega.Equal(etcd.GetDefaultLabels()))
	g.Expect(svc.Spec.PublishNotReadyAddresses).To(gomega.BeTrue())
	g.Expect(svc.Spec.Type).To(gomega.Equal(corev1.ServiceTypeClusterIP))
	g.Expect(svc.Spec.ClusterIP).To(gomega.Equal(corev1.ClusterIPNone))
	g.Expect(svc.Spec.SessionAffinity).To(gomega.Equal(corev1.ServiceAffinityNone))
	g.Expect(svc.Spec.Selector).To(gomega.Equal(etcd.GetDefaultLabels()))
	g.Expect(svc.Spec.Ports).To(gomega.ConsistOf(
		gomega.Equal(corev1.ServicePort{
			Name:       "peer",
			Protocol:   corev1.ProtocolTCP,
			Port:       peerPort,
			TargetPort: intstr.FromInt(int(peerPort)),
		}),
	))
}

func existingService(etcd *druidv1alpha1.Etcd) *corev1.Service {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      etcd.GetPeerServiceName(),
			Namespace: etcd.Namespace,
			Labels:    etcd.GetDefaultLabels(),
			OwnerReferences: []metav1.OwnerReference{
				etcd.GetAsOwnerReference(),
			},
		},
		Spec: corev1.ServiceSpec{
			Type:                     corev1.ServiceTypeClusterIP,
			ClusterIP:                corev1.ClusterIPNone,
			SessionAffinity:          corev1.ServiceAffinityNone,
			Selector:                 etcd.GetDefaultLabels(),
			PublishNotReadyAddresses: true,
			Ports: []corev1.ServicePort{
				{
					Name:       "peer",
					Protocol:   corev1.ProtocolTCP,
					Port:       *etcd.Spec.Etcd.ServerPort,
					TargetPort: intstr.FromInt(int(*etcd.Spec.Etcd.ServerPort)),
				},
			},
		},
	}
	return service
}

func setupWithFakeClient(t *testing.T, withExistingService bool) (g *gomega.WithT, ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd, cl client.Client, op resource.Operator) {
	g = gomega.NewWithT(t)
	ctx = resource.NewOperatorContext(context.TODO(), logr.Logger{}, "xxx-yyyy-zzzz")
	etcd = sampleEtcd()
	if withExistingService {
		cl = fakeclient.NewClientBuilder().WithObjects(existingService(etcd)).Build()
	} else {

		cl = fakeclient.NewClientBuilder().Build()
	}
	op = New(cl)
	return
}

func setupWithMockClient(t *testing.T) (g *gomega.WithT, ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd, cl *mockclient.MockClient, op resource.Operator) {
	g = gomega.NewWithT(t)
	ctx = resource.NewOperatorContext(context.TODO(), logr.Logger{}, "xxx-yyyy-zzzz")
	etcd = sampleEtcd()
	cl = mockclient.NewMockClient(gomock.NewController(t))
	op = New(cl)
	return
}
