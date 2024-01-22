package rolebinding

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
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

var (
	internalErr = errors.New("test internal error")
)

// ------------------------ GetExistingResourceNames ------------------------
func TestGetExistingResourceNames(t *testing.T) {
	etcd := testutils.EtcdBuilderWithDefaults(testutils.TestEtcdName, testutils.TestNamespace).Build()
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
				fakeClientBuilder.WithObjects(newRoleBinding(etcd))
			}
			operator := New(fakeClientBuilder.Build())
			opCtx := resource.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
			roleBindingNames, err := operator.GetExistingResourceNames(opCtx, etcd)
			if tc.expectedErr != nil {
				testutils.CheckDruidError(g, tc.expectedErr, err)
			} else {
				g.Expect(err).To(BeNil())
				g.Expect(roleBindingNames).To(Equal(tc.expectedRoleBindingNames))
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
			cl := testutils.NewFakeClientBuilder().WithCreateError(tc.createErr).Build()
			operator := New(cl)
			opCtx := resource.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
			syncErr := operator.Sync(opCtx, etcd)
			latestRoleBinding, getErr := getLatestRoleBinding(cl, etcd)
			if tc.expectedErr != nil {
				testutils.CheckDruidError(g, tc.expectedErr, syncErr)
				g.Expect(apierrors.IsNotFound(getErr)).To(BeTrue())
			} else {
				g.Expect(syncErr).ToNot(HaveOccurred())
				g.Expect(getErr).To(BeNil())
				g.Expect(latestRoleBinding).ToNot(BeNil())
				matchRoleBinding(g, etcd, *latestRoleBinding)
			}
		})
	}
}

// ----------------------------- TriggerDelete -------------------------------
func TestTriggerDelete(t *testing.T) {
	etcd := testutils.EtcdBuilderWithDefaults(testutils.TestEtcdName, testutils.TestNamespace).Build()
	internalStatusErr := apierrors.NewInternalError(internalErr)
	testCases := []struct {
		name              string
		roleBindingExists bool
		deleteErr         *apierrors.StatusError
		expectedErr       *druiderr.DruidError
	}{
		{
			name:              "successfully delete existing role",
			roleBindingExists: true,
		},
		{
			name:              "delete fails due to failing client delete",
			roleBindingExists: true,
			deleteErr:         internalStatusErr,
			expectedErr: &druiderr.DruidError{
				Code:      ErrDeleteRoleBinding,
				Cause:     internalStatusErr,
				Operation: "TriggerDelete",
			},
		},
		{
			name:              "delete is a no-op if role does not exist",
			roleBindingExists: false,
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
			if tc.roleBindingExists {
				fakeClientBuilder.WithObjects(newRoleBinding(etcd))
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

func newRoleBinding(etcd *druidv1alpha1.Etcd) *rbacv1.RoleBinding {
	rb := emptyRoleBinding(etcd)
	buildResource(etcd, rb)
	return rb
}

func getLatestRoleBinding(cl client.Client, etcd *druidv1alpha1.Etcd) (*rbacv1.RoleBinding, error) {
	rb := &rbacv1.RoleBinding{}
	err := cl.Get(context.Background(), client.ObjectKey{Name: etcd.GetRoleBindingName(), Namespace: etcd.Namespace}, rb)
	return rb, err
}

func matchRoleBinding(g *WithT, etcd *druidv1alpha1.Etcd, actualRoleBinding rbacv1.RoleBinding) {
	g.Expect(actualRoleBinding).To(MatchFields(IgnoreExtras, Fields{
		"ObjectMeta": MatchFields(IgnoreExtras, Fields{
			"Name":            Equal(etcd.GetRoleBindingName()),
			"Namespace":       Equal(etcd.Namespace),
			"Labels":          testutils.MatchResourceLabels(etcd.GetDefaultLabels()),
			"OwnerReferences": testutils.MatchEtcdOwnerReference(etcd.Name, etcd.UID),
		}),
		"RoleRef": Equal(rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     etcd.GetRoleName(),
		}),
		"Subjects": ConsistOf(
			rbacv1.Subject{
				Kind:      "ServiceAccount",
				Name:      etcd.GetServiceAccountName(),
				Namespace: etcd.Namespace,
			},
		),
	}))
}
