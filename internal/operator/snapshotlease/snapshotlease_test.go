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
	testutils "github.com/gardener/etcd-druid/test/utils"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	gomegatypes "github.com/onsi/gomega/types"
	coordinationv1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	testEtcdName      = "test-etcd"
	nonTargetEtcdName = "another-etcd"
	testNs            = "test-namespace"
)

var (
	internalErr    = errors.New("test internal error")
	apiInternalErr = apierrors.NewInternalError(internalErr)
)

// ------------------------ GetExistingResourceNames ------------------------
func TestGetExistingResourceNames(t *testing.T) {
	etcdBuilder := testutils.EtcdBuilderWithoutDefaults(testEtcdName, testNs)
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
				fakeClientBuilder.WithObjects(newDeltaSnapshotLease(etcd), newFullSnapshotLease(etcd))
			}
			operator := New(fakeClientBuilder.Build())
			opCtx := resource.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
			actualSnapshotLeaseNames, err := operator.GetExistingResourceNames(opCtx, etcd)
			if tc.expectedErr != nil {
				testutils.CheckDruidError(g, tc.expectedErr, err)
			} else {
				g.Expect(err).To(BeNil())
				g.Expect(actualSnapshotLeaseNames).To(Equal(tc.expectedLeaseNames))
			}
		})
	}
}

// ----------------------------------- Sync -----------------------------------
func TestSyncWhenBackupIsEnabled(t *testing.T) {
	etcd := testutils.EtcdBuilderWithDefaults(testEtcdName, testNs).Build()
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
	nonTargetEtcd := testutils.EtcdBuilderWithDefaults(nonTargetEtcdName, testNs).Build()
	existingEtcd := testutils.EtcdBuilderWithDefaults(testEtcdName, testNs).Build()   // backup is enabled
	updatedEtcd := testutils.EtcdBuilderWithoutDefaults(testEtcdName, testNs).Build() // backup is disabled
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
				WithObjects(
					newDeltaSnapshotLease(existingEtcd),
					newFullSnapshotLease(existingEtcd),
					newDeltaSnapshotLease(nonTargetEtcd),
					newFullSnapshotLease(nonTargetEtcd)).
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
				// To ensure that delete of snapshot leases did not remove non-target- snapshot leases also check that these still exist
				actualNonTargetSnapshotLeases, nonTargetSnapshotListErr := getLatestNonTargetSnapshotLeases(cl, nonTargetEtcd)
				g.Expect(nonTargetSnapshotListErr).To(BeNil())
				g.Expect(actualNonTargetSnapshotLeases).To(HaveLen(2))
			}
		})
	}
}

// ----------------------------- TriggerDelete -------------------------------
func TestTriggerDelete(t *testing.T) {
	nonTargetEtcd := testutils.EtcdBuilderWithDefaults(nonTargetEtcdName, testNs).Build()
	etcdBuilder := testutils.EtcdBuilderWithoutDefaults(testEtcdName, testNs).WithReplicas(3)
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
			fakeClientBuilder := testutils.NewFakeClientBuilder().
				WithDeleteAllOfError(tc.deleteAllErr).
				WithObjects(
					newDeltaSnapshotLease(nonTargetEtcd),
					newFullSnapshotLease(nonTargetEtcd),
				)
			if tc.backupEnabled {
				fakeClientBuilder.WithObjects(newDeltaSnapshotLease(etcd), newFullSnapshotLease(etcd))
			}
			cl := fakeClientBuilder.Build()
			operator := New(cl)
			opCtx := resource.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
			triggerDeleteErr := operator.TriggerDelete(opCtx, etcd)
			latestSnapshotLeases, snapshotLeaseListErr := getLatestSnapshotLeases(cl, etcd)
			if tc.expectedErr != nil {
				testutils.CheckDruidError(g, tc.expectedErr, triggerDeleteErr)
				g.Expect(snapshotLeaseListErr).ToNot(HaveOccurred())
				g.Expect(latestSnapshotLeases).To(HaveLen(2))
			} else {
				g.Expect(triggerDeleteErr).ToNot(HaveOccurred())
				g.Expect(snapshotLeaseListErr).ToNot(HaveOccurred())
				g.Expect(latestSnapshotLeases).To(HaveLen(0))
			}
			actualNonTargetSnapshotLeases, nonTargetSnapshotListErr := getLatestNonTargetSnapshotLeases(cl, nonTargetEtcd)
			g.Expect(nonTargetSnapshotListErr).ToNot(HaveOccurred())
			g.Expect(actualNonTargetSnapshotLeases).To(HaveLen(2))
		})
	}
}

// ---------------------------- Helper Functions -----------------------------
func newDeltaSnapshotLease(etcd *druidv1alpha1.Etcd) *coordinationv1.Lease {
	leaseName := etcd.GetDeltaSnapshotLeaseName()
	return buildLease(etcd, leaseName)
}

func newFullSnapshotLease(etcd *druidv1alpha1.Etcd) *coordinationv1.Lease {
	leaseName := etcd.GetFullSnapshotLeaseName()
	return buildLease(etcd, leaseName)
}

func buildLease(etcd *druidv1alpha1.Etcd, leaseName string) *coordinationv1.Lease {
	return &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      leaseName,
			Namespace: etcd.Namespace,
			Labels: utils.MergeMaps[string, string](etcd.GetDefaultLabels(), map[string]string{
				"gardener.cloud/owned-by":        etcd.Name,
				v1beta1constants.GardenerPurpose: "etcd-snapshot-lease",
			}),
			OwnerReferences: []metav1.OwnerReference{etcd.GetAsOwnerReference()},
		},
	}
}

func matchLease(leaseName string, etcd *druidv1alpha1.Etcd) gomegatypes.GomegaMatcher {
	expectedLabels := utils.MergeMaps[string, string](etcd.GetDefaultLabels(), map[string]string{
		common.GardenerOwnedBy:           etcd.Name,
		v1beta1constants.GardenerPurpose: purpose,
	})
	return MatchFields(IgnoreExtras, Fields{
		"ObjectMeta": MatchFields(IgnoreExtras, Fields{
			"Name":            Equal(leaseName),
			"Namespace":       Equal(etcd.Namespace),
			"Labels":          testutils.MatchResourceLabels(expectedLabels),
			"OwnerReferences": testutils.MatchEtcdOwnerReference(etcd.Name, etcd.UID),
		}),
	})
}

func getLatestNonTargetSnapshotLeases(cl client.Client, etcd *druidv1alpha1.Etcd) ([]coordinationv1.Lease, error) {
	return doGetLatestLeases(cl, etcd, map[string]string{
		common.GardenerOwnedBy:           etcd.Name,
		v1beta1constants.GardenerPurpose: "etcd-snapshot-lease",
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
