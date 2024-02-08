package utils

import (
	"context"
	"errors"
	"testing"

	testutils "github.com/gardener/etcd-druid/test/utils"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/uuid"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	stsName      = "etcd-test-0"
	stsNamespace = "test-ns"
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
		listErr      *apierrors.StatusError
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
			name:         "returns error when client list fails",
			isStsPresent: true,
			ownedByEtcd:  true,
			listErr:      apiInternalErr,
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
			cl := testutils.CreateTestFakeClientForAllObjectsInNamespace(nil, tc.listErr, etcd.Namespace, etcd.GetDefaultLabels(), existingObjects...)
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
