// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package poddistruptionbudget

import (
	"context"
	"testing"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/component"
	druiderr "github.com/gardener/etcd-druid/internal/errors"
	testutils "github.com/gardener/etcd-druid/test/utils"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

// ------------------------ GetExistingResourceNames ------------------------
func TestGetExistingResourceNames(t *testing.T) {
	etcd := testutils.EtcdBuilderWithDefaults(testutils.TestEtcdName, testutils.TestNamespace).Build()
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
			getErr:           apierrors.NewNotFound(corev1.Resource("poddisruptionbudgets"), druidv1alpha1.GetPodDisruptionBudgetName(etcd.ObjectMeta)),
			expectedPDBNames: []string{},
		},
		{
			name:      "should return error when get fails",
			pdbExists: true,
			getErr:    testutils.TestAPIInternalErr,
			expectedErr: &druiderr.DruidError{
				Code:      ErrGetPodDisruptionBudget,
				Cause:     testutils.TestAPIInternalErr,
				Operation: "GetExistingResourceNames",
			},
		},
	}

	g := NewWithT(t)
	t.Parallel()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var existingObjects []client.Object
			if tc.pdbExists {
				existingObjects = append(existingObjects, newPodDisruptionBudget(etcd))
			}
			cl := testutils.CreateTestFakeClientForObjects(tc.getErr, nil, nil, nil, existingObjects, getObjectKey(etcd.ObjectMeta))
			operator := New(cl)
			opCtx := component.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
			pdbNames, err := operator.GetExistingResourceNames(opCtx, etcd.ObjectMeta)
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
			createErr: testutils.TestAPIInternalErr,
			expectedErr: &druiderr.DruidError{
				Code:      ErrSyncPodDisruptionBudget,
				Cause:     testutils.TestAPIInternalErr,
				Operation: "Sync",
			},
		},
	}

	g := NewWithT(t)
	t.Parallel()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			etcdBuilder := testutils.EtcdBuilderWithDefaults(testutils.TestEtcdName, testutils.TestNamespace)
			etcd := etcdBuilder.WithReplicas(tc.etcdReplicas).Build()
			cl := testutils.CreateTestFakeClientForObjects(nil, tc.createErr, nil, nil, nil, getObjectKey(etcd.ObjectMeta))
			operator := New(cl)
			opCtx := component.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
			syncErr := operator.Sync(opCtx, etcd)
			latestPDB, getErr := getLatestPodDisruptionBudget(cl, etcd)
			if tc.expectedErr != nil {
				testutils.CheckDruidError(g, tc.expectedErr, syncErr)
				g.Expect(apierrors.IsNotFound(getErr)).To(BeTrue())
			} else {
				g.Expect(syncErr).ToNot(HaveOccurred())
				g.Expect(getErr).ToNot(HaveOccurred())
				g.Expect(latestPDB).ToNot(BeNil())
				matchPodDisruptionBudget(g, etcd, *latestPDB, tc.expectedPDBMinAvailable)
			}
		})
	}
}

func TestSyncWhenPDBExists(t *testing.T) {
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
			patchErr:                testutils.TestAPIInternalErr,
			expectedErr: &druiderr.DruidError{
				Code:      ErrSyncPodDisruptionBudget,
				Cause:     testutils.TestAPIInternalErr,
				Operation: "Sync",
			},
		},
	}

	g := NewWithT(t)
	t.Parallel()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			etcdBuilder := testutils.EtcdBuilderWithDefaults(testutils.TestEtcdName, testutils.TestNamespace)
			existingEtcd := etcdBuilder.WithReplicas(tc.originalEtcdReplicas).Build()
			cl := testutils.CreateTestFakeClientForObjects(nil, nil, tc.patchErr, nil, []client.Object{newPodDisruptionBudget(existingEtcd)}, getObjectKey(existingEtcd.ObjectMeta))
			operator := New(cl)
			opCtx := component.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
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
	etcd := testutils.EtcdBuilderWithDefaults(testutils.TestEtcdName, testutils.TestNamespace).Build()
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
			deleteErr: testutils.TestAPIInternalErr,
			expectedErr: &druiderr.DruidError{
				Code:      ErrDeletePodDisruptionBudget,
				Cause:     testutils.TestAPIInternalErr,
				Operation: "TriggerDelete",
			},
		},
	}

	g := NewWithT(t)
	t.Parallel()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var existingObjects []client.Object
			if tc.pdbExists {
				existingObjects = append(existingObjects, newPodDisruptionBudget(etcd))
			}
			cl := testutils.CreateTestFakeClientForObjects(nil, nil, nil, tc.deleteErr, existingObjects, getObjectKey(etcd.ObjectMeta))
			operator := New(cl)
			opCtx := component.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
			syncErr := operator.TriggerDelete(opCtx, etcd.ObjectMeta)
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

func newPodDisruptionBudget(etcd *druidv1alpha1.Etcd) *policyv1.PodDisruptionBudget {
	pdb := emptyPodDisruptionBudget(getObjectKey(etcd.ObjectMeta))
	buildResource(etcd, pdb)
	return pdb
}

func getLatestPodDisruptionBudget(cl client.Client, etcd *druidv1alpha1.Etcd) (*policyv1.PodDisruptionBudget, error) {
	pdb := &policyv1.PodDisruptionBudget{}
	err := cl.Get(context.Background(), client.ObjectKey{Name: etcd.Name, Namespace: etcd.Namespace}, pdb)
	return pdb, err
}

func matchPodDisruptionBudget(g *WithT, etcd *druidv1alpha1.Etcd, actualPDB policyv1.PodDisruptionBudget, expectedPDBMinAvailable int32) {
	g.Expect(actualPDB).To(MatchFields(IgnoreExtras, Fields{
		"ObjectMeta": MatchFields(IgnoreExtras, Fields{
			"Name":            Equal(druidv1alpha1.GetPodDisruptionBudgetName(etcd.ObjectMeta)),
			"Namespace":       Equal(etcd.Namespace),
			"Labels":          testutils.MatchResourceLabels(druidv1alpha1.GetDefaultLabels(etcd.ObjectMeta)),
			"OwnerReferences": testutils.MatchEtcdOwnerReference(etcd.Name, etcd.UID),
		}),
		"Spec": MatchFields(IgnoreExtras, Fields{
			"Selector": testutils.MatchSpecLabelSelector(druidv1alpha1.GetDefaultLabels(etcd.ObjectMeta)),
			"MinAvailable": PointTo(MatchFields(IgnoreExtras, Fields{
				"Type":   Equal(intstr.Int),
				"IntVal": Equal(expectedPDBMinAvailable),
			})),
		}),
	}))

}
