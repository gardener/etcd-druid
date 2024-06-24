// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/gardener/etcd-druid/internal/client/kubernetes"
	testutils "github.com/gardener/etcd-druid/test/utils"

	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/uuid"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	stsNamespace = "test-ns"
	stsName      = "etcd-test"
	eventName    = "test-event"
)

func TestIsStatefulSetReady(t *testing.T) {
	testCases := []struct {
		name                          string
		specGeneration                int64
		statusObservedGeneration      int64
		etcdSpecReplicas              int32
		statusReadyReplicas           int32
		statusCurrentReplicas         int32
		statusUpdatedReplicas         int32
		statusCurrentRevision         string
		statusUpdateRevision          string
		expectedStsReady              bool
		expectedNotReadyReasonPresent bool
	}{
		{
			name:                          "sts has less number of ready replicas as compared to configured etcd replicas",
			specGeneration:                1,
			statusObservedGeneration:      1,
			etcdSpecReplicas:              3,
			statusReadyReplicas:           2,
			expectedStsReady:              false,
			expectedNotReadyReasonPresent: true,
		},
		{
			name:                          "sts has equal number of replicas as defined in etcd but observed generation is outdated",
			specGeneration:                2,
			statusObservedGeneration:      1,
			etcdSpecReplicas:              3,
			statusReadyReplicas:           3,
			expectedStsReady:              false,
			expectedNotReadyReasonPresent: true,
		},
		{
			name:                          "sts has mismatching current and update revision",
			specGeneration:                2,
			statusObservedGeneration:      2,
			etcdSpecReplicas:              3,
			statusReadyReplicas:           3,
			statusCurrentRevision:         "etcd-main-6d5cc8f559",
			statusUpdateRevision:          "etcd-main-bf6b695326",
			expectedStsReady:              false,
			expectedNotReadyReasonPresent: true,
		},
		{
			name:                          "sts has mismatching status ready and updated replicas",
			specGeneration:                2,
			statusObservedGeneration:      2,
			etcdSpecReplicas:              3,
			statusReadyReplicas:           3,
			statusCurrentRevision:         "etcd-main-6d5cc8f559",
			statusUpdateRevision:          "etcd-main-bf6b695326",
			statusCurrentReplicas:         3,
			statusUpdatedReplicas:         2,
			expectedStsReady:              false,
			expectedNotReadyReasonPresent: true,
		},
		{
			name:                          "sts is completely up-to-date",
			specGeneration:                2,
			statusObservedGeneration:      2,
			etcdSpecReplicas:              3,
			statusReadyReplicas:           3,
			statusCurrentRevision:         "etcd-main-6d5cc8f559",
			statusUpdateRevision:          "etcd-main-6d5cc8f559",
			statusCurrentReplicas:         3,
			statusUpdatedReplicas:         3,
			expectedStsReady:              true,
			expectedNotReadyReasonPresent: false,
		},
	}

	g := NewWithT(t)
	t.Parallel()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			sts := testutils.CreateStatefulSet(stsName, stsNamespace, uuid.NewUUID(), 2)
			sts.Generation = tc.specGeneration
			sts.Status.ObservedGeneration = tc.statusObservedGeneration
			sts.Status.ReadyReplicas = tc.statusReadyReplicas
			sts.Status.CurrentReplicas = tc.statusCurrentReplicas
			sts.Status.UpdatedReplicas = tc.statusUpdatedReplicas
			sts.Status.CurrentRevision = tc.statusCurrentRevision
			sts.Status.UpdateRevision = tc.statusUpdateRevision
			stsReady, reasonMsg := IsStatefulSetReady(tc.etcdSpecReplicas, sts)
			g.Expect(stsReady).To(Equal(tc.expectedStsReady))
			g.Expect(!IsEmptyString(reasonMsg)).To(Equal(tc.expectedNotReadyReasonPresent))
		})
	}
}

func TestGetStatefulSet(t *testing.T) {
	internalErr := errors.New("test internal error")
	apiInternalErr := apierrors.NewInternalError(internalErr)

	etcd := testutils.EtcdBuilderWithDefaults(testutils.TestEtcdName, testutils.TestNamespace).WithReplicas(3).Build()
	testCases := []struct {
		name         string
		isStsPresent bool
		ownedByEtcd  bool
		getErr       *apierrors.StatusError
		expectedErr  *apierrors.StatusError
	}{
		{
			name:         "no sts found",
			isStsPresent: false,
			ownedByEtcd:  false,
		},
		{
			name:         "sts found but not owned by etcd",
			isStsPresent: true,
			ownedByEtcd:  false,
		},
		{
			name:         "sts found and owned by etcd",
			isStsPresent: true,
			ownedByEtcd:  true,
		},
		{
			name:         "returns error when client get fails",
			isStsPresent: true,
			ownedByEtcd:  true,
			getErr:       apiInternalErr,
			expectedErr:  apiInternalErr,
		},
	}

	g := NewWithT(t)
	t.Parallel()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var existingObjects []client.Object
			if tc.isStsPresent {
				etcdUID := etcd.UID
				if !tc.ownedByEtcd {
					etcdUID = uuid.NewUUID()
				}
				sts := testutils.CreateStatefulSet(etcd.Name, etcd.Namespace, etcdUID, etcd.Spec.Replicas)
				existingObjects = append(existingObjects, sts)
			}
			cl := testutils.CreateTestFakeClientWithSchemeForObjects(kubernetes.Scheme, tc.getErr, nil, nil, nil, existingObjects, client.ObjectKey{Name: testutils.TestEtcdName, Namespace: testutils.TestNamespace})
			foundSts, err := GetStatefulSet(context.Background(), cl, etcd)
			if tc.expectedErr != nil {
				g.Expect(err).To(HaveOccurred())
				g.Expect(errors.Is(err, tc.expectedErr)).To(BeTrue())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
				expectedStsToBeFound := tc.isStsPresent && tc.ownedByEtcd
				g.Expect(foundSts != nil).To(Equal(expectedStsToBeFound))
			}
		})
	}
}

func TestFetchPVCWarningMessagesForStatefulSet(t *testing.T) {
	internalErr := errors.New("test internal error")
	apiInternalErr := apierrors.NewInternalError(internalErr)

	etcd := testutils.EtcdBuilderWithDefaults(testutils.TestEtcdName, testutils.TestNamespace).WithReplicas(3).Build()
	sts := testutils.CreateStatefulSet(etcd.Name, etcd.Namespace, etcd.UID, etcd.Spec.Replicas)
	pvcPending := testutils.CreatePVC(sts, fmt.Sprintf("%s-0", sts.Name), corev1.ClaimPending)
	pvcBound := testutils.CreatePVC(sts, fmt.Sprintf("%s-1", sts.Name), corev1.ClaimBound)
	eventWarning := testutils.CreateEvent(eventName, sts.Namespace, "FailedMount", "test pvc warning message", corev1.EventTypeWarning, pvcPending, schema.GroupVersionKind{Group: "", Version: "v1", Kind: "PersistentVolumeClaim"})

	testCases := []struct {
		name        string
		sts         *appsv1.StatefulSet
		pvcList     []client.Object
		eventList   []client.Object
		errors      []testutils.ErrorsForGVK
		expectedErr *apierrors.StatusError
		expectedMsg string
	}{
		{
			name: "error in listing PVCs",
			errors: []testutils.ErrorsForGVK{
				{
					GVK:     corev1.SchemeGroupVersion.WithKind("PersistentVolumeClaimList"),
					ListErr: apiInternalErr,
				},
			},
			expectedErr: apiInternalErr,
		},
		{
			name:        "no PVCs found",
			pvcList:     nil,
			expectedErr: nil,
			expectedMsg: "",
		},
		{
			name:    "PVCs found but error in listing events",
			pvcList: []client.Object{pvcPending},
			errors: []testutils.ErrorsForGVK{
				{
					GVK:     corev1.SchemeGroupVersion.WithKind("EventList"),
					ListErr: apiInternalErr,
				},
			},
			expectedErr: apiInternalErr,
			expectedMsg: "",
		},
		{
			name:        "PVCs found with warning events",
			pvcList:     []client.Object{pvcPending},
			eventList:   []client.Object{eventWarning},
			expectedErr: nil,
			expectedMsg: eventWarning.Message,
		},
		{
			name:        "PVCs found but no warning events",
			pvcList:     []client.Object{pvcBound},
			expectedMsg: "",
		},
	}

	g := NewWithT(t)
	t.Parallel()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var existingObjects []client.Object
			if tc.pvcList != nil {
				existingObjects = append(existingObjects, tc.pvcList...)
			}
			if tc.eventList != nil {
				existingObjects = append(existingObjects, tc.eventList...)
			}

			cl := testutils.CreateTestFakeClientForObjectsInNamespaceWithGVK(tc.errors, etcd.Namespace, existingObjects...)
			messages, err := FetchPVCWarningMessagesForStatefulSet(context.Background(), cl, sts)
			if tc.expectedErr != nil {
				g.Expect(err).To(HaveOccurred())
				g.Expect(errors.Is(err, tc.expectedErr)).To(BeTrue())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(strings.Contains(messages, tc.expectedMsg)).To(BeTrue())
			}
		})
	}
}
