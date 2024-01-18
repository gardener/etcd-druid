package serviceaccount

import (
	"fmt"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	druiderr "github.com/gardener/etcd-druid/internal/errors"
	"github.com/gardener/etcd-druid/internal/operator/resource"
	"github.com/gardener/etcd-druid/internal/utils"
	"github.com/gardener/gardener/pkg/controllerutils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ErrGetServiceAccount    druidv1alpha1.ErrorCode = "ERR_GET_SERVICE_ACCOUNT"
	ErrDeleteServiceAccount druidv1alpha1.ErrorCode = "ERR_DELETE_SERVICE_ACCOUNT"
	ErrSyncServiceAccount   druidv1alpha1.ErrorCode = "ERR_SYNC_SERVICE_ACCOUNT"
)

const componentName = "druid-service-account"

type _resource struct {
	client           client.Client
	disableAutoMount bool
}

func (r _resource) GetExistingResourceNames(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd) ([]string, error) {
	resourceNames := make([]string, 0, 1)
	sa := &corev1.ServiceAccount{}
	saObjectKey := getObjectKey(etcd)
	if err := r.client.Get(ctx, saObjectKey, sa); err != nil {
		if errors.IsNotFound(err) {
			return resourceNames, nil
		}
		return resourceNames, druiderr.WrapError(err,
			ErrGetServiceAccount,
			"GetExistingResourceNames",
			fmt.Sprintf("Error getting service account: %s for etcd: %v", saObjectKey.Name, etcd.GetNamespaceName()))
	}
	resourceNames = append(resourceNames, sa.Name)
	return resourceNames, nil
}

func (r _resource) Sync(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	sa := emptyServiceAccount(getObjectKey(etcd))
	opResult, err := controllerutils.GetAndCreateOrStrategicMergePatch(ctx, r.client, sa, func() error {
		buildResource(etcd, sa, !r.disableAutoMount)
		return nil
	})
	if err == nil {
		ctx.Logger.Info("synced", "resource", "service-account", "name", sa.Name, "result", opResult)
		return nil
	}
	return druiderr.WrapError(err,
		ErrSyncServiceAccount,
		"Sync",
		fmt.Sprintf("Error during create or update of service account: %s for etcd: %v", sa.Name, etcd.GetNamespaceName()))
}

func (r _resource) TriggerDelete(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	ctx.Logger.Info("Triggering delete of service account")
	saObjectKey := getObjectKey(etcd)
	if err := client.IgnoreNotFound(r.client.Delete(ctx, emptyServiceAccount(saObjectKey))); err != nil {
		return druiderr.WrapError(err,
			ErrDeleteServiceAccount,
			"TriggerDelete",
			fmt.Sprintf("Failed to delete service account: %s for etcd: %v", saObjectKey.Name, etcd.GetNamespaceName()))
	}
	ctx.Logger.Info("deleted", "resource", "service-account", "name", saObjectKey.Name)
	return nil
}

func New(client client.Client, disableAutomount bool) resource.Operator {
	return &_resource{
		client:           client,
		disableAutoMount: disableAutomount,
	}
}

func buildResource(etcd *druidv1alpha1.Etcd, sa *corev1.ServiceAccount, autoMountServiceAccountToken bool) {
	sa.Labels = getLabels(etcd)
	sa.OwnerReferences = []metav1.OwnerReference{etcd.GetAsOwnerReference()}
	sa.AutomountServiceAccountToken = pointer.Bool(autoMountServiceAccountToken)
}

func getLabels(etcd *druidv1alpha1.Etcd) map[string]string {
	roleLabels := map[string]string{
		druidv1alpha1.LabelComponentKey: componentName,
		druidv1alpha1.LabelAppNameKey:   etcd.GetServiceAccountName(),
	}
	return utils.MergeMaps[string, string](etcd.GetDefaultLabels(), roleLabels)
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
