// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package clientservice

import (
	"context"
	"testing"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"
	"github.com/gardener/etcd-druid/internal/component"
	druiderr "github.com/gardener/etcd-druid/internal/errors"
	"github.com/gardener/etcd-druid/internal/utils"
	testutils "github.com/gardener/etcd-druid/test/utils"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
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
			getErr:               nil,
			expectedServiceNames: []string{druidv1alpha1.GetClientServiceName(etcd.ObjectMeta)},
		},
		{
			name:                 "should return empty slice when service is not found",
			svcExists:            false,
			getErr:               apierrors.NewNotFound(corev1.Resource("services"), druidv1alpha1.GetClientServiceName(etcd.ObjectMeta)),
			expectedServiceNames: []string{},
		},
		{
			name:      "should return error when get fails",
			svcExists: true,
			getErr:    testutils.TestAPIInternalErr,
			expectedErr: &druiderr.DruidError{
				Code:      ErrGetClientService,
				Cause:     testutils.TestAPIInternalErr,
				Operation: component.OperationGetExistingResourceNames,
			},
		},
	}

	g := NewWithT(t)
	t.Parallel()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cl := testutils.CreateTestFakeClientForObjects(tc.getErr, nil, nil, nil, []client.Object{newClientService(etcd)}, client.ObjectKey{Name: druidv1alpha1.GetClientServiceName(etcd.ObjectMeta), Namespace: etcd.Namespace})
			operator := New(cl)
			opCtx := component.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
			svcNames, err := operator.GetExistingResourceNames(opCtx, etcd.ObjectMeta)
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
		name                string
		clientPort          *int32
		backupPort          *int32
		peerPort            *int32
		trafficDistribution *string
		createErr           *apierrors.StatusError
		expectedErr         *druiderr.DruidError
	}{
		{
			name: "create client service with default ports",
		},
		{
			name:       "create client service with custom ports",
			clientPort: ptr.To[int32](2222),
			backupPort: ptr.To[int32](3333),
			peerPort:   ptr.To[int32](4444),
		},
		{
			name:                "create client service with traffic distribution",
			trafficDistribution: ptr.To("PreferClose"),
		},
		{
			name:      "create fails when there is a create error",
			createErr: testutils.TestAPIInternalErr,
			expectedErr: &druiderr.DruidError{
				Code:      ErrSyncClientService,
				Cause:     testutils.TestAPIInternalErr,
				Operation: component.OperationSync,
			},
		},
	}
	g := NewWithT(t)
	t.Parallel()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			etcd := buildEtcd(tc.clientPort, tc.peerPort, tc.backupPort, tc.trafficDistribution)
			cl := testutils.CreateTestFakeClientForObjects(nil, tc.createErr, nil, nil, nil, client.ObjectKey{Name: druidv1alpha1.GetClientServiceName(etcd.ObjectMeta), Namespace: etcd.Namespace})
			operator := New(cl)
			opCtx := component.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
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
		originalClientPort          int32  = 2379
		originalServerPort          int32  = 2380
		originalBackupPort          int32  = 8080
		originalTrafficDistribution string = "PreferClose"
	)
	existingEtcd := buildEtcd(ptr.To(originalClientPort), ptr.To(originalServerPort), ptr.To(originalBackupPort), ptr.To(originalTrafficDistribution))
	testCases := []struct {
		name                string
		clientPort          *int32
		backupPort          *int32
		peerPort            *int32
		trafficDistribution *string
		patchErr            *apierrors.StatusError
		expectedError       *druiderr.DruidError
	}{
		{
			name:       "update peer service with new server port",
			clientPort: ptr.To[int32](2222),
			peerPort:   ptr.To[int32](3333),
		},
		{
			name:                "update client service with new traffic distribution",
			trafficDistribution: ptr.To("Foo"),
		},
		{
			name:       "update fails when there is a patch error",
			clientPort: ptr.To[int32](2222),
			patchErr:   testutils.TestAPIInternalErr,
			expectedError: &druiderr.DruidError{
				Code:      ErrSyncClientService,
				Cause:     testutils.TestAPIInternalErr,
				Operation: component.OperationSync,
			},
		},
	}
	g := NewWithT(t)
	t.Parallel()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// ********************* Setup *********************
			cl := testutils.CreateTestFakeClientForObjects(nil, nil, tc.patchErr, nil, []client.Object{newClientService(existingEtcd)}, client.ObjectKey{Name: druidv1alpha1.GetClientServiceName(existingEtcd.ObjectMeta), Namespace: existingEtcd.Namespace})
			// ********************* test sync with updated ports *********************
			operator := New(cl)
			opCtx := component.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
			updatedEtcd := buildEtcd(tc.clientPort, tc.peerPort, tc.backupPort, tc.trafficDistribution)
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
				Operation: component.OperationTriggerDelete,
			},
			deleteErr: testutils.TestAPIInternalErr,
		},
	}
	g := NewWithT(t)
	t.Parallel()

	etcd := testutils.EtcdBuilderWithDefaults(testutils.TestEtcdName, testutils.TestNamespace).Build()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// ********************* Setup *********************
			var existingObjects []client.Object
			if tc.svcExists {
				existingObjects = append(existingObjects, newClientService(etcd))
			}
			cl := testutils.CreateTestFakeClientForObjects(nil, nil, nil, tc.deleteErr, existingObjects, client.ObjectKey{Name: druidv1alpha1.GetClientServiceName(etcd.ObjectMeta), Namespace: etcd.Namespace})
			operator := New(cl)
			opCtx := component.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
			// ********************* Test trigger delete *********************
			triggerDeleteErr := operator.TriggerDelete(opCtx, etcd.ObjectMeta)
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
func buildEtcd(clientPort, peerPort, backupPort *int32, trafficDistribution *string) *druidv1alpha1.Etcd {
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
	if trafficDistribution != nil {
		etcdBuilder.WithEtcdClientServiceTrafficDistribution(trafficDistribution)
	}
	return etcdBuilder.Build()
}

func matchClientService(g *WithT, etcd *druidv1alpha1.Etcd, actualSvc corev1.Service) {
	clientPort := ptr.Deref(etcd.Spec.Etcd.ClientPort, common.DefaultPortEtcdClient)
	backupPort := ptr.Deref(etcd.Spec.Backup.Port, common.DefaultPortEtcdBackupRestore)
	peerPort := ptr.Deref(etcd.Spec.Etcd.ServerPort, common.DefaultPortEtcdPeer)
	etcdObjMeta := etcd.ObjectMeta
	expectedLabels := druidv1alpha1.GetDefaultLabels(etcdObjMeta)
	var expectedAnnotations map[string]string
	var expectedTrafficDistribution *string
	if etcd.Spec.Etcd.ClientService != nil {
		expectedAnnotations = etcd.Spec.Etcd.ClientService.Annotations
		expectedLabels = utils.MergeMaps(etcd.Spec.Etcd.ClientService.Labels, druidv1alpha1.GetDefaultLabels(etcdObjMeta))
		expectedTrafficDistribution = etcd.Spec.Etcd.ClientService.TrafficDistribution
	}
	expectedLabelSelector := utils.MergeMaps(druidv1alpha1.GetDefaultLabels(etcdObjMeta), map[string]string{druidv1alpha1.LabelComponentKey: common.ComponentNameStatefulSet})

	g.Expect(actualSvc).To(MatchFields(IgnoreExtras, Fields{
		"ObjectMeta": MatchFields(IgnoreExtras, Fields{
			"Name":            Equal(druidv1alpha1.GetClientServiceName(etcd.ObjectMeta)),
			"Namespace":       Equal(etcd.Namespace),
			"Annotations":     testutils.MatchResourceAnnotations(expectedAnnotations),
			"Labels":          testutils.MatchResourceLabels(expectedLabels),
			"OwnerReferences": testutils.MatchEtcdOwnerReference(etcd.Name, etcd.UID),
		}),
		"Spec": MatchFields(IgnoreExtras, Fields{
			"Type":            Equal(corev1.ServiceTypeClusterIP),
			"SessionAffinity": Equal(corev1.ServiceAffinityNone),
			"Selector":        Equal(expectedLabelSelector),
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
			"TrafficDistribution": Equal(expectedTrafficDistribution),
		}),
	}))
}

func newClientService(etcd *druidv1alpha1.Etcd) *corev1.Service {
	svc := emptyClientService(getObjectKey(etcd.ObjectMeta))
	buildResource(etcd, svc)
	return svc
}

func getLatestClientService(cl client.Client, etcd *druidv1alpha1.Etcd) (*corev1.Service, error) {
	svc := &corev1.Service{}
	err := cl.Get(context.Background(), client.ObjectKey{Name: druidv1alpha1.GetClientServiceName(etcd.ObjectMeta), Namespace: etcd.Namespace}, svc)
	return svc, err
}
