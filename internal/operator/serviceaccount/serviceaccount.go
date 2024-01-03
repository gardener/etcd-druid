package serviceaccount

import (
	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/operator/resource"
	"github.com/gardener/gardener/pkg/controllerutils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type _resource struct {
	client           client.Client
	disableAutoMount bool
}

func (r _resource) GetExistingResourceNames(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd) ([]string, error) {
	resourceNames := make([]string, 0, 1)
	sa := &corev1.ServiceAccount{}
	if err := r.client.Get(ctx, getObjectKey(etcd), sa); err != nil {
		if errors.IsNotFound(err) {
			return resourceNames, nil
		}
		return resourceNames, err
	}
	resourceNames = append(resourceNames, sa.Name)
	return resourceNames, nil
}

func (r _resource) Sync(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	sa := emptyServiceAccount(getObjectKey(etcd))
	opResult, err := controllerutils.GetAndCreateOrStrategicMergePatch(ctx, r.client, sa, func() error {
		sa.Labels = etcd.GetDefaultLabels()
		sa.OwnerReferences = []metav1.OwnerReference{etcd.GetAsOwnerReference()}
		sa.AutomountServiceAccountToken = pointer.Bool(!r.disableAutoMount)
		return nil
	})
	ctx.Logger.Info("TriggerCreateOrUpdate operation result", "result", opResult)
	return err
}

func (r _resource) TriggerDelete(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	return client.IgnoreNotFound(r.client.Delete(ctx, emptyServiceAccount(getObjectKey(etcd))))
}

func New(client client.Client, disableAutomount bool) resource.Operator {
	return &_resource{
		client:           client,
		disableAutoMount: disableAutomount,
	}
}

func getObjectKey(etcd *druidv1alpha1.Etcd) client.ObjectKey {
	return client.ObjectKey{Name: etcd.GetServiceAccountName(), Namespace: etcd.Namespace}
}

func emptyServiceAccount(objectKey client.ObjectKey) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      objectKey.Name,
			Namespace: objectKey.Namespace,
		},
	}
}
