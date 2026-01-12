// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"testing"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"
	testutils "github.com/gardener/etcd-druid/test/utils"

	"github.com/go-logr/logr"
	coordinationv1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/gomega"
)

func TestIsPeerURLTLSEnabledForAllMembers(t *testing.T) {
	internalErr := errors.New("fake get internal error")
	apiInternalErr := apierrors.NewInternalError(internalErr)
	etcd := testutils.EtcdBuilderWithDefaults(testutils.TestEtcdName, testutils.TestNamespace).WithReplicas(3).WithPeerTLS().Build()
	testCases := []struct {
		name                         string
		numETCDMembersWithTLSEnabled int
		errors                       []testutils.ErrorsForGVK
		expectedErr                  *apierrors.StatusError
		expectedResult               bool
	}{
		{
			name:                         "should return false when none of the members have peer TLS enabled",
			numETCDMembersWithTLSEnabled: 0,
			expectedResult:               false,
		},
		{
			name:                         "should return false when one of three do not have peer TLS enabled",
			numETCDMembersWithTLSEnabled: 2,
			expectedResult:               false,
		},
		{
			name:                         "should return true when all members have peer TLS enabled",
			numETCDMembersWithTLSEnabled: 3,
			expectedResult:               true,
		},
		{
			name: "should return error when client list call fails",
			errors: []testutils.ErrorsForGVK{
				{
					GVK:     coordinationv1.SchemeGroupVersion.WithKind("Lease"),
					ListErr: apiInternalErr,
				},
			},
			expectedErr:    apiInternalErr,
			expectedResult: false,
		},
	}

	g := NewWithT(t)
	t.Parallel()
	logger := logr.Discard()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var existingObjects []client.Object
			for _, l := range createLeases(etcd, tc.numETCDMembersWithTLSEnabled) {
				existingObjects = append(existingObjects, l)
			}
			cl := testutils.CreateTestFakeClientForObjectsInNamespaceWithGVK(tc.errors, testutils.TestNamespace, existingObjects...)
			tlsEnabled, err := IsPeerURLInSyncForAllMembers(context.Background(), cl, logger, etcd, etcd.Spec.Replicas)
			if tc.expectedErr != nil {
				g.Expect(err).To(Equal(tc.expectedErr))
			} else {
				g.Expect(tlsEnabled).To(Equal(tc.expectedResult))
			}
		})
	}
}

func TestIsPeerURLTLSDisabledForAllMembers(t *testing.T) {
	internalErr := errors.New("fake get internal error")
	apiInternalErr := apierrors.NewInternalError(internalErr)
	etcd := testutils.EtcdBuilderWithDefaults(testutils.TestEtcdName, testutils.TestNamespace).WithReplicas(3).Build()
	testCases := []struct {
		name                         string
		numETCDMembersWithTLSEnabled int
		errors                       []testutils.ErrorsForGVK
		expectedErr                  *apierrors.StatusError
		expectedResult               bool
	}{
		{
			name:                         "should return true when all of the members have peer TLS disabled",
			numETCDMembersWithTLSEnabled: 0,
			expectedResult:               true,
		},
		{
			name:                         "should return false when one of three still have peer TLS enabled",
			numETCDMembersWithTLSEnabled: 1,
			expectedResult:               false,
		},
		{
			name:                         "should return false when none of the members have peer TLS disabled",
			numETCDMembersWithTLSEnabled: 3,
			expectedResult:               false,
		},
		{
			name: "should return error when client list call fails",
			errors: []testutils.ErrorsForGVK{
				{
					GVK:     coordinationv1.SchemeGroupVersion.WithKind("Lease"),
					ListErr: apiInternalErr,
				},
			},
			expectedErr:    apiInternalErr,
			expectedResult: false,
		},
	}

	g := NewWithT(t)
	t.Parallel()
	logger := logr.Discard()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var existingObjects []client.Object
			for _, l := range createLeases(etcd, tc.numETCDMembersWithTLSEnabled) {
				existingObjects = append(existingObjects, l)
			}
			cl := testutils.CreateTestFakeClientForObjectsInNamespaceWithGVK(tc.errors, testutils.TestNamespace, existingObjects...)
			tlsEnabled, err := IsPeerURLInSyncForAllMembers(context.Background(), cl, logger, etcd, etcd.Spec.Replicas)
			if tc.expectedErr != nil {
				g.Expect(err).To(Equal(tc.expectedErr))
			} else {
				g.Expect(tlsEnabled).To(Equal(tc.expectedResult))
			}
		})
	}
}

func createLeases(etcd *druidv1alpha1.Etcd, withTLSEnabled int) []*coordinationv1.Lease {
	numLeases := int(etcd.Spec.Replicas)
	leases := make([]*coordinationv1.Lease, 0, numLeases)
	labels := map[string]string{
		druidv1alpha1.LabelComponentKey: common.ComponentNameMemberLease,
		druidv1alpha1.LabelPartOfKey:    etcd.Name,
		druidv1alpha1.LabelManagedByKey: druidv1alpha1.LabelManagedByValue,
	}
	tlsEnabledCount := 0
	for i := range numLeases {
		var annotations map[string]string
		if tlsEnabledCount < withTLSEnabled {
			annotations = map[string]string{
				LeaseAnnotationKeyPeerURLTLSEnabled: "true",
			}
			tlsEnabledCount++
		} else {
			annotations = randomizeAnnotations()
		}
		lease := &coordinationv1.Lease{
			ObjectMeta: metav1.ObjectMeta{
				Name:            fmt.Sprintf("%s-%d", etcd.Name, i),
				Namespace:       etcd.Namespace,
				Annotations:     annotations,
				Labels:          labels,
				OwnerReferences: []metav1.OwnerReference{druidv1alpha1.GetAsOwnerReference(etcd.ObjectMeta)},
			},
		}
		leases = append(leases, lease)
	}
	return leases
}

func randomizeAnnotations() map[string]string {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	rBool := r.Intn(2) == 1
	if rBool {
		return map[string]string{
			LeaseAnnotationKeyPeerURLTLSEnabled: "false",
		}
	}
	return nil
}
