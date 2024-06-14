// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package memberlease

import (
	"context"
	"errors"
	"fmt"
	"testing"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"
	"github.com/gardener/etcd-druid/internal/component"
	druiderr "github.com/gardener/etcd-druid/internal/errors"
	"github.com/gardener/etcd-druid/internal/utils"
	testutils "github.com/gardener/etcd-druid/test/utils"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	gomegatypes "github.com/onsi/gomega/types"
	coordinationv1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	nonTargetEtcdName = "another-etcd"
)

// ------------------------ GetExistingResourceNames ------------------------
func TestGetExistingResourceNames(t *testing.T) {
	etcdBuilder := testutils.EtcdBuilderWithDefaults(testutils.TestEtcdName, testutils.TestNamespace)
	testCases := []struct {
		name              string
		etcdReplicas      int32
		numExistingLeases int
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
			etcdReplicas:      3,
			numExistingLeases: 0,
		},
		{
			name:    "should return error when list fails",
			listErr: testutils.TestAPIInternalErr,
			expectedErr: &druiderr.DruidError{
				Code:      ErrListMemberLease,
				Cause:     testutils.TestAPIInternalErr,
				Operation: "GetExistingResourceNames",
			},
		},
	}

	g := NewWithT(t)
	t.Parallel()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			etcd := etcdBuilder.WithReplicas(tc.etcdReplicas).Build()
			var existingObjects []client.Object
			if tc.numExistingLeases > 0 {
				leases, err := newMemberLeases(etcd, tc.numExistingLeases)
				g.Expect(err).ToNot(HaveOccurred())
				for _, lease := range leases {
					existingObjects = append(existingObjects, lease)
				}
			}
			cl := testutils.CreateTestFakeClientForAllObjectsInNamespace(nil, tc.listErr, etcd.Namespace, getSelectorLabelsForAllMemberLeases(etcd.ObjectMeta), existingObjects...)
			operator := New(cl)
			opCtx := component.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
			memberLeaseNames, err := operator.GetExistingResourceNames(opCtx, etcd.ObjectMeta)
			if tc.expectedErr != nil {
				testutils.CheckDruidError(g, tc.expectedErr, err)
			} else {
				g.Expect(err).To(BeNil())
				expectedLeaseNames := druidv1alpha1.GetMemberLeaseNames(etcd.ObjectMeta, etcd.Spec.Replicas)[:tc.numExistingLeases]
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
			createErr:         testutils.TestAPIInternalErr,
			expectedErr: &druiderr.DruidError{
				Code:      ErrSyncMemberLease,
				Cause:     testutils.TestAPIInternalErr,
				Operation: "Sync",
			},
		},
		{
			name:              "should return error when client get fails",
			etcdReplicas:      3,
			numExistingLeases: 0,
			getErr:            testutils.TestAPIInternalErr,
			expectedErr: &druiderr.DruidError{
				Code:      ErrSyncMemberLease,
				Cause:     testutils.TestAPIInternalErr,
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
			var existingObjects []client.Object
			if tc.numExistingLeases > 0 {
				leases, err := newMemberLeases(etcd, tc.numExistingLeases)
				g.Expect(err).ToNot(HaveOccurred())
				for _, lease := range leases {
					existingObjects = append(existingObjects, lease)
				}
			}
			cl := testutils.CreateTestFakeClientForObjects(tc.getErr, tc.createErr, nil, nil, existingObjects, getObjectKeys(etcd)...)

			// ***************** Setup updated etcd to be passed to the Sync method *****************
			var updatedEtcd *druidv1alpha1.Etcd
			// Currently we only support up-scaling. If and when down-scaling is supported, this condition will have to change.
			if tc.deltaEtcdReplicas > 0 {
				updatedEtcd = etcdBuilder.WithReplicas(tc.etcdReplicas + tc.deltaEtcdReplicas).Build()
			} else {
				updatedEtcd = etcd
			}
			// ***************** Setup component operator and test *****************
			operator := New(cl)
			opCtx := component.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
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
			deleteAllOfErr:    testutils.TestAPIInternalErr,
			expectedErr: &druiderr.DruidError{
				Code:      ErrDeleteMemberLease,
				Cause:     testutils.TestAPIInternalErr,
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
			var existingObjects []client.Object
			if tc.numExistingLeases > 0 {
				leases, err := newMemberLeases(etcd, tc.numExistingLeases)
				g.Expect(err).ToNot(HaveOccurred())
				for _, lease := range leases {
					existingObjects = append(existingObjects, lease)
				}
			}
			for _, nonTargetLeaseName := range nonTargetLeaseNames {
				existingObjects = append(existingObjects, testutils.CreateLease(nonTargetLeaseName, nonTargetEtcd.Namespace, nonTargetEtcd.Name, nonTargetEtcd.UID, common.ComponentNameMemberLease))
			}
			cl := testutils.CreateTestFakeClientForAllObjectsInNamespace(tc.deleteAllOfErr, nil, etcd.Namespace, getSelectorLabelsForAllMemberLeases(etcd.ObjectMeta), existingObjects...)
			// ***************** Setup component operator and test *****************
			operator := New(cl)
			opCtx := component.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
			err := operator.TriggerDelete(opCtx, etcd.ObjectMeta)
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
		utils.MergeMaps(map[string]string{
			druidv1alpha1.LabelComponentKey: common.ComponentNameMemberLease,
		}, druidv1alpha1.GetDefaultLabels(etcd.ObjectMeta)))
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
	memberLeaseNames := druidv1alpha1.GetMemberLeaseNames(etcd.ObjectMeta, etcd.Spec.Replicas)
	leases := make([]*coordinationv1.Lease, 0, numLeases)
	for i := 0; i < numLeases; i++ {
		lease := &coordinationv1.Lease{
			ObjectMeta: metav1.ObjectMeta{
				Name:            memberLeaseNames[i],
				Namespace:       etcd.Namespace,
				Labels:          getLabels(etcd, memberLeaseNames[i]),
				OwnerReferences: []metav1.OwnerReference{druidv1alpha1.GetAsOwnerReference(etcd.ObjectMeta)},
			},
		}
		leases = append(leases, lease)
	}
	return leases, nil
}
