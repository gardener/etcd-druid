// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package serviceaccount

import (
	"context"
	"testing"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	druiderr "github.com/gardener/etcd-druid/internal/errors"
	"github.com/gardener/etcd-druid/internal/operator/component"
	testutils "github.com/gardener/etcd-druid/test/utils"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

// ------------------------ GetExistingResourceNames ------------------------
func TestGetExistingResourceNames(t *testing.T) {
	etcd := testutils.EtcdBuilderWithDefaults(testutils.TestEtcdName, testutils.TestNamespace).Build()
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
			getErr:   testutils.TestAPIInternalErr,
			expectedErr: &druiderr.DruidError{
				Code:      ErrGetServiceAccount,
				Cause:     testutils.TestAPIInternalErr,
				Operation: "GetExistingResourceNames",
			},
		},
	}

	g := NewWithT(t)
	t.Parallel()
	for _, tc := range testCases {
		t.Run(tc.name, func(_ *testing.T) {
			var existingObjects []client.Object
			if tc.saExists {
				existingObjects = append(existingObjects, newServiceAccount(etcd, false))
			}
			cl := testutils.CreateTestFakeClientForObjects(tc.getErr, nil, nil, nil, existingObjects, getObjectKey(etcd))
			operator := New(cl, true)
			opCtx := component.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
			saNames, err := operator.GetExistingResourceNames(opCtx, etcd)
			if tc.expectedErr != nil {
				testutils.CheckDruidError(g, tc.expectedErr, err)
			} else {
				if tc.saExists {
					existingSA, getErr := getLatestServiceAccount(cl, etcd)
					g.Expect(getErr).ToNot(HaveOccurred())
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
			createErr: testutils.TestAPIInternalErr,
			expectedErr: &druiderr.DruidError{
				Code:      ErrSyncServiceAccount,
				Cause:     testutils.TestAPIInternalErr,
				Operation: "Sync",
			},
		},
	}

	g := NewWithT(t)
	t.Parallel()
	for _, tc := range testCases {
		t.Run(tc.name, func(_ *testing.T) {
			etcd := testutils.EtcdBuilderWithDefaults(testutils.TestEtcdName, testutils.TestNamespace).Build()
			cl := testutils.CreateTestFakeClientForObjects(nil, tc.createErr, nil, nil, nil, getObjectKey(etcd))
			operator := New(cl, tc.disableAutoMount)
			opCtx := component.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
			syncErr := operator.Sync(opCtx, etcd)
			latestSA, getErr := getLatestServiceAccount(cl, etcd)
			if tc.expectedErr != nil {
				testutils.CheckDruidError(g, tc.expectedErr, syncErr)
				g.Expect(apierrors.IsNotFound(getErr)).To(BeTrue())
			} else {
				g.Expect(getErr).To(BeNil())
				g.Expect(latestSA).ToNot(BeNil())
				matchServiceAccount(g, etcd, *latestSA, tc.disableAutoMount)
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
			deleteErr: testutils.TestAPIInternalErr,
			expectedErr: &druiderr.DruidError{
				Code:      ErrDeleteServiceAccount,
				Cause:     testutils.TestAPIInternalErr,
				Operation: "TriggerDelete",
			},
		},
	}

	g := NewWithT(t)
	t.Parallel()
	for _, tc := range testCases {
		t.Run(tc.name, func(_ *testing.T) {
			etcd := testutils.EtcdBuilderWithDefaults(testutils.TestEtcdName, testutils.TestNamespace).Build()
			var existingObjects []client.Object
			if tc.saExists {
				existingObjects = append(existingObjects, newServiceAccount(etcd, false))
			}
			cl := testutils.CreateTestFakeClientForObjects(nil, nil, nil, tc.deleteErr, existingObjects, getObjectKey(etcd))
			operator := New(cl, false)
			opCtx := component.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
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
func newServiceAccount(etcd *druidv1alpha1.Etcd, disableAutomount bool) *corev1.ServiceAccount {
	sa := emptyServiceAccount(getObjectKey(etcd))
	buildResource(etcd, sa, !disableAutomount)
	return sa
}

func getLatestServiceAccount(cl client.Client, etcd *druidv1alpha1.Etcd) (*corev1.ServiceAccount, error) {
	sa := &corev1.ServiceAccount{}
	err := cl.Get(context.Background(), client.ObjectKey{Name: etcd.GetServiceAccountName(), Namespace: etcd.Namespace}, sa)
	return sa, err
}

func matchServiceAccount(g *WithT, etcd *druidv1alpha1.Etcd, actualSA corev1.ServiceAccount, disableAutoMount bool) {
	g.Expect(actualSA).To(MatchFields(IgnoreExtras, Fields{
		"ObjectMeta": MatchFields(IgnoreExtras, Fields{
			"Name":            Equal(etcd.GetServiceAccountName()),
			"Namespace":       Equal(etcd.Namespace),
			"Labels":          testutils.MatchResourceLabels(etcd.GetDefaultLabels()),
			"OwnerReferences": testutils.MatchEtcdOwnerReference(etcd.Name, etcd.UID),
		}),
		"AutomountServiceAccountToken": PointTo(Equal(!disableAutoMount)),
	}))
}
