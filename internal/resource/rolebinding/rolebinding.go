package rolebinding

import (
	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/registry/resource"
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
	rb := &rbacv1.RoleBinding{}
	if err := r.client.Get(ctx, getObjectKey(etcd), rb); err != nil {
		if errors.IsNotFound(err) {
			return resourceNames, nil
		}
		return resourceNames, err
	}
	resourceNames = append(resourceNames, rb.Name)
	return resourceNames, nil
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

func (r _resource) Sync(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	rb := emptyRoleBinding(etcd)
	opResult, err := controllerutils.GetAndCreateOrStrategicMergePatch(ctx, r.client, rb, func() error {
		rb.Labels = etcd.GetDefaultLabels()
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
		return nil
	})
	r.logger.Info("TriggerCreateOrUpdate operation result", "result", opResult)
	return err
}

func (r _resource) TriggerDelete(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	return client.IgnoreNotFound(r.client.Delete(ctx, emptyRoleBinding(etcd)))
}

func New(client client.Client, logger logr.Logger) resource.Operator {
	return &_resource{
		client: client,
		logger: logger,
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
