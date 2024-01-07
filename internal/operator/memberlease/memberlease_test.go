package memberlease

import (
	"context"
	"errors"
	"testing"

	"github.com/gardener/etcd-druid/internal/operator/resource"
	testsample "github.com/gardener/etcd-druid/test/sample"
	testutils "github.com/gardener/etcd-druid/test/utils"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

const (
	testEtcdName = "test-etcd"
	testNs       = "test-namespace"
)

var (
	internalErr = errors.New("test internal error")
)

// ------------------------ GetExistingResourceNames ------------------------
func TestGetExistingResourceNames(t *testing.T) {
	etcdBuilder := testsample.EtcdBuilderWithDefaults(testEtcdName, testNs)
	listErr := apierrors.NewInternalError(internalErr)
	testCases := []struct {
		name              string
		etcdReplicas      int32
		numExistingLeases int
		getErr            *apierrors.StatusError
		listErr           *apierrors.StatusError
		expectedErr       *apierrors.StatusError
	}{
		{
			name:              "lease exists for a single node etcd cluster",
			etcdReplicas:      1,
			numExistingLeases: 1,
		},
		{
			name:              "all leases exist for a 3 node etcd cluster",
			etcdReplicas:      3,
			numExistingLeases: 3,
		},
		{
			name:              "2 of 3 leases exist for a 3 node etcd cluster",
			etcdReplicas:      3,
			numExistingLeases: 2,
		},
		{
			name:              "should return an empty slice when no member leases are found",
			getErr:            apierrors.NewNotFound(corev1.Resource("leases"), ""),
			etcdReplicas:      3,
			numExistingLeases: 0,
		},
		{
			name:        "should return error when list fails",
			listErr:     listErr,
			expectedErr: listErr,
		},
	}

	g := NewWithT(t)
	t.Parallel()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			etcd := etcdBuilder.WithReplicas(tc.etcdReplicas).Build()
			fakeClientBuilder := testutils.NewFakeClientBuilder()
			if tc.getErr != nil {
				fakeClientBuilder.WithGetError(tc.getErr)
			}
			if tc.listErr != nil {
				fakeClientBuilder.WithListError(tc.listErr)
			}
			if tc.numExistingLeases > 0 {
				leases, err := testsample.NewMemberLeases(etcd, tc.numExistingLeases)
				g.Expect(err).ToNot(HaveOccurred())
				for _, lease := range leases {
					fakeClientBuilder.WithObjects(lease)
				}
			}
			operator := New(fakeClientBuilder.Build())
			opCtx := resource.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
			memberLeaseNames, err := operator.GetExistingResourceNames(opCtx, etcd)
			if tc.expectedErr != nil {
				g.Expect(errors.Is(err, tc.expectedErr)).To(BeTrue())
			} else {
				g.Expect(err).To(BeNil())
			}
			g.Expect(len(memberLeaseNames), tc.numExistingLeases)
		})
	}
}
