package role

import (
	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/operator/resource"
	"github.com/gardener/gardener/pkg/controllerutils"
	"github.com/go-logr/logr"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type _resource struct {
	client client.Client
	logger logr.Logger
}

func (r _resource) GetExistingResourceNames(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd) ([]string, error) {
	resourceNames := make([]string, 0, 1)
	role := &rbacv1.Role{}
	if err := r.client.Get(ctx, getObjectKey(etcd), role); err != nil {
		if errors.IsNotFound(err) {
			return resourceNames, nil
		}
		return resourceNames, err
	}
	resourceNames = append(resourceNames, role.Name)
	return resourceNames, nil
}

func (r _resource) Sync(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	role := emptyRole(etcd)
	_, err := controllerutils.GetAndCreateOrStrategicMergePatch(ctx, r.client, role, func() error {
		role.Labels = etcd.GetDefaultLabels()
		role.OwnerReferences = []metav1.OwnerReference{etcd.GetAsOwnerReference()}
		role.Rules = createPolicyRules()
		return nil
	})
	return err
}

func (r _resource) TriggerDelete(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	return r.client.Delete(ctx, emptyRole(etcd))
}

func New(client client.Client, logger logr.Logger) resource.Operator {
	return &_resource{
		client: client,
		logger: logger,
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

func createPolicyRules() []rbacv1.PolicyRule {
	return []rbacv1.PolicyRule{
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
