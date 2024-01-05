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
			[]string{etcd.GetPeerServiceName()},
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
	t.Parallel()

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			fakeClientBuilder := testutils.NewFakeClientBuilder()
			if tc.getErr != nil {
				fakeClientBuilder.WithGetError(tc.getErr)
			}
			if tc.svcExists {
				fakeClientBuilder.WithObjects(sample.NewPeerService(etcd))
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
func TestPeerServiceSync(t *testing.T) {
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
				eb.WithBackupPort(nil)
			},
		},
		{
			name:      "With client error",
			svcExists: false,
			setupFn:   nil,
			expectError: &druiderr.DruidError{
				Code:      ErrSyncPeerService,
				Cause:     fmt.Errorf("fake get error"),
				Operation: "Sync",
				Message:   "Error during create or update of peer service",
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
				fakeClientBuilder.WithObjects(sample.NewPeerService(testsample.EtcdBuilderWithDefaults(testEtcdName, testNs).Build()))
			}
			cl := fakeClientBuilder.Build()
			operator := New(cl)
			opCtx := resource.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
			err := operator.Sync(opCtx, etcd)
			if tc.expectError != nil {
				g.Expect(err).To(HaveOccurred())
				var druidErr *druiderr.DruidError
				g.Expect(errors.As(err, &druidErr)).To(BeTrue())
				g.Expect(druidErr.Code).To(Equal(tc.expectError.Code))
				g.Expect(errors.Is(druidErr.Cause, tc.getErr)).To(BeTrue())
				g.Expect(druidErr.Message).To(Equal(tc.expectError.Message))
				g.Expect(druidErr.Operation).To(Equal(tc.expectError.Operation))

			} else {
				g.Expect(err).NotTo(HaveOccurred())
				service := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      etcd.GetPeerServiceName(),
						Namespace: etcd.Namespace,
					},
				}
				err = cl.Get(opCtx, client.ObjectKeyFromObject(service), service)
				g.Expect(err).NotTo(HaveOccurred())
				checkPeerService(g, service, etcd)
			}
		})
	}
}

// ----------------------------- TriggerDelete -------------------------------
func TestPeerServiceTriggerDelete(t *testing.T) {
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
				eb.WithEtcdServerPort(nil)
			},
		},
		{
			name:      "Client Error on Delete - Returns Error",
			svcExists: true,
			setupFn:   nil,
			expectError: &druiderr.DruidError{
				Code:      ErrDeletePeerService,
				Cause:     errors.New("fake delete error"),
				Operation: "TriggerDelete",
				Message:   "Failed to delete peer service",
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
				fakeClientBuilder.WithObjects(sample.NewPeerService(testsample.EtcdBuilderWithDefaults(testEtcdName, testNs).Build()))
			}
			cl := fakeClientBuilder.Build()
			operator := New(cl)
			opCtx := resource.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
			err := operator.TriggerDelete(opCtx, etcd)
			if tc.expectError != nil {
				g.Expect(err).To(HaveOccurred())
				var druidErr *druiderr.DruidError
				g.Expect(errors.As(err, &druidErr)).To(BeTrue())
				g.Expect(druidErr.Code).To(Equal(tc.expectError.Code))
				g.Expect(errors.Is(druidErr.Cause, tc.deleteErr)).To(BeTrue())
				g.Expect(druidErr.Message).To(Equal(tc.expectError.Message))
				g.Expect(druidErr.Operation).To(Equal(tc.expectError.Operation))

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
func checkPeerService(g *WithT, svc *corev1.Service, etcd *druidv1alpha1.Etcd) {
	peerPort := utils.TypeDeref[int32](etcd.Spec.Etcd.ServerPort, defaultServerPort)
	g.Expect(svc.OwnerReferences).To(Equal([]metav1.OwnerReference{etcd.GetAsOwnerReference()}))
	g.Expect(svc.Labels).To(Equal(etcd.GetDefaultLabels()))
	g.Expect(svc.Spec.PublishNotReadyAddresses).To(BeTrue())
	g.Expect(svc.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))
	g.Expect(svc.Spec.ClusterIP).To(Equal(corev1.ClusterIPNone))
	g.Expect(svc.Spec.SessionAffinity).To(Equal(corev1.ServiceAffinityNone))
	g.Expect(svc.Spec.Selector).To(Equal(etcd.GetDefaultLabels()))
	g.Expect(svc.Spec.Ports).To(ConsistOf(
		Equal(corev1.ServicePort{
			Name:       "peer",
			Protocol:   corev1.ProtocolTCP,
			Port:       peerPort,
			TargetPort: intstr.FromInt(int(peerPort)),
		}),
	))
}
