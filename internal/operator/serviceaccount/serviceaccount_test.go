package serviceaccount

import (
	"context"
	"errors"
	"testing"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	druiderr "github.com/gardener/etcd-druid/internal/errors"
	"github.com/gardener/etcd-druid/internal/operator/resource"
	testsample "github.com/gardener/etcd-druid/test/sample"
	testutils "github.com/gardener/etcd-druid/test/utils"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	gomegatypes "github.com/onsi/gomega/types"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

const (
	testEtcdName = "test-etcd"
	testNs       = "test-namespace"
)

var (
	internalErr    = errors.New("fake get internal error")
	apiInternalErr = apierrors.NewInternalError(internalErr)
)

// ------------------------ GetExistingResourceNames ------------------------
func TestGetExistingResourceNames(t *testing.T) {
	etcd := testsample.EtcdBuilderWithDefaults(testEtcdName, testNs).Build()
	testCases := []struct {
		name            string
		saExists        bool
		getErr          *apierrors.StatusError
		expectedErr     *druiderr.DruidError
		expectedSANames []string
	}{
		{
			name:            "should return empty slice, when no service account exists",
			saExists:        false,
			getErr:          apierrors.NewNotFound(corev1.Resource("serviceaccounts"), etcd.GetServiceAccountName()),
			expectedSANames: []string{},
		},
		{
			name:            "should return existing service account name",
			saExists:        true,
			expectedSANames: []string{etcd.GetServiceAccountName()},
		},
		{
			name:     "should return err when client get fails",
			saExists: true,
			getErr:   apiInternalErr,
			expectedErr: &druiderr.DruidError{
				Code:      ErrGetServiceAccount,
				Cause:     apiInternalErr,
				Operation: "GetExistingResourceNames",
			},
		},
	}

	g := NewWithT(t)
	t.Parallel()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fakeClientBuilder := testutils.NewFakeClientBuilder().WithGetError(tc.getErr)
			if tc.saExists {
				fakeClientBuilder.WithObjects(testsample.NewServiceAccount(etcd, false))
			}
			cl := fakeClientBuilder.Build()
			operator := New(cl, true)
			opCtx := resource.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
			saNames, err := operator.GetExistingResourceNames(opCtx, etcd)
			if tc.expectedErr != nil {
				testutils.CheckDruidError(g, tc.expectedErr, err)
			} else {
				if tc.saExists {
					existingSA, err := getLatestServiceAccount(cl, etcd)
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(saNames).To(HaveLen(1))
					g.Expect(saNames[0]).To(Equal(existingSA.Name))
				} else {
					g.Expect(saNames).To(HaveLen(0))
				}
			}
		})
	}
}

// ----------------------------------- Sync -----------------------------------
func TestSync(t *testing.T) {
	testCases := []struct {
		name             string
		disableAutoMount bool
		createErr        *apierrors.StatusError
		expectedErr      *druiderr.DruidError
	}{
		{
			name: "create service account when none exists",
		},
		{
			name:             "create service account with disabled auto mount",
			disableAutoMount: true,
		},
		{
			name:      "should return err when client create fails",
			createErr: apiInternalErr,
			expectedErr: &druiderr.DruidError{
				Code:      ErrSyncServiceAccount,
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
			etcd := testsample.EtcdBuilderWithDefaults(testEtcdName, testNs).Build()
			operator := New(cl, tc.disableAutoMount)
			opCtx := resource.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
			syncErr := operator.Sync(opCtx, etcd)
			latestSA, getErr := getLatestServiceAccount(cl, etcd)
			if tc.expectedErr != nil {
				testutils.CheckDruidError(g, tc.expectedErr, syncErr)
				g.Expect(apierrors.IsNotFound(getErr)).To(BeTrue())
			} else {
				g.Expect(getErr).To(BeNil())
				g.Expect(*latestSA).To(matchServiceAccount(etcd.GetServiceAccountName(), etcd.Namespace, etcd.Name, etcd.UID, tc.disableAutoMount))
			}
		})
	}
}

// ----------------------------- TriggerDelete -------------------------------
func TestTriggerDelete(t *testing.T) {
	testCases := []struct {
		name        string
		saExists    bool
		deleteErr   *apierrors.StatusError
		expectedErr *druiderr.DruidError
	}{
		{
			name:     "no-op when service account does not exist",
			saExists: false,
		},
		{
			name:     "successfully delete service account",
			saExists: true,
		},
		{
			name:      "returns error when client delete fails",
			saExists:  true,
			deleteErr: apiInternalErr,
			expectedErr: &druiderr.DruidError{
				Code:      ErrDeleteServiceAccount,
				Cause:     apiInternalErr,
				Operation: "TriggerDelete",
			},
		},
	}

	g := NewWithT(t)
	t.Parallel()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			etcd := testsample.EtcdBuilderWithDefaults(testEtcdName, testNs).Build()
			fakeClientBuilder := testutils.NewFakeClientBuilder().WithDeleteError(tc.deleteErr)
			if tc.saExists {
				fakeClientBuilder.WithObjects(testsample.NewServiceAccount(etcd, false))
			}
			cl := fakeClientBuilder.Build()
			operator := New(cl, false)
			opCtx := resource.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
			triggerDeleteErr := operator.TriggerDelete(opCtx, etcd)
			latestSA, getErr := getLatestServiceAccount(cl, etcd)
			if tc.expectedErr != nil {
				testutils.CheckDruidError(g, tc.expectedErr, triggerDeleteErr)
				g.Expect(getErr).To(BeNil())
				g.Expect(latestSA).ToNot(BeNil())
			} else {
				g.Expect(triggerDeleteErr).To(BeNil())
				g.Expect(apierrors.IsNotFound(getErr)).To(BeTrue())
			}
		})
	}
}

// ---------------------------- Helper Functions -----------------------------
func getLatestServiceAccount(cl client.Client, etcd *druidv1alpha1.Etcd) (*corev1.ServiceAccount, error) {
	sa := &corev1.ServiceAccount{}
	err := cl.Get(context.Background(), client.ObjectKey{Name: etcd.GetServiceAccountName(), Namespace: etcd.Namespace}, sa)
	return sa, err
}

func matchServiceAccount(saName, saNamespace, etcdName string, etcdUID types.UID, disableAutomount bool) gomegatypes.GomegaMatcher {
	return MatchFields(IgnoreExtras, Fields{
		"ObjectMeta": MatchFields(IgnoreExtras, Fields{
			"Name":      Equal(saName),
			"Namespace": Equal(saNamespace),
			"OwnerReferences": ConsistOf(MatchFields(IgnoreExtras, Fields{
				"APIVersion":         Equal(druidv1alpha1.GroupVersion.String()),
				"Kind":               Equal("Etcd"),
				"Name":               Equal(etcdName),
				"UID":                Equal(etcdUID),
				"Controller":         PointTo(BeTrue()),
				"BlockOwnerDeletion": PointTo(BeTrue()),
			})),
		}),
		"AutomountServiceAccountToken": PointTo(Equal(!disableAutomount)),
	})
}
