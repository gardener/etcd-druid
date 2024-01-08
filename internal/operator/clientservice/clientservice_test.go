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

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	druiderr "github.com/gardener/etcd-druid/internal/errors"
	"k8s.io/utils/pointer"

	"github.com/gardener/etcd-druid/internal/operator/resource"
	"github.com/gardener/etcd-druid/internal/utils"
	"github.com/gardener/etcd-druid/test/sample"
	testsample "github.com/gardener/etcd-druid/test/sample"
	testutils "github.com/gardener/etcd-druid/test/utils"
	"github.com/go-logr/logr"
	"github.com/google/uuid"

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

var (
	internalErr    = errors.New("fake get internal error")
	apiInternalErr = apierrors.NewInternalError(internalErr)
)

// ------------------------ GetExistingResourceNames ------------------------
func TestGetExistingResourceNames(t *testing.T) {
	etcd := testsample.EtcdBuilderWithDefaults(testEtcdName, testNs).Build()
	testCases := []struct {
		name                 string
		svcExists            bool
		getErr               *apierrors.StatusError
		expectedErr          *druiderr.DruidError
		expectedServiceNames []string
	}{
		{
			name:                 "should return the existing service name",
			svcExists:            true,
			expectedServiceNames: []string{etcd.GetClientServiceName()},
		},
		{
			name:                 "should return empty slice when service is not found",
			svcExists:            false,
			getErr:               apierrors.NewNotFound(corev1.Resource("services"), etcd.GetClientServiceName()),
			expectedServiceNames: []string{},
		},
		{
			name:      "should return error when get fails",
			svcExists: true,
			getErr:    apiInternalErr,
			expectedErr: &druiderr.DruidError{
				Code:      ErrGetClientService,
				Cause:     apiInternalErr,
				Operation: "GetExistingResourceNames",
			},
		},
	}

	g := NewWithT(t)
	t.Parallel()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fakeClientBuilder := testutils.NewFakeClientBuilder().WithGetError(tc.getErr)
			if tc.svcExists {
				fakeClientBuilder.WithObjects(sample.NewClientService(etcd))
			}
			operator := New(fakeClientBuilder.Build())
			opCtx := resource.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
			svcNames, err := operator.GetExistingResourceNames(opCtx, etcd)
			if tc.expectedErr != nil {
				testutils.CheckDruidError(g, tc.expectedErr, err)
			} else {
				g.Expect(err).To(BeNil())
			}
			g.Expect(svcNames, tc.expectedServiceNames)
		})
	}
}

// ----------------------------------- Sync -----------------------------------
func TestSyncWhenNoServiceExists(t *testing.T) {
	testCases := []struct {
		name          string
		clientPort    *int32
		backupPort    *int32
		peerPort      *int32
		createErr     *apierrors.StatusError
		expectedError *druiderr.DruidError
	}{
		{
			name: "create client service with default ports",
		},
		{
			name:       "create client service with custom ports",
			clientPort: pointer.Int32(2222),
			backupPort: pointer.Int32(3333),
			peerPort:   pointer.Int32(4444),
		},
		{
			name:      "create fails when there is a create error",
			createErr: apiInternalErr,
			expectedError: &druiderr.DruidError{
				Code:      ErrSyncClientService,
				Cause:     apiInternalErr,
				Operation: "Sync",
			},
		},
	}
	g := NewWithT(t)
	t.Parallel()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cl := testutils.NewFakeClientBuilder().WithCreateError(tc.createErr).Build()
			etcd := buildEtcd(tc.clientPort, tc.peerPort, tc.backupPort)
			operator := New(cl)
			opCtx := resource.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
			err := operator.Sync(opCtx, etcd)
			if tc.expectedError != nil {
				testutils.CheckDruidError(g, tc.expectedError, err)
			} else {
				g.Expect(err).NotTo(HaveOccurred())
				checkClientService(g, cl, etcd)
			}
		})
	}
}

func TestSyncWhenServiceExists(t *testing.T) {
	testCases := []struct {
		name          string
		clientPort    *int32
		backupPort    *int32
		peerPort      *int32
		patchErr      *apierrors.StatusError
		expectedError *druiderr.DruidError
	}{
		{
			name:       "update peer service with new server port",
			clientPort: pointer.Int32(2222),
			peerPort:   pointer.Int32(3333),
		},
		{
			name:       "update fails when there is a patch error",
			clientPort: pointer.Int32(2222),
			patchErr:   apiInternalErr,
			expectedError: &druiderr.DruidError{
				Code:      ErrSyncClientService,
				Cause:     apiInternalErr,
				Operation: "Sync",
			},
		},
	}
	g := NewWithT(t)
	t.Parallel()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			existingEtcd := buildEtcd(nil, nil, nil)
			cl := testutils.NewFakeClientBuilder().WithPatchError(tc.patchErr).
				WithObjects(sample.NewClientService(existingEtcd)).
				Build()
			operator := New(cl)
			opCtx := resource.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
			updatedEtcd := buildEtcd(tc.clientPort, tc.peerPort, tc.backupPort)
			err := operator.Sync(opCtx, updatedEtcd)
			if tc.expectedError != nil {
				testutils.CheckDruidError(g, tc.expectedError, err)
			} else {
				g.Expect(err).NotTo(HaveOccurred())
				checkClientService(g, cl, updatedEtcd)
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
				Cause:     apiInternalErr,
				Operation: "TriggerDelete",
			},
			deleteErr: apiInternalErr,
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
				testutils.CheckDruidError(g, tc.expectError, err)
			} else {
				g.Expect(err).NotTo(HaveOccurred())
				serviceList := &corev1.List{}
				err = cl.List(opCtx, serviceList)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(serviceList.Items).To(BeEmpty())

			}
		})
	}
}

// ---------------------------- Helper Functions -----------------------------
func buildEtcd(clientPort, peerPort, backupPort *int32) *druidv1alpha1.Etcd {
	etcdBuilder := testsample.EtcdBuilderWithDefaults(testEtcdName, testNs)
	if clientPort != nil {
		etcdBuilder.WithEtcdClientPort(clientPort)
	}
	if peerPort != nil {
		etcdBuilder.WithEtcdServerPort(peerPort)
	}
	if backupPort != nil {
		etcdBuilder.WithBackupPort(backupPort)
	}
	return etcdBuilder.Build()
}

func checkClientService(g *WithT, cl client.Client, etcd *druidv1alpha1.Etcd) {
	svc := getLatestClientService(g, cl, etcd)
	clientPort := utils.TypeDeref[int32](etcd.Spec.Etcd.ClientPort, defaultClientPort)
	backupPort := utils.TypeDeref[int32](etcd.Spec.Backup.Port, defaultBackupPort)
	peerPort := utils.TypeDeref[int32](etcd.Spec.Etcd.ServerPort, defaultServerPort)

	expectedLabels := etcd.GetDefaultLabels()
	var expectedAnnotations map[string]string
	if etcd.Spec.Etcd.ClientService != nil {
		expectedAnnotations = etcd.Spec.Etcd.ClientService.Annotations
		expectedLabels = utils.MergeMaps[string](etcd.Spec.Etcd.ClientService.Labels, etcd.GetDefaultLabels())
	}
	g.Expect(svc.OwnerReferences).To(Equal([]metav1.OwnerReference{etcd.GetAsOwnerReference()}))
	g.Expect(svc.Annotations).To(Equal(expectedAnnotations))
	g.Expect(svc.Labels).To(Equal(expectedLabels))
	g.Expect(svc.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))
	g.Expect(svc.Spec.SessionAffinity).To(Equal(corev1.ServiceAffinityNone))
	g.Expect(svc.Spec.Selector).To(Equal(etcd.GetDefaultLabels()))
	g.Expect(svc.Spec.Ports).To(ConsistOf(
		Equal(corev1.ServicePort{
			Name:       "client",
			Protocol:   corev1.ProtocolTCP,
			Port:       clientPort,
			TargetPort: intstr.FromInt(int(clientPort)),
		}),
		Equal(corev1.ServicePort{
			Name:       "server",
			Protocol:   corev1.ProtocolTCP,
			Port:       peerPort,
			TargetPort: intstr.FromInt(int(peerPort)),
		}),
		Equal(corev1.ServicePort{
			Name:       "backuprestore",
			Protocol:   corev1.ProtocolTCP,
			Port:       backupPort,
			TargetPort: intstr.FromInt(int(backupPort)),
		}),
	))
}

func getLatestClientService(g *WithT, cl client.Client, etcd *druidv1alpha1.Etcd) *corev1.Service {
	svc := &corev1.Service{}
	err := cl.Get(context.Background(), client.ObjectKey{Name: etcd.GetClientServiceName(), Namespace: etcd.Namespace}, svc)
	g.Expect(err).To(BeNil())
	return svc
}
