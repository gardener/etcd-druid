package poddistruptionbudget

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
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/gomega"
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
		name             string
		pdbExists        bool
		getErr           *apierrors.StatusError
		expectedErr      *druiderr.DruidError
		expectedPDBNames []string
	}{
		{
			name:             "should return the existing PDB",
			pdbExists:        true,
			expectedPDBNames: []string{etcd.Name},
		},
		{
			name:             "should return empty slice when PDB is not found",
			pdbExists:        false,
			getErr:           apierrors.NewNotFound(corev1.Resource("services"), etcd.GetClientServiceName()),
			expectedPDBNames: []string{},
		},
		{
			name:      "should return error when get fails",
			pdbExists: true,
			getErr:    apiInternalErr,
			expectedErr: &druiderr.DruidError{
				Code:      ErrGetPodDisruptionBudget,
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
			if tc.pdbExists {
				fakeClientBuilder.WithObjects(testsample.NewPodDisruptionBudget(etcd))
			}
			operator := New(fakeClientBuilder.Build())
			opCtx := resource.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
			pdbNames, err := operator.GetExistingResourceNames(opCtx, etcd)
			if tc.expectedErr != nil {
				testutils.CheckDruidError(g, tc.expectedErr, err)
			} else {
				g.Expect(err).To(BeNil())
				g.Expect(pdbNames).To(Equal(tc.expectedPDBNames))
			}
		})
	}
}

// ----------------------------------- Sync -----------------------------------
func TestSyncWhenNoPDBExists(t *testing.T) {
	etcdBuilder := testsample.EtcdBuilderWithDefaults(testEtcdName, testNs)
	testCases := []struct {
		name                    string
		etcdReplicas            int32
		createErr               *apierrors.StatusError
		expectedPDBMinAvailable int32
		expectedErr             *druiderr.DruidError
	}{
		{
			name:                    "create PDB for single node etcd cluster when none exists",
			etcdReplicas:            1,
			expectedPDBMinAvailable: 0,
		},
		{
			name:                    "create PDB for multi node etcd cluster when none exists",
			etcdReplicas:            3,
			expectedPDBMinAvailable: 2,
		},
		{
			name:      "returns error when client create fails",
			createErr: apiInternalErr,
			expectedErr: &druiderr.DruidError{
				Code:      ErrSyncPodDisruptionBudget,
				Cause:     apiInternalErr,
				Operation: "Sync",
			},
		},
	}

	g := NewWithT(t)
	t.Parallel()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			etcd := etcdBuilder.WithReplicas(tc.etcdReplicas).Build()
			cl := testutils.NewFakeClientBuilder().WithCreateError(tc.createErr).Build()
			operator := New(cl)
			opCtx := resource.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
			syncErr := operator.Sync(opCtx, etcd)
			latestPDB, getErr := getLatestPodDisruptionBudget(cl, etcd)
			if tc.expectedErr != nil {
				testutils.CheckDruidError(g, tc.expectedErr, syncErr)
				g.Expect(apierrors.IsNotFound(getErr)).To(BeTrue())
			} else {
				g.Expect(syncErr).ToNot(HaveOccurred())
				g.Expect(getErr).ToNot(HaveOccurred())
				checkPodDisruptionBudget(g, etcd, latestPDB, tc.expectedPDBMinAvailable)
			}
		})
	}
}

func TestSyncWhenPDBExists(t *testing.T) {
	etcdBuilder := testsample.EtcdBuilderWithDefaults(testEtcdName, testNs)
	testCases := []struct {
		name                    string
		originalEtcdReplicas    int32
		updatedEtcdReplicas     int32
		expectedPDBMinAvailable int32
		patchErr                *apierrors.StatusError
		expectedErr             *druiderr.DruidError
	}{
		{
			name:                    "successfully update PDB when etcd cluster replicas changed from 1 -> 3",
			originalEtcdReplicas:    1,
			updatedEtcdReplicas:     3,
			expectedPDBMinAvailable: 2,
		},
		{
			name:                    "returns error when client patch fails",
			originalEtcdReplicas:    1,
			updatedEtcdReplicas:     3,
			expectedPDBMinAvailable: 0,
			patchErr:                apiInternalErr,
			expectedErr: &druiderr.DruidError{
				Code:      ErrSyncPodDisruptionBudget,
				Cause:     apiInternalErr,
				Operation: "Sync",
			},
		},
	}

	g := NewWithT(t)
	t.Parallel()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			existingEtcd := etcdBuilder.WithReplicas(tc.originalEtcdReplicas).Build()
			cl := testutils.NewFakeClientBuilder().
				WithPatchError(tc.patchErr).
				WithObjects(testsample.NewPodDisruptionBudget(existingEtcd)).
				Build()
			operator := New(cl)
			opCtx := resource.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
			updatedEtcd := etcdBuilder.WithReplicas(tc.updatedEtcdReplicas).Build()
			syncErr := operator.Sync(opCtx, updatedEtcd)
			latestPDB, getErr := getLatestPodDisruptionBudget(cl, updatedEtcd)
			g.Expect(getErr).To(BeNil())
			if tc.expectedErr != nil {
				testutils.CheckDruidError(g, tc.expectedErr, syncErr)
			} else {
				g.Expect(syncErr).To(BeNil())
			}
			g.Expect(latestPDB.Spec.MinAvailable.IntVal).To(Equal(tc.expectedPDBMinAvailable))
		})
	}
}

// ----------------------------- TriggerDelete -------------------------------
func TestTriggerDelete(t *testing.T) {
	etcd := testsample.EtcdBuilderWithDefaults(testEtcdName, testNs).Build()
	testCases := []struct {
		name        string
		pdbExists   bool
		deleteErr   *apierrors.StatusError
		expectedErr *druiderr.DruidError
	}{
		{
			name:      "no-op and no error if the pdb is not found",
			pdbExists: false,
		},
		{
			name:      "successfully deletes existing pdb",
			pdbExists: true,
		},
		{
			name:      "returns error when client delete fails",
			pdbExists: true,
			deleteErr: apiInternalErr,
			expectedErr: &druiderr.DruidError{
				Code:      ErrDeletePodDisruptionBudget,
				Cause:     apiInternalErr,
				Operation: "TriggerDelete",
			},
		},
	}

	g := NewWithT(t)
	t.Parallel()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fakeClientBuilder := testutils.NewFakeClientBuilder().WithDeleteError(tc.deleteErr)
			if tc.pdbExists {
				fakeClientBuilder.WithObjects(testsample.NewPodDisruptionBudget(etcd))
			}
			cl := fakeClientBuilder.Build()
			operator := New(cl)
			opCtx := resource.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
			syncErr := operator.TriggerDelete(opCtx, etcd)
			_, getErr := getLatestPodDisruptionBudget(cl, etcd)
			if tc.expectedErr != nil {
				testutils.CheckDruidError(g, tc.expectedErr, syncErr)
				g.Expect(getErr).To(BeNil())
			} else {
				g.Expect(syncErr).NotTo(HaveOccurred())
				g.Expect(apierrors.IsNotFound(getErr)).To(BeTrue())
			}
		})
	}
}

// ---------------------------- Helper Functions -----------------------------

func checkPodDisruptionBudget(g *WithT, etcd *druidv1alpha1.Etcd, actualPDB *policyv1.PodDisruptionBudget, expectedPDBMinAvailable int32) {
	g.Expect(actualPDB.Name).To(Equal(etcd.Name))
	g.Expect(actualPDB.Namespace).To(Equal(etcd.Namespace))
	g.Expect(actualPDB.Labels).To(Equal(etcd.GetDefaultLabels()))
	expectedAnnotations := map[string]string{
		common.GardenerOwnedBy:   fmt.Sprintf("%s/%s", etcd.Namespace, etcd.Name),
		common.GardenerOwnerType: "etcd",
	}
	g.Expect(actualPDB.Annotations).To(Equal(expectedAnnotations))
	g.Expect(actualPDB.OwnerReferences).To(Equal([]metav1.OwnerReference{etcd.GetAsOwnerReference()}))
	g.Expect(*actualPDB.Spec.Selector).To(Equal(metav1.LabelSelector{MatchLabels: etcd.GetDefaultLabels()}))
	g.Expect(actualPDB.Spec.MinAvailable.IntVal).To(Equal(expectedPDBMinAvailable))
}

func getLatestPodDisruptionBudget(cl client.Client, etcd *druidv1alpha1.Etcd) (*policyv1.PodDisruptionBudget, error) {
	pdb := &policyv1.PodDisruptionBudget{}
	err := cl.Get(context.Background(), client.ObjectKey{Name: etcd.Name, Namespace: etcd.Namespace}, pdb)
	return pdb, err
}
