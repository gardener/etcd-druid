package role

import (
	"fmt"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
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
	ErrGetRole    druidv1alpha1.ErrorCode = "ERR_GET_ROLE"
	ErrSyncRole   druidv1alpha1.ErrorCode = "ERR_SYNC_ROLE"
	ErrDeleteRole druidv1alpha1.ErrorCode = "ERR_DELETE_ROLE"
)

const componentName = "druid-role"

type _resource struct {
	client client.Client
}

func (r _resource) GetExistingResourceNames(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd) ([]string, error) {
	resourceNames := make([]string, 0, 1)
	roleObjectKey := getObjectKey(etcd)
	role := &rbacv1.Role{}
	if err := r.client.Get(ctx, roleObjectKey, role); err != nil {
		if errors.IsNotFound(err) {
			return resourceNames, nil
		}
		return resourceNames, druiderr.WrapError(err,
			ErrGetRole,
			"GetExistingResourceNames",
			fmt.Sprintf("Error getting role: %s for etcd: %v", roleObjectKey.Name, etcd.GetNamespaceName()))
	}
	resourceNames = append(resourceNames, role.Name)
	return resourceNames, nil
}

func (r _resource) Sync(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	role := emptyRole(etcd)
	result, err := controllerutils.GetAndCreateOrStrategicMergePatch(ctx, r.client, role, func() error {
		buildResource(etcd, role)
		return nil
	})
	if err == nil {
		ctx.Logger.Info("synced", "resource", "role", "name", role.Name, "result", result)
	}
	return druiderr.WrapError(err,
		ErrSyncRole,
		"Sync",
		fmt.Sprintf("Error during create or update of role %s for etcd: %v", etcd.GetRoleName(), etcd.GetNamespaceName()),
	)
}

func (r _resource) TriggerDelete(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	ctx.Logger.Info("Triggering delete of role")
	err := client.IgnoreNotFound(r.client.Delete(ctx, emptyRole(etcd)))
	if err == nil {
		ctx.Logger.Info("deleted", "resource", "role", "name", etcd.GetRoleName())
	}
	return druiderr.WrapError(err,
		ErrDeleteRole,
		"TriggerDelete",
		fmt.Sprintf("Failed to delete role: %s for etcd: %v", etcd.GetRoleName(), etcd.GetNamespaceName()),
	)
}

func New(client client.Client) resource.Operator {
	return &_resource{
		client: client,
	}
}

func getObjectKey(etcd *druidv1alpha1.Etcd) client.ObjectKey {
	return client.ObjectKey{Name: etcd.GetRoleName(), Namespace: etcd.Namespace}
}

func emptyRole(etcd *druidv1alpha1.Etcd) *rbacv1.Role {
	return &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      etcd.GetRoleName(),
			Namespace: etcd.Namespace,
		},
	}
}

func buildResource(etcd *druidv1alpha1.Etcd, role *rbacv1.Role) {
	role.Labels = getLabels(etcd)
	role.OwnerReferences = []metav1.OwnerReference{etcd.GetAsOwnerReference()}
	role.Rules = []rbacv1.PolicyRule{
		{
			APIGroups: []string{"coordination.k8s.io"},
			Resources: []string{"leases"},
			Verbs:     []string{"get", "list", "patch", "update", "watch"},
		},
		{
			APIGroups: []string{"apps"},
			Resources: []string{"statefulsets"},
			Verbs:     []string{"get", "list", "patch", "update", "watch"},
		},
		{
			APIGroups: []string{""},
			Resources: []string{"pods"},
			Verbs:     []string{"get", "list", "watch"},
		},
	}
}

func getLabels(etcd *druidv1alpha1.Etcd) map[string]string {
	roleLabels := map[string]string{
		druidv1alpha1.LabelComponentKey: componentName,
		druidv1alpha1.LabelAppNameKey:   etcd.GetRoleName(),
	}
	return utils.MergeMaps[string, string](etcd.GetDefaultLabels(), roleLabels)
}
