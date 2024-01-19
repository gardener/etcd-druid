package rolebinding

import (
	"fmt"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"
	druiderr "github.com/gardener/etcd-druid/internal/errors"
	"github.com/gardener/etcd-druid/internal/operator/resource"
	"github.com/gardener/etcd-druid/internal/utils"
	"github.com/gardener/gardener/pkg/controllerutils"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ErrGetRoleBinding    druidv1alpha1.ErrorCode = "ERR_GET_ROLE_BINDING"
	ErrSyncRoleBinding   druidv1alpha1.ErrorCode = "ERR_SYNC_ROLE_BINDING"
	ErrDeleteRoleBinding druidv1alpha1.ErrorCode = "ERR_DELETE_ROLE_BINDING"
)

type _resource struct {
	client client.Client
}

func (r _resource) GetExistingResourceNames(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd) ([]string, error) {
	resourceNames := make([]string, 0, 1)
	rbObjectKey := getObjectKey(etcd)
	rb := &rbacv1.RoleBinding{}
	if err := r.client.Get(ctx, rbObjectKey, rb); err != nil {
		if errors.IsNotFound(err) {
			return resourceNames, nil
		}
		return resourceNames, druiderr.WrapError(err,
			ErrGetRoleBinding,
			"GetExistingResourceNames",
			fmt.Sprintf("Error getting role-binding: %s for etcd: %v", rbObjectKey.Name, etcd.GetNamespaceName()))
	}
	resourceNames = append(resourceNames, rb.Name)
	return resourceNames, nil
}

func (r _resource) TriggerDelete(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	ctx.Logger.Info("Triggering delete of role")
	err := client.IgnoreNotFound(r.client.Delete(ctx, emptyRoleBinding(etcd)))
	if err == nil {
		ctx.Logger.Info("deleted", "resource", "role-binding", "name", etcd.GetRoleBindingName())
	}
	return druiderr.WrapError(err,
		ErrDeleteRoleBinding,
		"TriggerDelete",
		fmt.Sprintf("Failed to delete role-binding: %s for etcd: %v", etcd.GetRoleBindingName(), etcd.GetNamespaceName()),
	)
}

func (r _resource) Sync(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	rb := emptyRoleBinding(etcd)
	result, err := controllerutils.GetAndCreateOrStrategicMergePatch(ctx, r.client, rb, func() error {
		buildResource(etcd, rb)
		return nil
	})
	if err == nil {
		ctx.Logger.Info("synced", "resource", "role", "name", rb.Name, "result", result)
	}
	return druiderr.WrapError(err,
		ErrSyncRoleBinding,
		"Sync",
		fmt.Sprintf("Error during create or update of role-binding %s for etcd: %v", etcd.GetRoleBindingName(), etcd.GetNamespaceName()),
	)
}

func (r _resource) Exists(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd) (bool, error) {
	rb := &rbacv1.RoleBinding{}
	if err := r.client.Get(ctx, getObjectKey(etcd), rb); err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func New(client client.Client) resource.Operator {
	return &_resource{
		client: client,
	}
}

func getObjectKey(etcd *druidv1alpha1.Etcd) client.ObjectKey {
	return client.ObjectKey{Name: etcd.GetRoleBindingName(), Namespace: etcd.Namespace}
}

func emptyRoleBinding(etcd *druidv1alpha1.Etcd) *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      etcd.GetRoleBindingName(),
			Namespace: etcd.Namespace,
		},
	}
}

func buildResource(etcd *druidv1alpha1.Etcd, rb *rbacv1.RoleBinding) {
	rb.Labels = getLabels(etcd)
	rb.OwnerReferences = []metav1.OwnerReference{etcd.GetAsOwnerReference()}
	rb.RoleRef = rbacv1.RoleRef{
		APIGroup: "rbac.authorization.k8s.io",
		Kind:     "Role",
		Name:     etcd.GetRoleName(),
	}
	rb.Subjects = []rbacv1.Subject{
		{
			Kind:      "ServiceAccount",
			Name:      etcd.GetServiceAccountName(),
			Namespace: etcd.Namespace,
		},
	}
}

func getLabels(etcd *druidv1alpha1.Etcd) map[string]string {
	roleLabels := map[string]string{
		druidv1alpha1.LabelComponentKey: common.RoleBindingComponentName,
		druidv1alpha1.LabelAppNameKey:   etcd.GetRoleBindingName(),
	}
	return utils.MergeMaps[string, string](etcd.GetDefaultLabels(), roleLabels)
}
