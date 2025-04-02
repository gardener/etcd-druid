package etcd

import (
	"context"
	"testing"

	"github.com/gardener/etcd-druid/test/utils"

	. "github.com/onsi/gomega"
	"k8s.io/utils/ptr"
)

// TestValidateUpdateStatusClusterSize tests the validation of `Status.ClusterSize` field in the Etcd resource.
func TestValidateUpdateStatusClusterSize(t *testing.T) {
	skipCELTestsForOlderK8sVersions(t)

	testCases := []struct {
		name           string
		oldClusterSize *int32
		newClusterSize *int32
		expectErr      bool
	}{
		{
			name:           "valid-1",
			oldClusterSize: nil,
			newClusterSize: nil,
			expectErr:      false,
		},
		{
			name:           "valid-2",
			oldClusterSize: nil,
			newClusterSize: ptr.To[int32](0),
			expectErr:      false,
		},
		{
			name:           "valid-3",
			oldClusterSize: nil,
			newClusterSize: ptr.To[int32](3),
			expectErr:      false,
		},
		{
			name:           "valid-4",
			oldClusterSize: ptr.To[int32](3),
			newClusterSize: ptr.To[int32](3),
			expectErr:      false,
		},
		{
			name:           "valid-5",
			oldClusterSize: ptr.To[int32](3),
			newClusterSize: ptr.To[int32](5),
			expectErr:      false,
		},
		{
			name:           "invalid-1",
			oldClusterSize: ptr.To[int32](3),
			newClusterSize: ptr.To[int32](1),
			expectErr:      true,
		},
	}

	testNs, g := setupTestEnvironment(t)
	t.Parallel()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			etcd := utils.EtcdBuilderWithoutDefaults(tc.name, testNs).
				Build()
			cl := itTestEnv.GetClient()
			ctx := context.Background()
			g.Expect(cl.Create(ctx, etcd)).To(Succeed())
			// update status subresource, since status is not created in the object Create() call
			etcd.Status.ClusterSize = tc.oldClusterSize
			g.Expect(cl.Status().Update(ctx, etcd)).To(Succeed())

			// test status update with new clusterSize value
			etcd.Status.ClusterSize = tc.newClusterSize
			statusUpdateErr := cl.Status().Update(ctx, etcd)
			if tc.expectErr {
				g.Expect(statusUpdateErr).ToNot(BeNil())
			} else {
				g.Expect(statusUpdateErr).To(BeNil())
			}
		})
	}
}
