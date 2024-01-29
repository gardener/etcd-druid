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
	"testing"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	druiderr "github.com/gardener/etcd-druid/internal/errors"
	. "github.com/onsi/gomega/gstruct"
	"k8s.io/utils/pointer"

	"github.com/gardener/etcd-druid/internal/operator/resource"
	"github.com/gardener/etcd-druid/internal/utils"
	testutils "github.com/gardener/etcd-druid/test/utils"
	"github.com/go-logr/logr"
	"github.com/google/uuid"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ------------------------ GetExistingResourceNames ------------------------
func TestGetExistingResourceNames(t *testing.T) {
	etcd := testutils.EtcdBuilderWithDefaults(testutils.TestEtcdName, testutils.TestNamespace).Build()
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
			getErr:    testutils.TestAPIInternalErr,
			expectedErr: &druiderr.DruidError{
				Code:      ErrGetClientService,
				Cause:     testutils.TestAPIInternalErr,
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
				fakeClientBuilder.WithObjects(newClientService(etcd))
			}
			operator := New(fakeClientBuilder.Build())
			opCtx := resource.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
			svcNames, err := operator.GetExistingResourceNames(opCtx, etcd)
			if tc.expectedErr != nil {
				testutils.CheckDruidError(g, tc.expectedErr, err)
			} else {
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(svcNames).To(Equal(tc.expectedServiceNames))
			}
		})
	}
}

// ----------------------------------- Sync -----------------------------------
func TestSyncWhenNoServiceExists(t *testing.T) {
	testCases := []struct {
		name        string
		clientPort  *int32
		backupPort  *int32
		peerPort    *int32
		createErr   *apierrors.StatusError
		expectedErr *druiderr.DruidError
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
			createErr: testutils.TestAPIInternalErr,
			expectedErr: &druiderr.DruidError{
				Code:      ErrSyncClientService,
				Cause:     testutils.TestAPIInternalErr,
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
			latestClientSvc, getErr := getLatestClientService(cl, etcd)
			if tc.expectedErr != nil {
				testutils.CheckDruidError(g, tc.expectedErr, err)
				g.Expect(apierrors.IsNotFound(getErr)).To(BeTrue())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
				matchClientService(g, etcd, *latestClientSvc)
			}
		})
	}
}

func TestSyncWhenServiceExists(t *testing.T) {
	const (
		originalClientPort = 2379
		originalServerPort = 2380
		originalBackupPort = 8080
	)
	existingEtcd := buildEtcd(pointer.Int32(originalClientPort), pointer.Int32(originalServerPort), pointer.Int32(originalBackupPort))
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
			patchErr:   testutils.TestAPIInternalErr,
			expectedError: &druiderr.DruidError{
				Code:      ErrSyncClientService,
				Cause:     testutils.TestAPIInternalErr,
				Operation: "Sync",
			},
		},
	}
	g := NewWithT(t)
	t.Parallel()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// ********************* Setup *********************
			cl := testutils.NewFakeClientBuilder().
				WithPatchError(tc.patchErr).
				WithObjects(newClientService(existingEtcd)).
				Build()
			// ********************* test sync with updated ports *********************
			operator := New(cl)
			opCtx := resource.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
			updatedEtcd := buildEtcd(tc.clientPort, tc.peerPort, tc.backupPort)
			syncErr := operator.Sync(opCtx, updatedEtcd)
			latestClientSvc, getErr := getLatestClientService(cl, updatedEtcd)
			g.Expect(latestClientSvc).ToNot(BeNil())
			if tc.expectedError != nil {
				testutils.CheckDruidError(g, tc.expectedError, syncErr)
				g.Expect(getErr).ToNot(HaveOccurred())
				matchClientService(g, existingEtcd, *latestClientSvc)
			} else {
				g.Expect(syncErr).NotTo(HaveOccurred())
				matchClientService(g, updatedEtcd, *latestClientSvc)
			}
		})
	}
}

// ----------------------------- TriggerDelete -------------------------------
func TestTriggerDelete(t *testing.T) {
	testCases := []struct {
		name        string
		svcExists   bool
		deleteErr   *apierrors.StatusError
		expectedErr *druiderr.DruidError
	}{
		{
			name:      "no-op when client service does not exist",
			svcExists: false,
		},
		{
			name:      "successfully delete existing client service",
			svcExists: true,
		},
		{
			name:      "returns error when client delete fails",
			svcExists: true,
			expectedErr: &druiderr.DruidError{
				Code:      ErrDeleteClientService,
				Cause:     testutils.TestAPIInternalErr,
				Operation: "TriggerDelete",
			},
			deleteErr: testutils.TestAPIInternalErr,
		},
	}
	g := NewWithT(t)
	t.Parallel()

	etcd := testutils.EtcdBuilderWithDefaults(testutils.TestEtcdName, testutils.TestNamespace).Build()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// ********************* Setup *********************
			cl := testutils.NewFakeClientBuilder().WithDeleteError(tc.deleteErr).Build()
			operator := New(cl)
			opCtx := resource.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
			if tc.svcExists {
				syncErr := operator.Sync(opCtx, etcd)
				g.Expect(syncErr).ToNot(HaveOccurred())
				ensureClientServiceExists(g, cl, etcd)
			}
			// ********************* Test trigger delete *********************
			triggerDeleteErr := operator.TriggerDelete(opCtx, etcd)
			latestClientService, getErr := getLatestClientService(cl, etcd)
			if tc.expectedErr != nil {
				testutils.CheckDruidError(g, tc.expectedErr, triggerDeleteErr)
				g.Expect(getErr).To(BeNil())
				g.Expect(latestClientService).ToNot(BeNil())
			} else {
				g.Expect(triggerDeleteErr).NotTo(HaveOccurred())
				g.Expect(apierrors.IsNotFound(getErr)).To(BeTrue())
			}
		})
	}
}

// ---------------------------- Helper Functions -----------------------------
func buildEtcd(clientPort, peerPort, backupPort *int32) *druidv1alpha1.Etcd {
	etcdBuilder := testutils.EtcdBuilderWithDefaults(testutils.TestEtcdName, testutils.TestNamespace)
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

func matchClientService(g *WithT, etcd *druidv1alpha1.Etcd, actualSvc corev1.Service) {
	clientPort := utils.TypeDeref[int32](etcd.Spec.Etcd.ClientPort, defaultClientPort)
	backupPort := utils.TypeDeref[int32](etcd.Spec.Backup.Port, defaultBackupPort)
	peerPort := utils.TypeDeref[int32](etcd.Spec.Etcd.ServerPort, defaultServerPort)

	expectedLabels := etcd.GetDefaultLabels()
	var expectedAnnotations map[string]string
	if etcd.Spec.Etcd.ClientService != nil {
		expectedAnnotations = etcd.Spec.Etcd.ClientService.Annotations
		expectedLabels = utils.MergeMaps[string](etcd.Spec.Etcd.ClientService.Labels, etcd.GetDefaultLabels())
	}

	g.Expect(actualSvc).To(MatchFields(IgnoreExtras, Fields{
		"ObjectMeta": MatchFields(IgnoreExtras, Fields{
			"Name":            Equal(etcd.GetClientServiceName()),
			"Namespace":       Equal(etcd.Namespace),
			"Annotations":     testutils.MatchResourceAnnotations(expectedAnnotations),
			"Labels":          testutils.MatchResourceLabels(expectedLabels),
			"OwnerReferences": testutils.MatchEtcdOwnerReference(etcd.Name, etcd.UID),
		}),
		"Spec": MatchFields(IgnoreExtras, Fields{
			"Type":            Equal(corev1.ServiceTypeClusterIP),
			"SessionAffinity": Equal(corev1.ServiceAffinityNone),
			"Selector":        Equal(etcd.GetDefaultLabels()),
			"Ports": ConsistOf(
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
			),
		}),
	}))
}

func newClientService(etcd *druidv1alpha1.Etcd) *corev1.Service {
	svc := emptyClientService(getObjectKey(etcd))
	buildResource(etcd, svc)
	return svc
}

func ensureClientServiceExists(g *WithT, cl client.Client, etcd *druidv1alpha1.Etcd) {
	svc, err := getLatestClientService(cl, etcd)
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(svc).ToNot(BeNil())
}

func getLatestClientService(cl client.Client, etcd *druidv1alpha1.Etcd) (*corev1.Service, error) {
	svc := &corev1.Service{}
	err := cl.Get(context.Background(), client.ObjectKey{Name: etcd.GetClientServiceName(), Namespace: etcd.Namespace}, svc)
	return svc, err
}
