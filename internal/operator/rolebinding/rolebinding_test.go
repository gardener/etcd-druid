package rolebinding

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
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
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
	internalErr = errors.New("test internal error")
)

// ------------------------ GetExistingResourceNames ------------------------
func TestGetExistingResourceNames(t *testing.T) {
	etcd := testsample.EtcdBuilderWithDefaults(testEtcdName, testNs).Build()
	getErr := apierrors.NewInternalError(internalErr)
	testCases := []struct {
		name                     string
		roleBindingExists        bool
		getErr                   *apierrors.StatusError
		expectedErr              *druiderr.DruidError
		expectedRoleBindingNames []string
	}{
		{
			name:                     "should return the existing role binding name",
			roleBindingExists:        true,
			expectedRoleBindingNames: []string{etcd.GetRoleBindingName()},
		},
		{
			name:                     "should return empty slice when role binding is not found",
			roleBindingExists:        false,
			getErr:                   apierrors.NewNotFound(corev1.Resource("roles"), etcd.GetRoleBindingName()),
			expectedRoleBindingNames: []string{},
		},
		{
			name:              "should return error when get fails",
			roleBindingExists: true,
			getErr:            getErr,
			expectedErr: &druiderr.DruidError{
				Code:      ErrGetRoleBinding,
				Cause:     getErr,
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
			if tc.roleBindingExists {
				fakeClientBuilder.WithObjects(testsample.NewRoleBinding(etcd))
			}
			operator := New(fakeClientBuilder.Build())
			opCtx := resource.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
			roleBindingNames, err := operator.GetExistingResourceNames(opCtx, etcd)
			if tc.expectedErr != nil {
				testutils.CheckDruidError(g, tc.expectedErr, err)
			} else {
				g.Expect(err).To(BeNil())
			}
			g.Expect(roleBindingNames, tc.expectedRoleBindingNames)
		})
	}
}

// ----------------------------------- Sync -----------------------------------
func TestSync(t *testing.T) {
	etcd := testsample.EtcdBuilderWithDefaults(testEtcdName, testNs).Build()
	internalStatusErr := apierrors.NewInternalError(internalErr)
	testCases := []struct {
		name              string
		roleBindingExists bool
		getErr            *apierrors.StatusError
		createErr         *apierrors.StatusError
		expectedErr       *druiderr.DruidError
	}{
		{
			name:              "create role when none exists",
			roleBindingExists: false,
		},
		{
			name:              "create role fails when client create fails",
			roleBindingExists: false,
			createErr:         internalStatusErr,
			expectedErr: &druiderr.DruidError{
				Code:      ErrSyncRoleBinding,
				Cause:     internalStatusErr,
				Operation: "Sync",
			},
		},
		{
			name:              "create role fails when get errors out",
			roleBindingExists: false,
			getErr:            internalStatusErr,
			expectedErr: &druiderr.DruidError{
				Code:      ErrSyncRoleBinding,
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
			if tc.roleBindingExists {
				fakeClientBuilder.WithObjects(testsample.NewRoleBinding(etcd))
			}
			cl := fakeClientBuilder.Build()
			operator := New(cl)
			opCtx := resource.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
			err := operator.Sync(opCtx, etcd)
			if tc.expectedErr != nil {
				testutils.CheckDruidError(g, tc.expectedErr, err)
			} else {
				g.Expect(err).ToNot(HaveOccurred())
				existingRoleBinding := &rbacv1.RoleBinding{
					ObjectMeta: metav1.ObjectMeta{
						Name:      etcd.GetRoleBindingName(),
						Namespace: etcd.Namespace,
					},
				}
				err = cl.Get(context.Background(), client.ObjectKeyFromObject(existingRoleBinding), existingRoleBinding)
				g.Expect(err).ToNot(HaveOccurred())
				checkRoleBinding(g, existingRoleBinding, etcd)
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
				Code:      ErrDeleteRoleBinding,
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
				fakeClientBuilder.WithObjects(testsample.NewRoleBinding(etcd))
			}
			cl := fakeClientBuilder.Build()
			operator := New(cl)
			opCtx := resource.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
			err := operator.TriggerDelete(opCtx, etcd)
			if tc.expectedErr != nil {
				testutils.CheckDruidError(g, tc.expectedErr, err)
			} else {
				g.Expect(err).NotTo(HaveOccurred())
				existingRoleBinding := rbacv1.RoleBinding{}
				err = cl.Get(context.Background(), client.ObjectKey{Name: etcd.GetRoleBindingName(), Namespace: etcd.Namespace}, &existingRoleBinding)
				g.Expect(err).To(HaveOccurred())
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}
		})
	}
}

// ---------------------------- Helper Functions -----------------------------
func checkRoleBinding(g *WithT, roleBinding *rbacv1.RoleBinding, etcd *druidv1alpha1.Etcd) {
	g.Expect(roleBinding.OwnerReferences).To(Equal([]metav1.OwnerReference{etcd.GetAsOwnerReference()}))
	g.Expect(roleBinding.Labels).To(Equal(etcd.GetDefaultLabels()))
	g.Expect(roleBinding.RoleRef).To(Equal(rbacv1.RoleRef{
		APIGroup: "rbac.authorization.k8s.io",
		Kind:     "Role",
		Name:     etcd.GetRoleName(),
	}))
	g.Expect(roleBinding.Subjects).To(ConsistOf(
		rbacv1.Subject{
			Kind:      "ServiceAccount",
			Name:      etcd.GetServiceAccountName(),
			Namespace: etcd.Namespace,
		},
	))
}
