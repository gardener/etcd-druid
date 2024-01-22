package role

import (
	"context"
	"errors"
	"testing"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	druiderr "github.com/gardener/etcd-druid/internal/errors"
	"github.com/gardener/etcd-druid/internal/operator/resource"
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

var (
	internalErr = errors.New("test internal error")
)

// ------------------------ GetExistingResourceNames ------------------------
func TestGetExistingResourceNames(t *testing.T) {
	etcd := testutils.EtcdBuilderWithDefaults(testutils.TestEtcdName, testutils.TestNamespace).Build()
	getInternalErr := apierrors.NewInternalError(internalErr)
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
			expectedRoleNames: []string{etcd.GetRoleName()},
		},
		{
			name:              "should return empty slice when role is not found",
			roleExists:        false,
			getErr:            apierrors.NewNotFound(corev1.Resource("roles"), etcd.GetRoleName()),
			expectedRoleNames: []string{},
		},
		{
			name:       "should return error when get fails",
			roleExists: true,
			getErr:     getInternalErr,
			expectedErr: &druiderr.DruidError{
				Code:      ErrGetRole,
				Cause:     getInternalErr,
				Operation: "GetExistingResourceNames",
			},
		},
	}

	g := NewWithT(t)
	t.Parallel()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fakeClientBuilder := testutils.NewFakeClientBuilder()
			if tc.getErr != nil {
				fakeClientBuilder.WithGetError(tc.getErr)
			}
			if tc.roleExists {
				fakeClientBuilder.WithObjects(newRole(etcd))
			}
			operator := New(fakeClientBuilder.Build())
			opCtx := resource.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
			roleNames, err := operator.GetExistingResourceNames(opCtx, etcd)
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
	internalStatusErr := apierrors.NewInternalError(internalErr)
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
			createErr: internalStatusErr,
			expectedErr: &druiderr.DruidError{
				Code:      ErrSyncRole,
				Cause:     internalStatusErr,
				Operation: "Sync",
			},
		},
	}

	g := NewWithT(t)
	t.Parallel()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cl := testutils.NewFakeClientBuilder().
				WithCreateError(tc.createErr).
				Build()
			operator := New(cl)
			opCtx := resource.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
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
	internalStatusErr := apierrors.NewInternalError(internalErr)
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
			deleteErr:  internalStatusErr,
			expectedErr: &druiderr.DruidError{
				Code:      ErrDeleteRole,
				Cause:     internalStatusErr,
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
			fakeClientBuilder := testutils.NewFakeClientBuilder()
			if tc.deleteErr != nil {
				fakeClientBuilder.WithDeleteError(tc.deleteErr)
			}
			if tc.roleExists {
				fakeClientBuilder.WithObjects(newRole(etcd))
			}
			cl := fakeClientBuilder.Build()
			operator := New(cl)
			opCtx := resource.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
			deleteErr := operator.TriggerDelete(opCtx, etcd)
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
	role := emptyRole(etcd)
	buildResource(etcd, role)
	return role
}

func getLatestRole(cl client.Client, etcd *druidv1alpha1.Etcd) (*rbacv1.Role, error) {
	role := &rbacv1.Role{}
	err := cl.Get(context.Background(), client.ObjectKey{Name: etcd.GetRoleName(), Namespace: etcd.Namespace}, role)
	return role, err
}

func matchRole(g *WithT, etcd *druidv1alpha1.Etcd, actualRole rbacv1.Role) {
	g.Expect(actualRole).To(MatchFields(IgnoreExtras, Fields{
		"ObjectMeta": MatchFields(IgnoreExtras, Fields{
			"Name":            Equal(etcd.GetRoleName()),
			"Namespace":       Equal(etcd.Namespace),
			"Labels":          testutils.MatchResourceLabels(etcd.GetDefaultLabels()),
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
