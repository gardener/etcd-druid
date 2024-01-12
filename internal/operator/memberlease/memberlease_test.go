package memberlease

import (
	"context"
	"errors"
	"fmt"
	"testing"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"
	druiderr "github.com/gardener/etcd-druid/internal/errors"
	"github.com/gardener/etcd-druid/internal/operator/resource"
	testsample "github.com/gardener/etcd-druid/test/sample"
	testutils "github.com/gardener/etcd-druid/test/utils"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	gomegatypes "github.com/onsi/gomega/types"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	testEtcdName = "test-etcd"
	testNs       = "test-namespace"
)

var (
	internalErr    = errors.New("test internal error")
	apiInternalErr = apierrors.NewInternalError(internalErr)
)

// ------------------------ GetExistingResourceNames ------------------------
func TestGetExistingResourceNames(t *testing.T) {
	etcdBuilder := testsample.EtcdBuilderWithDefaults(testEtcdName, testNs)
	testCases := []struct {
		name              string
		etcdReplicas      int32
		numExistingLeases int
		getErr            *apierrors.StatusError
		listErr           *apierrors.StatusError
		expectedErr       *druiderr.DruidError
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
			name:    "should return error when list fails",
			listErr: apiInternalErr,
			expectedErr: &druiderr.DruidError{
				Code:      ErrListMemberLease,
				Cause:     apiInternalErr,
				Operation: "GetExistingResourceNames",
			},
		},
	}

	g := NewWithT(t)
	t.Parallel()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			etcd := etcdBuilder.WithReplicas(tc.etcdReplicas).Build()
			fakeClientBuilder := testutils.NewFakeClientBuilder().
				WithGetError(tc.getErr).
				WithListError(tc.listErr)
			if tc.numExistingLeases > 0 {
				leases, err := testsample.NewMemberLeases(etcd, tc.numExistingLeases, getAdditionalMemberLeaseLabels(etcd.Name))
				g.Expect(err).ToNot(HaveOccurred())
				for _, lease := range leases {
					fakeClientBuilder.WithObjects(lease)
				}
			}
			operator := New(fakeClientBuilder.Build())
			opCtx := resource.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
			memberLeaseNames, err := operator.GetExistingResourceNames(opCtx, etcd)
			if tc.expectedErr != nil {
				testutils.CheckDruidError(g, tc.expectedErr, err)
			} else {
				g.Expect(err).To(BeNil())
				expectedLeaseNames := etcd.GetMemberLeaseNames()[:tc.numExistingLeases]
				g.Expect(memberLeaseNames).To(Equal(expectedLeaseNames))
			}
		})
	}
}

// ----------------------------------- Sync -----------------------------------
func TestSync(t *testing.T) {
	etcdBuilder := testsample.EtcdBuilderWithDefaults(testEtcdName, testNs)
	testCases := []struct {
		name              string
		etcdReplicas      int32 // original replicas
		deltaEtcdReplicas int32 // change in etcd replicas if any
		numExistingLeases int
		createErr         *apierrors.StatusError
		getErr            *apierrors.StatusError
		expectedErr       *druiderr.DruidError
	}{
		{
			name:              "create member leases for a single node etcd cluster",
			etcdReplicas:      1,
			numExistingLeases: 0,
		},
		{
			name:              "creates member leases when etcd replicas is changes from 1 -> 3",
			etcdReplicas:      1,
			numExistingLeases: 1,
			deltaEtcdReplicas: 2,
		},
		{
			name:              "should return error when client create fails",
			etcdReplicas:      3,
			numExistingLeases: 0,
			createErr:         apiInternalErr,
			expectedErr: &druiderr.DruidError{
				Code:      ErrSyncMemberLease,
				Cause:     apiInternalErr,
				Operation: "Sync",
			},
		},
		{
			name:              "should return error when client get fails",
			etcdReplicas:      3,
			numExistingLeases: 0,
			getErr:            apiInternalErr,
			expectedErr: &druiderr.DruidError{
				Code:      ErrSyncMemberLease,
				Cause:     apiInternalErr,
				Operation: "Sync",
			},
		},
	}

	g := NewWithT(t)
	t.Parallel()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// *************** set up existing environment *****************
			etcd := etcdBuilder.WithReplicas(tc.etcdReplicas).Build()
			fakeClientBuilder := testutils.NewFakeClientBuilder().WithCreateError(tc.createErr).WithGetError(tc.getErr)
			if tc.numExistingLeases > 0 {
				leases, err := testsample.NewMemberLeases(etcd, tc.numExistingLeases, getAdditionalMemberLeaseLabels(etcd.Name))
				g.Expect(err).ToNot(HaveOccurred())
				for _, lease := range leases {
					fakeClientBuilder.WithObjects(lease)
				}
			}
			cl := fakeClientBuilder.Build()

			// ***************** Setup updated etcd to be passed to the Sync method *****************
			var updatedEtcd *druidv1alpha1.Etcd
			// Currently we only support up-scaling. If and when down-scaling is supported, this condition will have to change.
			if tc.deltaEtcdReplicas > 0 {
				updatedEtcd = etcdBuilder.WithReplicas(tc.etcdReplicas + tc.deltaEtcdReplicas).Build()
			} else {
				updatedEtcd = etcd
			}
			// ***************** Setup operator and test *****************
			operator := New(cl)
			opCtx := resource.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
			err := operator.Sync(opCtx, updatedEtcd)
			memberLeasesPostSync := getLatestMemberLeases(g, cl, updatedEtcd)
			if tc.expectedErr != nil {
				testutils.CheckDruidError(g, tc.expectedErr, err)
				g.Expect(memberLeasesPostSync).Should(HaveLen(tc.numExistingLeases))
			} else {
				g.Expect(memberLeasesPostSync).To(ConsistOf(memberLeases(updatedEtcd.Name, updatedEtcd.UID, updatedEtcd.Spec.Replicas)))
			}
		})
	}
}

// ----------------------------- TriggerDelete -------------------------------
func TestTriggerDelete(t *testing.T) {
	testCases := []struct {
		name                   string
		etcdReplicas           int32 // original replicas
		numExistingLeases      int
		testWithSnapshotLeases bool // this is to ensure that delete of member leases should not delete snapshot leases
		deleteAllOfErr         *apierrors.StatusError
		expectedErr            *druiderr.DruidError
	}{
		{
			name:              "no-op when no member lease exists",
			etcdReplicas:      3,
			numExistingLeases: 0,
		},
		{
			name:              "successfully deletes all member leases",
			etcdReplicas:      3,
			numExistingLeases: 3,
		},
		{
			name:                   "only delete member leases and not snapshot leases",
			etcdReplicas:           3,
			numExistingLeases:      3,
			testWithSnapshotLeases: true,
		},
		{
			name:              "successfully deletes remainder member leases",
			etcdReplicas:      3,
			numExistingLeases: 1,
		},
		{
			name:              "returns error when client delete fails",
			etcdReplicas:      3,
			numExistingLeases: 3,
			deleteAllOfErr:    apiInternalErr,
			expectedErr: &druiderr.DruidError{
				Code:      ErrDeleteMemberLease,
				Cause:     apiInternalErr,
				Operation: "TriggerDelete",
			},
		},
	}

	g := NewWithT(t)
	t.Parallel()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// *************** set up existing environment *****************
			etcd := testsample.EtcdBuilderWithDefaults(testEtcdName, testNs).WithReplicas(tc.etcdReplicas).Build()
			fakeClientBuilder := testutils.NewFakeClientBuilder().WithDeleteAllOfError(tc.deleteAllOfErr)
			if tc.numExistingLeases > 0 {
				leases, err := testsample.NewMemberLeases(etcd, tc.numExistingLeases, getAdditionalMemberLeaseLabels(etcd.Name))
				g.Expect(err).ToNot(HaveOccurred())
				for _, lease := range leases {
					fakeClientBuilder.WithObjects(lease)
				}
			}
			if tc.testWithSnapshotLeases {
				fakeClientBuilder.WithObjects(testsample.NewDeltaSnapshotLease(etcd), testsample.NewFullSnapshotLease(etcd))
			}
			cl := fakeClientBuilder.Build()
			// ***************** Setup operator and test *****************
			operator := New(cl)
			opCtx := resource.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
			memberLeasesBeforeDelete := getLatestMemberLeases(g, cl, etcd)
			fmt.Println(memberLeasesBeforeDelete)
			err := operator.TriggerDelete(opCtx, etcd)
			memberLeasesPostDelete := getLatestMemberLeases(g, cl, etcd)
			if tc.expectedErr != nil {
				testutils.CheckDruidError(g, tc.expectedErr, err)
				g.Expect(memberLeasesPostDelete).Should(HaveLen(tc.numExistingLeases))
			} else {
				g.Expect(memberLeasesPostDelete).Should(HaveLen(0))
				if tc.testWithSnapshotLeases {
					snapshotLeases := getLatestSnapshotLeases(g, cl, etcd)
					g.Expect(snapshotLeases).To(HaveLen(2))
				}
			}
		})
	}
}

// ---------------------------- Helper Functions -----------------------------
func getAdditionalMemberLeaseLabels(etcdName string) map[string]string {
	return map[string]string{
		common.GardenerOwnedBy:           etcdName,
		v1beta1constants.GardenerPurpose: purpose,
	}
}

func getLatestMemberLeases(g *WithT, cl client.Client, etcd *druidv1alpha1.Etcd) []coordinationv1.Lease {
	return doGetLatestLeases(g, cl, etcd, map[string]string{
		common.GardenerOwnedBy:           etcd.Name,
		v1beta1constants.GardenerPurpose: purpose,
	})
}

func getLatestSnapshotLeases(g *WithT, cl client.Client, etcd *druidv1alpha1.Etcd) []coordinationv1.Lease {
	return doGetLatestLeases(g, cl, etcd, map[string]string{
		common.GardenerOwnedBy:           etcd.Name,
		v1beta1constants.GardenerPurpose: "etcd-snapshot-lease",
	})
}

func doGetLatestLeases(g *WithT, cl client.Client, etcd *druidv1alpha1.Etcd, matchingLabels map[string]string) []coordinationv1.Lease {
	leases := &coordinationv1.LeaseList{}
	g.Expect(cl.List(context.Background(),
		leases,
		client.InNamespace(etcd.Namespace),
		client.MatchingLabels(matchingLabels))).To(Succeed())
	return leases.Items
}

func memberLeases(etcdName string, etcdUID types.UID, replicas int32) []interface{} {
	var elements []interface{}
	for i := 0; i < int(replicas); i++ {
		leaseName := fmt.Sprintf("%s-%d", etcdName, i)
		elements = append(elements, matchLeaseElement(leaseName, etcdName, etcdUID))
	}
	return elements
}

func matchLeaseElement(leaseName, etcdName string, etcdUID types.UID) gomegatypes.GomegaMatcher {
	return MatchFields(IgnoreExtras, Fields{
		"ObjectMeta": MatchFields(IgnoreExtras, Fields{
			"Name": Equal(leaseName),
			"OwnerReferences": ConsistOf(MatchFields(IgnoreExtras, Fields{
				"APIVersion":         Equal(druidv1alpha1.GroupVersion.String()),
				"Kind":               Equal("Etcd"),
				"Name":               Equal(etcdName),
				"UID":                Equal(etcdUID),
				"Controller":         PointTo(BeTrue()),
				"BlockOwnerDeletion": PointTo(BeTrue()),
			})),
		}),
	})
}
