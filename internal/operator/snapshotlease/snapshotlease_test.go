package snapshotlease

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
	testsample "github.com/gardener/etcd-druid/test/sample"
	testutils "github.com/gardener/etcd-druid/test/utils"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	gomegatypes "github.com/onsi/gomega/types"
	coordinationv1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
	etcdBuilder := testsample.EtcdBuilderWithoutDefaults(testEtcdName, testNs)
	testCases := []struct {
		name               string
		backupEnabled      bool
		getErr             *apierrors.StatusError
		expectedLeaseNames []string
		expectedErr        *druiderr.DruidError
	}{
		{
			name:               "no snapshot leases created when backup is disabled",
			backupEnabled:      false,
			expectedLeaseNames: []string{},
		},
		{
			name:          "successfully returns delta and full snapshot leases",
			backupEnabled: true,
			expectedLeaseNames: []string{
				fmt.Sprintf("%s-delta-snap", testEtcdName),
				fmt.Sprintf("%s-full-snap", testEtcdName),
			},
		},
		{
			name:          "returns error when client get fails",
			backupEnabled: true,
			getErr:        apiInternalErr,
			expectedErr: &druiderr.DruidError{
				Code:      ErrGetSnapshotLease,
				Cause:     apiInternalErr,
				Operation: "GetExistingResourceNames",
			},
		},
	}

	g := NewWithT(t)
	t.Parallel()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.backupEnabled {
				etcdBuilder.WithDefaultBackup()
			}
			etcd := etcdBuilder.Build()
			fakeClientBuilder := testutils.NewFakeClientBuilder().WithGetError(tc.getErr)
			if tc.backupEnabled {
				fakeClientBuilder.WithObjects(testsample.NewDeltaSnapshotLease(etcd), testsample.NewFullSnapshotLease(etcd))
			}
			operator := New(fakeClientBuilder.Build())
			opCtx := resource.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
			actualSnapshotLeaseNames, err := operator.GetExistingResourceNames(opCtx, etcd)
			if tc.expectedErr != nil {
				testutils.CheckDruidError(g, tc.expectedErr, err)
			} else {
				g.Expect(err).To(BeNil())
				g.Expect(actualSnapshotLeaseNames, tc.expectedLeaseNames)
			}
		})
	}
}

// ----------------------------------- Sync -----------------------------------
func TestSyncWhenBackupIsEnabled(t *testing.T) {
	etcd := testsample.EtcdBuilderWithDefaults(testEtcdName, testNs).Build()
	testCases := []struct {
		name        string
		createErr   *apierrors.StatusError
		expectedErr *druiderr.DruidError
	}{
		{
			name: "create snapshot lease when backup is enabled",
		},
		{
			name:      "returns error when client create fails",
			createErr: apiInternalErr,
			expectedErr: &druiderr.DruidError{
				Code:      ErrSyncSnapshotLease,
				Cause:     apiInternalErr,
				Operation: "Sync",
			},
		},
	}

	g := NewWithT(t)
	t.Parallel()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cl := testutils.NewFakeClientBuilder().WithCreateError(tc.createErr).Build()
			operator := New(cl)
			opCtx := resource.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
			syncErr := operator.Sync(opCtx, etcd)
			latestSnapshotLeases, listErr := getLatestSnapshotLeases(cl, etcd)
			if tc.expectedErr != nil {
				testutils.CheckDruidError(g, tc.expectedErr, syncErr)
				g.Expect(listErr).ToNot(HaveOccurred())
				g.Expect(latestSnapshotLeases).To(BeEmpty())
			} else {
				g.Expect(listErr).ToNot(HaveOccurred())
				g.Expect(latestSnapshotLeases).To(ConsistOf(matchLease(etcd.GetDeltaSnapshotLeaseName(), etcd), matchLease(etcd.GetFullSnapshotLeaseName(), etcd)))
			}
		})
	}
}

func TestSyncWhenBackupHasBeenDisabled(t *testing.T) {
	existingEtcd := testsample.EtcdBuilderWithDefaults(testEtcdName, testNs).Build()   // backup is enabled
	updatedEtcd := testsample.EtcdBuilderWithoutDefaults(testEtcdName, testNs).Build() // backup is disabled
	testCases := []struct {
		name           string
		deleteAllOfErr *apierrors.StatusError
		expectedErr    *druiderr.DruidError
	}{
		{
			name: "deletes snapshot leases when backup has been disabled",
		},
		{
			name:           "returns error when client delete fails",
			deleteAllOfErr: apiInternalErr,
			expectedErr: &druiderr.DruidError{
				Code:      ErrSyncSnapshotLease,
				Cause:     apiInternalErr,
				Operation: "Sync",
			},
		},
	}

	g := NewWithT(t)
	t.Parallel()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cl := testutils.NewFakeClientBuilder().
				WithDeleteAllOfError(tc.deleteAllOfErr).
				WithObjects(testsample.NewDeltaSnapshotLease(existingEtcd), testsample.NewFullSnapshotLease(existingEtcd)).
				Build()
			operator := New(cl)
			opCtx := resource.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
			syncErr := operator.Sync(opCtx, updatedEtcd)
			latestSnapshotLeases, listErr := getLatestSnapshotLeases(cl, updatedEtcd)
			g.Expect(listErr).ToNot(HaveOccurred())
			if tc.expectedErr != nil {
				testutils.CheckDruidError(g, tc.expectedErr, syncErr)
				g.Expect(latestSnapshotLeases).To(HaveLen(2))
			} else {
				g.Expect(latestSnapshotLeases).To(HaveLen(0))
			}
		})
	}
}

// ----------------------------- TriggerDelete -------------------------------
func TestTriggerDelete(t *testing.T) {
	etcdBuilder := testsample.EtcdBuilderWithoutDefaults(testEtcdName, testNs).WithReplicas(3)
	testCases := []struct {
		name          string
		backupEnabled bool
		deleteAllErr  *apierrors.StatusError
		expectedErr   *druiderr.DruidError
	}{
		{
			name:          "no-op when backup is not enabled",
			backupEnabled: false,
		},
		{
			name:          "should only delete snapshot leases when backup is enabled",
			backupEnabled: true,
		},
		{
			name:          "should return error when client delete-all fails",
			backupEnabled: true,
			deleteAllErr:  apiInternalErr,
			expectedErr: &druiderr.DruidError{
				Code:      ErrDeleteSnapshotLease,
				Cause:     apiInternalErr,
				Operation: "TriggerDelete",
			},
		},
	}

	g := NewWithT(t)
	t.Parallel()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.backupEnabled {
				etcdBuilder.WithDefaultBackup()
			}
			etcd := etcdBuilder.Build()
			fakeClientBuilder := testutils.NewFakeClientBuilder().WithDeleteAllOfError(tc.deleteAllErr)
			memberLeases, err := testsample.NewMemberLeases(etcd, int(etcd.Spec.Replicas), map[string]string{
				common.GardenerOwnedBy:           etcd.Name,
				v1beta1constants.GardenerPurpose: "etcd-member-lease",
			})
			g.Expect(err).ToNot(HaveOccurred())
			for _, lease := range memberLeases {
				fakeClientBuilder.WithObjects(lease)
			}
			if tc.backupEnabled {
				fakeClientBuilder.WithObjects(testsample.NewDeltaSnapshotLease(etcd), testsample.NewFullSnapshotLease(etcd))
			}
			cl := fakeClientBuilder.Build()
			operator := New(cl)
			opCtx := resource.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
			triggerDeleteErr := operator.TriggerDelete(opCtx, etcd)
			latestSnapshotLeases, snapshotLeaseListErr := getLatestSnapshotLeases(cl, etcd)
			latestMemberLeases, memberLeaseListErr := getLatestMemberLeases(cl, etcd)
			if tc.expectedErr != nil {
				testutils.CheckDruidError(g, tc.expectedErr, triggerDeleteErr)
				g.Expect(snapshotLeaseListErr).ToNot(HaveOccurred())
				g.Expect(latestSnapshotLeases).To(HaveLen(2))
				g.Expect(memberLeaseListErr).ToNot(HaveOccurred())
				g.Expect(latestMemberLeases).To(HaveLen(int(etcd.Spec.Replicas)))
			} else {
				g.Expect(triggerDeleteErr).ToNot(HaveOccurred())
				g.Expect(snapshotLeaseListErr).ToNot(HaveOccurred())
				g.Expect(latestSnapshotLeases).To(HaveLen(0))
				g.Expect(memberLeaseListErr).ToNot(HaveOccurred())
				g.Expect(latestMemberLeases).To(HaveLen(int(etcd.Spec.Replicas)))
			}
		})
	}
}

// ---------------------------- Helper Functions -----------------------------

func matchLease(leaseName string, etcd *druidv1alpha1.Etcd) gomegatypes.GomegaMatcher {
	expectedLabels := utils.MergeMaps[string, string](etcd.GetDefaultLabels(), map[string]string{
		common.GardenerOwnedBy:           etcd.Name,
		v1beta1constants.GardenerPurpose: purpose,
	})
	return MatchFields(IgnoreExtras, Fields{
		"ObjectMeta": MatchFields(IgnoreExtras, Fields{
			"Name":      Equal(leaseName),
			"Namespace": Equal(etcd.Namespace),
			"Labels":    Equal(expectedLabels),
			"OwnerReferences": ConsistOf(MatchFields(IgnoreExtras, Fields{
				"APIVersion":         Equal(druidv1alpha1.GroupVersion.String()),
				"Kind":               Equal("Etcd"),
				"Name":               Equal(etcd.Name),
				"UID":                Equal(etcd.UID),
				"Controller":         PointTo(BeTrue()),
				"BlockOwnerDeletion": PointTo(BeTrue()),
			})),
		}),
	})
}

func getLatestMemberLeases(cl client.Client, etcd *druidv1alpha1.Etcd) ([]coordinationv1.Lease, error) {
	return doGetLatestLeases(cl, etcd, map[string]string{
		common.GardenerOwnedBy:           etcd.Name,
		v1beta1constants.GardenerPurpose: "etcd-member-lease",
	})
}

func getLatestSnapshotLeases(cl client.Client, etcd *druidv1alpha1.Etcd) ([]coordinationv1.Lease, error) {
	return doGetLatestLeases(cl, etcd, map[string]string{
		common.GardenerOwnedBy:           etcd.Name,
		v1beta1constants.GardenerPurpose: purpose,
	})
}

func doGetLatestLeases(cl client.Client, etcd *druidv1alpha1.Etcd, matchingLabels map[string]string) ([]coordinationv1.Lease, error) {
	leases := &coordinationv1.LeaseList{}
	err := cl.List(context.Background(),
		leases,
		client.InNamespace(etcd.Namespace),
		client.MatchingLabels(matchingLabels))
	if err != nil {
		return nil, err
	}
	return leases.Items, nil
}
