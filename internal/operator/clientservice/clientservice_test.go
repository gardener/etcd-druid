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
	"fmt"
	"testing"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	druiderr "github.com/gardener/etcd-druid/internal/errors"

	"github.com/gardener/etcd-druid/internal/operator/resource"
	"github.com/gardener/etcd-druid/internal/utils"
	"github.com/gardener/etcd-druid/test/sample"
	testsample "github.com/gardener/etcd-druid/test/sample"
	testutils "github.com/gardener/etcd-druid/test/utils"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/onsi/gomega"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	testEtcdName = "test-etcd"
	testNs       = "test-namespace"
)

// ------------------------ GetExistingResourceNames ------------------------
func TestGetExistingResourceNames(t *testing.T) {
	internalErr := errors.New("test internal error")
	etcd := testsample.EtcdBuilderWithDefaults(testEtcdName, testNs).Build()
	testCases := []struct {
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
			apierrors.NewNotFound(corev1.Resource("services"), etcd.GetClientServiceName()),
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
	t.Parallel()

	for _, tc := range testCases {
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

// ----------------------------------- Sync -----------------------------------
func TestClientServiceSync(t *testing.T) {
	etcdBuilder := testsample.EtcdBuilderWithDefaults(testEtcdName, testNs)
	testCases := []struct {
		name        string
		svcExists   bool
		setupFn     func(eb *testsample.EtcdBuilder)
		expectError *druiderr.DruidError
		getErr      *apierrors.StatusError
	}{
		{
			name:      "Create new service",
			svcExists: false,
		},
		{
			name:      "Update existing service",
			svcExists: true,

			setupFn: func(eb *testsample.EtcdBuilder) {
				eb.WithEtcdClientPort(nil).
					WithBackupPort(nil).
					WithEtcdServerPort(nil).
					WithEtcdClientServiceLabels(map[string]string{"testingKey": "testingValue"}).
					WithEtcdClientServiceAnnotations(map[string]string{"testingAnnotationKey": "testingValue"})
			},
		},
		{
			name:      "With client error",
			svcExists: false,
			setupFn:   nil,
			expectError: &druiderr.DruidError{
				Code:      ErrSyncClientService,
				Cause:     fmt.Errorf("fake get error"),
				Operation: "Sync",
				Message:   "Error during create or update of client service",
			},
			getErr: apierrors.NewInternalError(errors.New("fake get error")),
		},
	}

	g := NewWithT(t)
	t.Parallel()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.setupFn != nil {
				tc.setupFn(etcdBuilder)
			}
			etcd := etcdBuilder.Build()
			fakeClientBuilder := testutils.NewFakeClientBuilder()
			if tc.getErr != nil {
				fakeClientBuilder.WithGetError(tc.getErr)
			}
			if tc.svcExists {
				fakeClientBuilder.WithObjects(sample.NewClientService(testsample.EtcdBuilderWithDefaults(testEtcdName, testNs).Build()))
			}
			cl := fakeClientBuilder.Build()
			operator := New(cl)
			opCtx := resource.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
			err := operator.Sync(opCtx, etcd)
			if tc.expectError != nil {
				g.Expect(err).To(gomega.HaveOccurred())
				var druidErr *druiderr.DruidError
				g.Expect(errors.As(err, &druidErr)).To(BeTrue())
				g.Expect(druidErr.Code).To(Equal(tc.expectError.Code))
				g.Expect(errors.Is(druidErr.Cause, tc.getErr)).To(BeTrue())
				g.Expect(druidErr.Message).To(Equal(tc.expectError.Message))
				g.Expect(druidErr.Operation).To(Equal(tc.expectError.Operation))

			} else {
				g.Expect(err).NotTo(gomega.HaveOccurred())
				service := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      etcd.GetClientServiceName(),
						Namespace: etcd.Namespace,
					},
				}
				err = cl.Get(opCtx, client.ObjectKeyFromObject(service), service)
				g.Expect(err).NotTo(gomega.HaveOccurred())
				checkClientService(g, service, etcd)
			}
		})
	}
}

// ----------------------------- TriggerDelete -------------------------------
func TestClientServiceTriggerDelete(t *testing.T) {
	etcdBuilder := testsample.EtcdBuilderWithDefaults(testEtcdName, testNs)
	testCases := []struct {
		name        string
		svcExists   bool
		setupFn     func(eb *testsample.EtcdBuilder)
		expectError *druiderr.DruidError
		deleteErr   *apierrors.StatusError
	}{
		{
			name:      "Existing Service - Delete Operation",
			svcExists: false,
		},
		{
			name:      "Service Not Found - No Operation",
			svcExists: true,
			setupFn: func(eb *testsample.EtcdBuilder) {
				eb.WithEtcdClientPort(nil).
					WithBackupPort(nil).
					WithEtcdServerPort(nil).
					WithEtcdClientServiceLabels(map[string]string{"testingKey": "testingValue"}).
					WithEtcdClientServiceAnnotations(map[string]string{"testingAnnotationKey": "testingValue"})
			},
		},
		{
			name:      "Client Error on Delete - Returns Error",
			svcExists: true,
			setupFn:   nil,
			expectError: &druiderr.DruidError{
				Code:      ErrDeleteClientService,
				Cause:     errors.New("fake delete error"),
				Operation: "TriggerDelete",
				Message:   "Failed to delete client service",
			},
			deleteErr: apierrors.NewInternalError(errors.New("fake delete error")),
		},
	}
	g := NewWithT(t)
	t.Parallel()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.setupFn != nil {
				tc.setupFn(etcdBuilder)
			}
			etcd := etcdBuilder.Build()
			fakeClientBuilder := testutils.NewFakeClientBuilder()
			if tc.deleteErr != nil {
				fakeClientBuilder.WithDeleteError(tc.deleteErr)
			}
			if tc.svcExists {
				fakeClientBuilder.WithObjects(sample.NewClientService(testsample.EtcdBuilderWithDefaults(testEtcdName, testNs).Build()))
			}
			cl := fakeClientBuilder.Build()
			operator := New(cl)
			opCtx := resource.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
			err := operator.TriggerDelete(opCtx, etcd)
			if tc.expectError != nil {
				g.Expect(err).To(gomega.HaveOccurred())
				var druidErr *druiderr.DruidError
				g.Expect(errors.As(err, &druidErr)).To(BeTrue())
				g.Expect(druidErr.Code).To(Equal(tc.expectError.Code))
				g.Expect(errors.Is(druidErr.Cause, tc.deleteErr)).To(BeTrue())
				g.Expect(druidErr.Message).To(Equal(tc.expectError.Message))
				g.Expect(druidErr.Operation).To(Equal(tc.expectError.Operation))

			} else {
				g.Expect(err).NotTo(gomega.HaveOccurred())
				serviceList := &corev1.List{}
				err = cl.List(opCtx, serviceList)
				g.Expect(err).NotTo(gomega.HaveOccurred())
				g.Expect(serviceList.Items).To(gomega.BeEmpty())

			}
		})
	}
}

func checkClientService(g *gomega.WithT, svc *corev1.Service, etcd *druidv1alpha1.Etcd) {
	clientPort := utils.TypeDeref[int32](etcd.Spec.Etcd.ClientPort, defaultClientPort)
	backupPort := utils.TypeDeref[int32](etcd.Spec.Backup.Port, defaultBackupPort)
	peerPort := utils.TypeDeref[int32](etcd.Spec.Etcd.ServerPort, defaultServerPort)

	expectedLabels := etcd.GetDefaultLabels()
	var expectedAnnotations map[string]string
	if etcd.Spec.Etcd.ClientService != nil {
		expectedAnnotations = etcd.Spec.Etcd.ClientService.Annotations
		expectedLabels = utils.MergeMaps[string](etcd.Spec.Etcd.ClientService.Labels, etcd.GetDefaultLabels())
	}
	g.Expect(svc.OwnerReferences).To(gomega.Equal([]metav1.OwnerReference{etcd.GetAsOwnerReference()}))
	g.Expect(svc.Annotations).To(gomega.Equal(expectedAnnotations))
	g.Expect(svc.Labels).To(gomega.Equal(expectedLabels))
	g.Expect(svc.Spec.Type).To(gomega.Equal(corev1.ServiceTypeClusterIP))
	g.Expect(svc.Spec.SessionAffinity).To(gomega.Equal(corev1.ServiceAffinityNone))
	g.Expect(svc.Spec.Selector).To(gomega.Equal(etcd.GetDefaultLabels()))
	g.Expect(svc.Spec.Ports).To(gomega.ConsistOf(
		gomega.Equal(corev1.ServicePort{
			Name:       "client",
			Protocol:   corev1.ProtocolTCP,
			Port:       clientPort,
			TargetPort: intstr.FromInt(int(clientPort)),
		}),
		gomega.Equal(corev1.ServicePort{
			Name:       "server",
			Protocol:   corev1.ProtocolTCP,
			Port:       peerPort,
			TargetPort: intstr.FromInt(int(peerPort)),
		}),
		gomega.Equal(corev1.ServicePort{
			Name:       "backuprestore",
			Protocol:   corev1.ProtocolTCP,
			Port:       backupPort,
			TargetPort: intstr.FromInt(int(backupPort)),
		}),
	))
}
