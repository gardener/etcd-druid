package utils

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"testing"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"
	testutils "github.com/gardener/etcd-druid/test/utils"
	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
	coordinationv1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestIsPeerURLTLSEnabledForAllMembers(t *testing.T) {
	internalErr := errors.New("fake get internal error")
	apiInternalErr := apierrors.NewInternalError(internalErr)
	const etcdReplicas = 3
	testCases := []struct {
		name                         string
		numETCDMembersWithTLSEnabled int
		listErr                      *apierrors.StatusError
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
			name:           "should return error when client list call fails",
			listErr:        apiInternalErr,
			expectedErr:    apiInternalErr,
			expectedResult: false,
		},
	}

	g := NewWithT(t)
	t.Parallel()
	logger := logr.Discard()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fakeClientBuilder := testutils.NewFakeClientBuilder().WithListError(tc.listErr)
			for _, l := range createLeases(testutils.TestNamespace, testutils.TestEtcdName, etcdReplicas, tc.numETCDMembersWithTLSEnabled) {
				fakeClientBuilder.WithObjects(l)
			}
			cl := fakeClientBuilder.Build()
			tlsEnabled, err := IsPeerURLTLSEnabledForAllMembers(context.Background(), cl, logger, testutils.TestNamespace, testutils.TestEtcdName)
			if tc.expectedErr != nil {
				g.Expect(err).To(Equal(tc.expectedErr))
			} else {
				g.Expect(tlsEnabled).To(Equal(tc.expectedResult))
			}
		})
	}
}

func createLeases(namespace, etcdName string, numLease, withTLSEnabled int) []*coordinationv1.Lease {
	leases := make([]*coordinationv1.Lease, 0, numLease)
	labels := map[string]string{
		druidv1alpha1.LabelComponentKey: common.MemberLeaseComponentName,
		druidv1alpha1.LabelPartOfKey:    etcdName,
		druidv1alpha1.LabelManagedByKey: druidv1alpha1.LabelManagedByValue,
	}
	tlsEnabledCount := 0
	for i := 0; i < numLease; i++ {
		var annotations map[string]string
		if tlsEnabledCount < withTLSEnabled {
			annotations = map[string]string{
				peerURLTLSEnabledKey: "true",
			}
			tlsEnabledCount++
		} else {
			annotations = randomizeAnnotations()
		}
		lease := &coordinationv1.Lease{
			ObjectMeta: metav1.ObjectMeta{
				Name:        fmt.Sprintf("%s-%d", etcdName, i),
				Namespace:   namespace,
				Annotations: annotations,
				Labels:      labels,
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
			peerURLTLSEnabledKey: "false",
		}
	}
	return nil
}
