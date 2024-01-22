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
	"github.com/gardener/etcd-druid/internal/utils"
	testutils "github.com/gardener/etcd-druid/test/utils"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	gomegatypes "github.com/onsi/gomega/types"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	nonTargetEtcdName = "another-etcd"
)

var (
	internalErr    = errors.New("test internal error")
	apiInternalErr = apierrors.NewInternalError(internalErr)
)

// ------------------------ GetExistingResourceNames ------------------------
func TestGetExistingResourceNames(t *testing.T) {
	etcdBuilder := testutils.EtcdBuilderWithDefaults(testutils.TestEtcdName, testutils.TestNamespace)
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
				leases, err := newMemberLeases(etcd, tc.numExistingLeases)
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
	etcdBuilder := testutils.EtcdBuilderWithDefaults(testutils.TestEtcdName, testutils.TestNamespace)
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
				leases, err := newMemberLeases(etcd, tc.numExistingLeases)
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
		name              string
		etcdReplicas      int32 // original replicas
		numExistingLeases int
		deleteAllOfErr    *apierrors.StatusError
		expectedErr       *druiderr.DruidError
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
			name:              "only delete member leases and not snapshot leases",
			etcdReplicas:      3,
			numExistingLeases: 3,
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
	nonTargetEtcd := testutils.EtcdBuilderWithDefaults(nonTargetEtcdName, testutils.TestNamespace).Build()
	nonTargetLeaseNames := []string{"another-etcd-0", "another-etcd-1"}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// *************** set up existing environment *****************
			etcd := testutils.EtcdBuilderWithDefaults(testutils.TestEtcdName, testutils.TestNamespace).WithReplicas(tc.etcdReplicas).Build()
			fakeClientBuilder := testutils.NewFakeClientBuilder().WithDeleteAllOfError(tc.deleteAllOfErr)
			if tc.numExistingLeases > 0 {
				leases, err := newMemberLeases(etcd, tc.numExistingLeases)
				g.Expect(err).ToNot(HaveOccurred())
				for _, lease := range leases {
					fakeClientBuilder.WithObjects(lease)
				}
			}
			for _, nonTargetLeaseName := range nonTargetLeaseNames {
				fakeClientBuilder.WithObjects(testutils.CreateLease(nonTargetLeaseName, nonTargetEtcd.Namespace, nonTargetEtcd.Name, nonTargetEtcd.UID, common.MemberLeaseComponentName))
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
				nonTargetMemberLeases := getLatestMemberLeases(g, cl, nonTargetEtcd)
				g.Expect(nonTargetMemberLeases).To(HaveLen(len(nonTargetLeaseNames)))
				g.Expect(nonTargetMemberLeases).To(ConsistOf(memberLeases(nonTargetEtcd.Name, nonTargetEtcd.UID, int32(len(nonTargetLeaseNames)))))
			}
		})
	}
}

// ---------------------------- Helper Functions -----------------------------
func getLatestMemberLeases(g *WithT, cl client.Client, etcd *druidv1alpha1.Etcd) []coordinationv1.Lease {
	return doGetLatestLeases(g,
		cl,
		etcd,
		utils.MergeMaps[string, string](map[string]string{
			druidv1alpha1.LabelComponentKey: common.MemberLeaseComponentName,
		}, etcd.GetDefaultLabels()))
}

func doGetLatestLeases(g *WithT, cl client.Client, etcd *druidv1alpha1.Etcd, matchingLabels map[string]string) []coordinationv1.Lease {
	leases := &coordinationv1.LeaseList{}
	g.Expect(cl.List(context.Background(),
		leases,
		client.InNamespace(etcd.Namespace),
		client.MatchingLabels(matchingLabels))).To(Succeed())
	return leases.Items
}

func memberLeases(etcdName string, etcdUID types.UID, numLeases int32) []interface{} {
	var elements []interface{}
	for i := 0; i < int(numLeases); i++ {
		leaseName := fmt.Sprintf("%s-%d", etcdName, i)
		elements = append(elements, matchLeaseElement(leaseName, etcdName, etcdUID))
	}
	return elements
}

func matchLeaseElement(leaseName, etcdName string, etcdUID types.UID) gomegatypes.GomegaMatcher {
	return MatchFields(IgnoreExtras, Fields{
		"ObjectMeta": MatchFields(IgnoreExtras, Fields{
			"Name":            Equal(leaseName),
			"OwnerReferences": testutils.MatchEtcdOwnerReference(etcdName, etcdUID),
		}),
	})
}

func newMemberLeases(etcd *druidv1alpha1.Etcd, numLeases int) ([]*coordinationv1.Lease, error) {
	if numLeases > int(etcd.Spec.Replicas) {
		return nil, errors.New("number of requested leases is greater than the etcd replicas")
	}
	memberLeaseNames := etcd.GetMemberLeaseNames()
	leases := make([]*coordinationv1.Lease, 0, numLeases)
	for i := 0; i < numLeases; i++ {
		lease := &coordinationv1.Lease{
			ObjectMeta: metav1.ObjectMeta{
				Name:            memberLeaseNames[i],
				Namespace:       etcd.Namespace,
				Labels:          getLabels(etcd, memberLeaseNames[i]),
				OwnerReferences: []metav1.OwnerReference{etcd.GetAsOwnerReference()},
			},
		}
		leases = append(leases, lease)
	}
	return leases, nil
}
