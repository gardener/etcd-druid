package role

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
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	testEtcdName = "test-etcd"
	testNs       = "test-namespace"
)

var (
	internalErr = errors.New("test internal error")
)

// ------------------------ GetExistingResourceNames ------------------------
func TestGetExistingResourceNames(t *testing.T) {
	etcd := testsample.EtcdBuilderWithDefaults(testEtcdName, testNs).Build()
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
				fakeClientBuilder.WithObjects(testsample.NewRole(etcd))
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
	etcd := testsample.EtcdBuilderWithDefaults(testEtcdName, testNs).Build()
	internalStatusErr := apierrors.NewInternalError(internalErr)
	testCases := []struct {
		name        string
		roleExists  bool
		getErr      *apierrors.StatusError
		createErr   *apierrors.StatusError
		expectedErr *druiderr.DruidError
	}{
		{
			name:       "create role when none exists",
			roleExists: false,
		},
		{
			name:       "create role fails when client create fails",
			roleExists: false,
			createErr:  internalStatusErr,
			expectedErr: &druiderr.DruidError{
				Code:      ErrSyncRole,
				Cause:     internalStatusErr,
				Operation: "Sync",
			},
		},
		{
			name:       "create role fails when get errors out",
			roleExists: false,
			getErr:     internalStatusErr,
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
			fakeClientBuilder := testutils.NewFakeClientBuilder().
				WithGetError(tc.getErr).
				WithCreateError(tc.createErr)
			if tc.roleExists {
				fakeClientBuilder.WithObjects(testsample.NewRole(etcd))
			}
			cl := fakeClientBuilder.Build()
			operator := New(cl)
			opCtx := resource.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
			err := operator.Sync(opCtx, etcd)
			if tc.expectedErr != nil {
				testutils.CheckDruidError(g, tc.expectedErr, err)
			} else {
				g.Expect(err).ToNot(HaveOccurred())
				existingRole := &rbacv1.Role{
					ObjectMeta: metav1.ObjectMeta{
						Name:      etcd.GetRoleName(),
						Namespace: etcd.Namespace,
					},
				}
				err = cl.Get(context.Background(), client.ObjectKeyFromObject(existingRole), existingRole)
				g.Expect(err).ToNot(HaveOccurred())
				checkRole(g, existingRole, etcd)
			}
		})
	}
}

// ----------------------------- TriggerDelete -------------------------------
func TestTriggerDelete(t *testing.T) {
	etcd := testsample.EtcdBuilderWithDefaults(testEtcdName, testNs).Build()
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
				fakeClientBuilder.WithObjects(testsample.NewRole(etcd))
			}
			cl := fakeClientBuilder.Build()
			operator := New(cl)
			opCtx := resource.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
			err := operator.TriggerDelete(opCtx, etcd)
			if tc.expectedErr != nil {
				testutils.CheckDruidError(g, tc.expectedErr, err)
			} else {
				g.Expect(err).NotTo(HaveOccurred())
				existingRole := rbacv1.Role{}
				err = cl.Get(context.Background(), client.ObjectKey{Name: etcd.GetRoleName(), Namespace: etcd.Namespace}, &existingRole)
				g.Expect(err).To(HaveOccurred())
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}
		})
	}
}

// ---------------------------- Helper Functions -----------------------------
func checkRole(g *WithT, role *rbacv1.Role, etcd *druidv1alpha1.Etcd) {
	g.Expect(role.OwnerReferences).To(Equal([]metav1.OwnerReference{etcd.GetAsOwnerReference()}))
	g.Expect(role.Labels).To(Equal(etcd.GetDefaultLabels()))
	g.Expect(role.Rules).To(ConsistOf(
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
	))
}
