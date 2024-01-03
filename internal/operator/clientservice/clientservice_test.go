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

package clientservice

import (
	"context"
	"errors"
	"testing"

	"github.com/gardener/etcd-druid/internal/operator/resource"
	"github.com/gardener/etcd-druid/test/sample"
	testutils "github.com/gardener/etcd-druid/test/utils"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// ------------------------ GetExistingResourceNames ------------------------

const (
	testEtcdName = "test-etcd"
	testNs       = "test-namespace"
)

func TestGetExistingResourceNames(t *testing.T) {
	internalErr := errors.New("test internal error")
	etcd := testutils.EtcdBuilderWithDefaults(testEtcdName, testNs).Build()
	t.Parallel()
	testcases := []struct {
		name                 string
		svcExists            bool
		getErr               *apierrors.StatusError
		expectedErr          error
		expectedServiceNames []string
	}{
		{
			"should return the existing service name",
			true,
			nil,
			nil,
			[]string{etcd.GetClientServiceName()},
		},
		{
			"should return empty slice when service is not found",
			false,
			apierrors.NewNotFound(corev1.Resource("services"), ""),
			nil,
			[]string{},
		},
		{
			"should return error when get fails",
			true,
			apierrors.NewInternalError(internalErr),
			apierrors.NewInternalError(internalErr),
			nil,
		},
	}

	g := NewWithT(t)

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			fakeClientBuilder := testutils.NewFakeClientBuilder()
			if tc.getErr != nil {
				fakeClientBuilder.WithGetError(tc.getErr)
			}
			if tc.svcExists {
				fakeClientBuilder.WithObjects(sample.NewClientService(etcd))
			}
			operator := New(fakeClientBuilder.Build())
			opCtx := resource.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
			svcNames, err := operator.GetExistingResourceNames(opCtx, etcd)
			if tc.expectedErr != nil {
				g.Expect(errors.Is(err, tc.getErr)).To(BeTrue())
			} else {
				g.Expect(err).To(BeNil())
			}
			g.Expect(svcNames, tc.expectedServiceNames)
		})
	}
}

//func TestClientServiceGetExistingResourceNames_WithExistingService(t *testing.T) {
//	g, ctx, etcd, _, op := setupWithFakeClient(t, true)
//
//	err := op.Sync(ctx, etcd)
//	g.Expect(err).NotTo(gomega.HaveOccurred())
//
//	names, err := op.GetExistingResourceNames(ctx, etcd)
//	g.Expect(err).NotTo(gomega.HaveOccurred())
//	g.Expect(names).To(gomega.ContainElement(etcd.GetClientServiceName()))
//}
//
//func TestClientServiceGetExistingResourceNames_ServiceNotFound(t *testing.T) {
//	g, ctx, etcd, _, op := setupWithFakeClient(t, false)
//
//	names, err := op.GetExistingResourceNames(ctx, etcd)
//	g.Expect(err).NotTo(gomega.HaveOccurred())
//	g.Expect(names).To(gomega.BeEmpty())
//}
//
//func TestClientServiceGetExistingResourceNames_WithError(t *testing.T) {
//	g, ctx, etcd, cl, op := setupWithMockClient(t)
//	cl.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(fmt.Errorf("fake get error")).AnyTimes()
//
//	names, err := op.GetExistingResourceNames(ctx, etcd)
//	g.Expect(err).To(gomega.HaveOccurred())
//	g.Expect(names).To(gomega.BeEmpty())
//}
//
//// ----------------------------------- Sync -----------------------------------
//
//func TestClientServiceSync_CreateNewService(t *testing.T) {
//	g, ctx, etcd, cl, op := setupWithFakeClient(t, false)
//	err := op.Sync(ctx, etcd)
//	g.Expect(err).NotTo(gomega.HaveOccurred())
//
//	service := &corev1.Service{
//		ObjectMeta: metav1.ObjectMeta{
//			Name:      etcd.GetClientServiceName(),
//			Namespace: etcd.Namespace,
//		},
//	}
//	err = cl.Get(ctx, client.ObjectKeyFromObject(service), service)
//	g.Expect(err).NotTo(gomega.HaveOccurred())
//	checkClientService(g, service, etcd)
//}
//
//func TestClientServiceSync_UpdateExistingService(t *testing.T) {
//	g, ctx, etcd, cl, op := setupWithFakeClient(t, true)
//	etcd.Spec.Etcd.ServerPort = nil
//	etcd.Spec.Etcd.ClientPort = nil
//	etcd.Spec.Backup.Port = nil
//
//	etcd.Spec.Etcd.ClientService = &druidv1alpha1.ClientService{
//		Labels:      map[string]string{"testingKey": "testingValue"},
//		Annotations: map[string]string{"testingAnnotationKey": "testingValue"},
//	}
//
//	err := op.Sync(ctx, etcd)
//	g.Expect(err).NotTo(gomega.HaveOccurred())
//
//	service := &corev1.Service{
//		ObjectMeta: metav1.ObjectMeta{
//			Name:      etcd.GetClientServiceName(),
//			Namespace: etcd.Namespace,
//		},
//	}
//	err = cl.Get(ctx, client.ObjectKeyFromObject(service), service)
//	g.Expect(err).NotTo(gomega.HaveOccurred())
//	g.Expect(service).ToNot(gomega.BeNil())
//	checkClientService(g, service, etcd)
//}
//
//func TestClientServiceSync_WithClientError(t *testing.T) {
//	g, ctx, etcd, cl, op := setupWithMockClient(t)
//	cl.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Return(fmt.Errorf("fake get error")).AnyTimes()
//	cl.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any()).Return(fmt.Errorf("fake create error")).AnyTimes()
//	cl.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any()).Return(fmt.Errorf("fake update error")).AnyTimes()
//
//	err := op.Sync(ctx, etcd)
//	g.Expect(err).To(gomega.HaveOccurred())
//	g.Expect(err.Error()).To(gomega.ContainSubstring(string(ErrSyncingClientService)))
//}
//
//// ----------------------------- TriggerDelete -------------------------------
//
//func TestClientServiceTriggerDelete_ExistingService(t *testing.T) {
//	g, ctx, etcd, cl, op := setupWithFakeClient(t, true)
//	err := op.TriggerDelete(ctx, etcd)
//	g.Expect(err).NotTo(gomega.HaveOccurred())
//
//	serviceList := &corev1.List{}
//	err = cl.List(ctx, serviceList)
//	g.Expect(err).NotTo(gomega.HaveOccurred())
//	g.Expect(serviceList.Items).To(gomega.BeEmpty())
//}
//
//func TestClientServiceTriggerDelete_ServiceNotFound(t *testing.T) {
//	g, ctx, etcd, _, op := setupWithFakeClient(t, false)
//
//	err := op.TriggerDelete(ctx, etcd)
//	g.Expect(err).NotTo(gomega.HaveOccurred())
//}
//
//func TestClientServiceTriggerDelete_WithClientError(t *testing.T) {
//	g, ctx, etcd, cl, op := setupWithMockClient(t)
//	// Configure the mock client to return an error on delete operation
//	cl.EXPECT().Delete(gomock.Any(), gomock.Any(), gomock.Any()).Return(fmt.Errorf("fake delete error")).AnyTimes()
//
//	err := op.TriggerDelete(ctx, etcd)
//	g.Expect(err).To(gomega.HaveOccurred())
//	g.Expect(err.Error()).To(gomega.ContainSubstring(string(ErrDeletingClientService)))
//}
//
//// ---------------------------- Helper Functions -----------------------------
//
//func sampleEtcd() *druidv1alpha1.Etcd {
//	return &druidv1alpha1.Etcd{
//
//		ObjectMeta: metav1.ObjectMeta{
//			Name:      "test-etcd",
//			Namespace: "test-namespace",
//			UID:       "xxx-yyy-zzz",
//		},
//		Spec: druidv1alpha1.EtcdSpec{
//
//			Backup: druidv1alpha1.BackupSpec{
//				Port: pointer.Int32(1111),
//			},
//			Etcd: druidv1alpha1.EtcdConfig{
//				ClientPort: pointer.Int32(2222),
//				ServerPort: pointer.Int32(3333),
//			},
//		},
//	}
//}
//
//func checkClientService(g *gomega.WithT, svc *corev1.Service, etcd *druidv1alpha1.Etcd) {
//	clientPort := utils.TypeDeref[int32](etcd.Spec.Etcd.ClientPort, defaultClientPort)
//	backupPort := utils.TypeDeref[int32](etcd.Spec.Backup.Port, defaultBackupPort)
//	peerPort := utils.TypeDeref[int32](etcd.Spec.Etcd.ServerPort, defaultServerPort)
//
//	expectedLabels := etcd.GetDefaultLabels()
//	var expectedAnnotations map[string]string
//	if etcd.Spec.Etcd.ClientService != nil {
//		expectedAnnotations = etcd.Spec.Etcd.ClientService.Annotations
//		expectedLabels = utils.MergeMaps[string](etcd.Spec.Etcd.ClientService.Labels, etcd.GetDefaultLabels())
//	}
//	g.Expect(svc.OwnerReferences).To(gomega.Equal([]metav1.OwnerReference{etcd.GetAsOwnerReference()}))
//	g.Expect(svc.Annotations).To(gomega.Equal(expectedAnnotations))
//	g.Expect(svc.Labels).To(gomega.Equal(expectedLabels))
//	g.Expect(svc.Spec.Type).To(gomega.Equal(corev1.ServiceTypeClusterIP))
//	g.Expect(svc.Spec.SessionAffinity).To(gomega.Equal(corev1.ServiceAffinityNone))
//	g.Expect(svc.Spec.Selector).To(gomega.Equal(etcd.GetDefaultLabels()))
//	g.Expect(svc.Spec.Ports).To(gomega.ConsistOf(
//		gomega.Equal(corev1.ServicePort{
//			Name:       "client",
//			Protocol:   corev1.ProtocolTCP,
//			Port:       clientPort,
//			TargetPort: intstr.FromInt(int(clientPort)),
//		}),
//		gomega.Equal(corev1.ServicePort{
//			Name:       "server",
//			Protocol:   corev1.ProtocolTCP,
//			Port:       peerPort,
//			TargetPort: intstr.FromInt(int(peerPort)),
//		}),
//		gomega.Equal(corev1.ServicePort{
//			Name:       "backuprestore",
//			Protocol:   corev1.ProtocolTCP,
//			Port:       backupPort,
//			TargetPort: intstr.FromInt(int(backupPort)),
//		}),
//	))
//}
//
//func existingClientService(etcd *druidv1alpha1.Etcd) *corev1.Service {
//	var annotations, labels map[string]string
//	if etcd.Spec.Etcd.ClientService != nil {
//		annotations = etcd.Spec.Etcd.ClientService.Annotations
//		labels = utils.MergeMaps[string](etcd.Spec.Etcd.ClientService.Labels, etcd.GetDefaultLabels())
//
//	}
//	return &corev1.Service{
//		ObjectMeta: metav1.ObjectMeta{
//			Name:        etcd.GetClientServiceName(),
//			Namespace:   etcd.Namespace,
//			Annotations: annotations,
//			Labels:      labels,
//			OwnerReferences: []metav1.OwnerReference{
//				etcd.GetAsOwnerReference(),
//			},
//		},
//		Spec: corev1.ServiceSpec{
//			Type:                     corev1.ServiceTypeClusterIP,
//			ClusterIP:                corev1.ClusterIPNone,
//			SessionAffinity:          corev1.ServiceAffinityNone,
//			Selector:                 etcd.GetDefaultLabels(),
//			PublishNotReadyAddresses: true,
//			Ports: []corev1.ServicePort{
//				{
//					Name:       "client",
//					Protocol:   corev1.ProtocolTCP,
//					Port:       *etcd.Spec.Etcd.ClientPort,
//					TargetPort: intstr.FromInt(int(*etcd.Spec.Etcd.ClientPort)),
//				},
//				{
//					Name:       "server",
//					Protocol:   corev1.ProtocolTCP,
//					Port:       *etcd.Spec.Etcd.ServerPort,
//					TargetPort: intstr.FromInt(int(*etcd.Spec.Etcd.ServerPort)),
//				},
//				{
//					Name:       "backuprestore",
//					Protocol:   corev1.ProtocolTCP,
//					Port:       *etcd.Spec.Backup.Port,
//					TargetPort: intstr.FromInt(int(*etcd.Spec.Backup.Port)),
//				},
//			},
//		},
//	}
//}
//
//func setupWithFakeClient(t *testing.T, withExistingService bool) (g *gomega.WithT, ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd, cl client.Client, op resource.Operator) {
//	g = gomega.NewWithT(t)
//	ctx = resource.NewOperatorContext(context.TODO(), logr.Logger{}, "xxx-yyyy-zzzz")
//	etcd = sampleEtcd()
//	if withExistingService {
//		cl = fakeclient.NewClientBuilder().WithObjects(existingClientService(etcd)).Build()
//	} else {
//
//		cl = fakeclient.NewClientBuilder().Build()
//	}
//	op = New(cl)
//	return
//}
//
//func setupWithMockClient(t *testing.T) (g *gomega.WithT, ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd, cl *mockclient.MockClient, op resource.Operator) {
//	g = gomega.NewWithT(t)
//	ctx = resource.NewOperatorContext(context.TODO(), logr.Logger{}, "xxx-yyyy-zzzz")
//	etcd = sampleEtcd()
//	cl = mockclient.NewMockClient(gomock.NewController(t))
//	op = New(cl)
//	return
//}
