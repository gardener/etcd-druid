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
	"testing"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	druiderr "github.com/gardener/etcd-druid/internal/errors"
	"github.com/gardener/etcd-druid/internal/operator/resource"
	"github.com/gardener/etcd-druid/internal/utils"
	testutils "github.com/gardener/etcd-druid/test/utils"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
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
	etcd := testutils.EtcdBuilderWithDefaults(testEtcdName, testNs).Build()
	testcases := []struct {
		name                 string
		svcExists            bool
		getErr               *apierrors.StatusError
		expectedErr          *druiderr.DruidError
		expectedServiceNames []string
	}{
		{
			name:                 "should return the existing service name",
			svcExists:            true,
			expectedServiceNames: []string{etcd.GetPeerServiceName()},
		},
		{
			name:                 "should return empty slice when service is not found",
			svcExists:            false,
			getErr:               apierrors.NewNotFound(corev1.Resource("services"), ""),
			expectedServiceNames: []string{},
		},
		{
			name:      "should return error when get fails",
			svcExists: true,
			getErr:    apiInternalErr,
			expectedErr: &druiderr.DruidError{
				Code:      ErrGetPeerService,
				Cause:     apiInternalErr,
				Operation: "GetExistingResourceNames",
			},
		},
	}

	g := NewWithT(t)
	t.Parallel()

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			fakeClientBuilder := testutils.NewFakeClientBuilder().WithGetError(tc.getErr)
			if tc.svcExists {
				fakeClientBuilder.WithObjects(newPeerService(etcd))
			}
			operator := New(fakeClientBuilder.Build())
			opCtx := resource.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
			svcNames, err := operator.GetExistingResourceNames(opCtx, etcd)
			if tc.expectedErr != nil {
				testutils.CheckDruidError(g, tc.expectedErr, err)
			} else {
				g.Expect(err).To(BeNil())
				g.Expect(svcNames).To(Equal(tc.expectedServiceNames))
			}
		})
	}
}

// ----------------------------------- Sync -----------------------------------
func TestSyncWhenNoServiceExists(t *testing.T) {
	etcdBuilder := testutils.EtcdBuilderWithDefaults(testEtcdName, testNs)
	testCases := []struct {
		name           string
		createWithPort *int32
		createErr      *apierrors.StatusError
		expectedError  *druiderr.DruidError
	}{
		{
			name: "create peer service with default ports when none exists",
		},
		{
			name:           "create service when none exists with custom ports",
			createWithPort: pointer.Int32(2222),
		},
		{
			name:      "returns error when client create fails",
			createErr: apiInternalErr,
			expectedError: &druiderr.DruidError{
				Code:      ErrSyncPeerService,
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
			if tc.createWithPort != nil {
				etcdBuilder.WithEtcdServerPort(tc.createWithPort)
			}
			etcd := etcdBuilder.Build()
			operator := New(cl)
			opCtx := resource.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
			syncErr := operator.Sync(opCtx, etcd)
			latestPeerService, getErr := getLatestPeerService(cl, etcd)
			if tc.expectedError != nil {
				testutils.CheckDruidError(g, tc.expectedError, syncErr)
				g.Expect(apierrors.IsNotFound(getErr)).To(BeTrue())
			} else {
				g.Expect(syncErr).NotTo(HaveOccurred())
				g.Expect(getErr).ToNot(HaveOccurred())
				g.Expect(latestPeerService).ToNot(BeNil())
				matchPeerService(g, etcd, *latestPeerService)
			}
		})
	}
}

func TestSyncWhenServiceExists(t *testing.T) {
	etcdBuilder := testutils.EtcdBuilderWithDefaults(testEtcdName, testNs)
	testCases := []struct {
		name           string
		updateWithPort *int32
		patchErr       *apierrors.StatusError
		expectedError  *druiderr.DruidError
	}{
		{
			name:           "update peer service with new server port",
			updateWithPort: pointer.Int32(2222),
		},
		{
			name:           "update fails when there is a patch error",
			updateWithPort: pointer.Int32(2222),
			patchErr:       apiInternalErr,
			expectedError: &druiderr.DruidError{
				Code:      ErrSyncPeerService,
				Cause:     apiInternalErr,
				Operation: "Sync",
			},
		},
	}
	g := NewWithT(t)
	t.Parallel()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			existingEtcd := etcdBuilder.Build()
			cl := testutils.NewFakeClientBuilder().WithPatchError(tc.patchErr).
				WithObjects(newPeerService(existingEtcd)).
				Build()
			operator := New(cl)
			opCtx := resource.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
			updatedEtcd := etcdBuilder.WithEtcdServerPort(tc.updateWithPort).Build()
			syncErr := operator.Sync(opCtx, updatedEtcd)
			latestPeerService, getErr := getLatestPeerService(cl, updatedEtcd)
			if tc.expectedError != nil {
				testutils.CheckDruidError(g, tc.expectedError, syncErr)
				g.Expect(getErr).ToNot(HaveOccurred())
			} else {
				g.Expect(syncErr).NotTo(HaveOccurred())
				g.Expect(latestPeerService).ToNot(BeNil())
				matchPeerService(g, updatedEtcd, *latestPeerService)
			}
		})
	}
}

// ----------------------------- TriggerDelete -------------------------------
func TestPeerServiceTriggerDelete(t *testing.T) {
	etcd := testutils.EtcdBuilderWithDefaults(testEtcdName, testNs).Build()
	deleteInternalErr := apierrors.NewInternalError(errors.New("fake delete internal error"))
	testCases := []struct {
		name        string
		svcExists   bool
		deleteErr   *apierrors.StatusError
		expectError *druiderr.DruidError
	}{
		{
			name:      "no-op and no error if peer service not found",
			svcExists: false,
		},
		{
			name:      "successfully deletes an existing peer service",
			svcExists: true,
		},
		{
			name:      "returns error when client delete fails",
			svcExists: true,
			deleteErr: deleteInternalErr,
			expectError: &druiderr.DruidError{
				Code:      ErrDeletePeerService,
				Cause:     deleteInternalErr,
				Operation: "TriggerDelete",
			},
		},
	}
	g := NewWithT(t)
	t.Parallel()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fakeClientBuilder := testutils.NewFakeClientBuilder().WithDeleteError(tc.deleteErr)
			if tc.svcExists {
				fakeClientBuilder.WithObjects(newPeerService(etcd))
			}
			cl := fakeClientBuilder.Build()
			operator := New(cl)
			opCtx := resource.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
			syncErr := operator.TriggerDelete(opCtx, etcd)
			_, getErr := getLatestPeerService(cl, etcd)
			if tc.expectError != nil {
				testutils.CheckDruidError(g, tc.expectError, syncErr)
				g.Expect(getErr).ToNot(HaveOccurred())
			} else {
				g.Expect(syncErr).NotTo(HaveOccurred())
				g.Expect(apierrors.IsNotFound(getErr)).To(BeTrue())
			}
		})
	}
}

// ---------------------------- Helper Functions -----------------------------

func newPeerService(etcd *druidv1alpha1.Etcd) *corev1.Service {
	svc := emptyPeerService(getObjectKey(etcd))
	buildResource(etcd, svc)
	return svc
}

func matchPeerService(g *WithT, etcd *druidv1alpha1.Etcd, actualSvc corev1.Service) {
	peerPort := utils.TypeDeref[int32](etcd.Spec.Etcd.ServerPort, defaultServerPort)
	g.Expect(actualSvc).To(MatchFields(IgnoreExtras, Fields{
		"ObjectMeta": MatchFields(IgnoreExtras, Fields{
			"Name":            Equal(etcd.GetPeerServiceName()),
			"Namespace":       Equal(etcd.Namespace),
			"Labels":          testutils.MatchResourceLabels(etcd.GetDefaultLabels()),
			"OwnerReferences": testutils.MatchEtcdOwnerReference(etcd.Name, etcd.UID),
		}),
		"Spec": MatchFields(IgnoreExtras, Fields{
			"Type":            Equal(corev1.ServiceTypeClusterIP),
			"ClusterIP":       Equal(corev1.ClusterIPNone),
			"SessionAffinity": Equal(corev1.ServiceAffinityNone),
			"Selector":        Equal(etcd.GetDefaultLabels()),
			"Ports": ConsistOf(
				Equal(corev1.ServicePort{
					Name:       "peer",
					Protocol:   corev1.ProtocolTCP,
					Port:       peerPort,
					TargetPort: intstr.FromInt(int(peerPort)),
				}),
			),
		}),
	}))
}

func getLatestPeerService(cl client.Client, etcd *druidv1alpha1.Etcd) (*corev1.Service, error) {
	svc := &corev1.Service{}
	err := cl.Get(context.Background(), client.ObjectKey{Name: etcd.GetPeerServiceName(), Namespace: etcd.Namespace}, svc)
	return svc, err
}
