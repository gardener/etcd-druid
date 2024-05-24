// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package role

import (
	"context"
	"testing"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/component"
	druiderr "github.com/gardener/etcd-druid/internal/errors"
	testutils "github.com/gardener/etcd-druid/test/utils"
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ------------------------ GetExistingResourceNames ------------------------
func TestGetExistingResourceNames(t *testing.T) {
	etcd := testutils.EtcdBuilderWithDefaults(testutils.TestEtcdName, testutils.TestNamespace).Build()
	testCases := []struct {
		name              string
		roleExists        bool
		getErr            *apierrors.StatusError
		expectedErr       *druiderr.DruidError
		expectedRoleNames []string
	}{
		{
			name:              "should return the existing role name",
			roleExists:        true,
			expectedRoleNames: []string{druidv1alpha1.GetRoleName(etcd.ObjectMeta)},
		},
		{
			name:              "should return empty slice when role is not found",
			roleExists:        false,
			getErr:            apierrors.NewNotFound(corev1.Resource("roles"), druidv1alpha1.GetRoleName(etcd.ObjectMeta)),
			expectedRoleNames: []string{},
		},
		{
			name:       "should return error when get fails",
			roleExists: true,
			getErr:     testutils.TestAPIInternalErr,
			expectedErr: &druiderr.DruidError{
				Code:      ErrGetRole,
				Cause:     testutils.TestAPIInternalErr,
				Operation: "GetExistingResourceNames",
			},
		},
	}

	g := NewWithT(t)
	t.Parallel()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var existingObjects []client.Object
			if tc.roleExists {
				existingObjects = append(existingObjects, newRole(etcd))
			}
			cl := testutils.CreateTestFakeClientForObjects(tc.getErr, nil, nil, nil, existingObjects, getObjectKey(etcd.ObjectMeta))
			operator := New(cl)
			opCtx := component.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
			roleNames, err := operator.GetExistingResourceNames(opCtx, etcd.ObjectMeta)
			if tc.expectedErr != nil {
				testutils.CheckDruidError(g, tc.expectedErr, err)
			} else {
				g.Expect(err).To(BeNil())
				g.Expect(roleNames).To(Equal(tc.expectedRoleNames))
			}
		})
	}
}

// ----------------------------------- Sync -----------------------------------
func TestSync(t *testing.T) {
	etcd := testutils.EtcdBuilderWithDefaults(testutils.TestEtcdName, testutils.TestNamespace).Build()
	testCases := []struct {
		name        string
		createErr   *apierrors.StatusError
		expectedErr *druiderr.DruidError
	}{
		{
			name: "create role when none exists",
		},
		{
			name:      "create role fails when client create fails",
			createErr: testutils.TestAPIInternalErr,
			expectedErr: &druiderr.DruidError{
				Code:      ErrSyncRole,
				Cause:     testutils.TestAPIInternalErr,
				Operation: "Sync",
			},
		},
	}

	g := NewWithT(t)
	t.Parallel()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cl := testutils.CreateTestFakeClientForObjects(nil, tc.createErr, nil, nil, nil, getObjectKey(etcd.ObjectMeta))
			operator := New(cl)
			opCtx := component.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
			syncErr := operator.Sync(opCtx, etcd)
			latestRole, getErr := getLatestRole(cl, etcd)
			if tc.expectedErr != nil {
				testutils.CheckDruidError(g, tc.expectedErr, syncErr)
				g.Expect(apierrors.IsNotFound(getErr)).To(BeTrue())
			} else {
				g.Expect(syncErr).ToNot(HaveOccurred())
				g.Expect(getErr).ToNot(HaveOccurred())
				g.Expect(latestRole).ToNot(BeNil())
				matchRole(g, etcd, *latestRole)
			}
		})
	}
}

// ----------------------------- TriggerDelete -------------------------------
func TestTriggerDelete(t *testing.T) {
	etcd := testutils.EtcdBuilderWithDefaults(testutils.TestEtcdName, testutils.TestNamespace).Build()
	testCases := []struct {
		name        string
		roleExists  bool
		deleteErr   *apierrors.StatusError
		expectedErr *druiderr.DruidError
	}{
		{
			name:       "successfully delete existing role",
			roleExists: true,
		},
		{
			name:       "delete fails due to failing client delete",
			roleExists: true,
			deleteErr:  testutils.TestAPIInternalErr,
			expectedErr: &druiderr.DruidError{
				Code:      ErrDeleteRole,
				Cause:     testutils.TestAPIInternalErr,
				Operation: "TriggerDelete",
			},
		},
		{
			name:       "delete is a no-op if role does not exist",
			roleExists: false,
		},
	}

	g := NewWithT(t)
	t.Parallel()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var existingObjects []client.Object
			if tc.roleExists {
				existingObjects = append(existingObjects, newRole(etcd))
			}
			cl := testutils.CreateTestFakeClientForObjects(nil, nil, nil, tc.deleteErr, existingObjects, getObjectKey(etcd.ObjectMeta))
			operator := New(cl)
			opCtx := component.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
			deleteErr := operator.TriggerDelete(opCtx, etcd.ObjectMeta)
			latestRole, getErr := getLatestRole(cl, etcd)
			if tc.expectedErr != nil {
				testutils.CheckDruidError(g, tc.expectedErr, deleteErr)
				g.Expect(getErr).ToNot(HaveOccurred())
				g.Expect(latestRole).ToNot(BeNil())
			} else {
				g.Expect(deleteErr).NotTo(HaveOccurred())
				g.Expect(apierrors.IsNotFound(getErr)).To(BeTrue())
			}
		})
	}
}

// ---------------------------- Helper Functions -----------------------------

func newRole(etcd *druidv1alpha1.Etcd) *rbacv1.Role {
	role := emptyRole(getObjectKey(etcd.ObjectMeta))
	buildResource(etcd, role)
	return role
}

func getLatestRole(cl client.Client, etcd *druidv1alpha1.Etcd) (*rbacv1.Role, error) {
	role := &rbacv1.Role{}
	err := cl.Get(context.Background(), client.ObjectKey{Name: druidv1alpha1.GetRoleName(etcd.ObjectMeta), Namespace: etcd.Namespace}, role)
	return role, err
}

func matchRole(g *WithT, etcd *druidv1alpha1.Etcd, actualRole rbacv1.Role) {
	g.Expect(actualRole).To(MatchFields(IgnoreExtras, Fields{
		"ObjectMeta": MatchFields(IgnoreExtras, Fields{
			"Name":            Equal(druidv1alpha1.GetRoleName(etcd.ObjectMeta)),
			"Namespace":       Equal(etcd.Namespace),
			"Labels":          testutils.MatchResourceLabels(druidv1alpha1.GetDefaultLabels(etcd.ObjectMeta)),
			"OwnerReferences": testutils.MatchEtcdOwnerReference(etcd.Name, etcd.UID),
		}),
		"Rules": ConsistOf(
			rbacv1.PolicyRule{
				APIGroups: []string{"coordination.k8s.io"},
				Resources: []string{"leases"},
				Verbs:     []string{"get", "list", "patch", "update", "watch"},
			},
			rbacv1.PolicyRule{
				APIGroups: []string{"apps"},
				Resources: []string{"statefulsets"},
				Verbs:     []string{"get", "list", "patch", "update", "watch"},
			},
			rbacv1.PolicyRule{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     []string{"get", "list", "watch"},
			},
		),
	}))
}
