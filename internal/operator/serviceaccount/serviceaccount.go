// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package serviceaccount

import (
	"fmt"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"
	druiderr "github.com/gardener/etcd-druid/internal/errors"
	"github.com/gardener/etcd-druid/internal/operator/component"
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

type _resource struct {
	client           client.Client
	disableAutoMount bool
}

func (r _resource) GetExistingResourceNames(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) ([]string, error) {
	resourceNames := make([]string, 0, 1)
	sa := &corev1.ServiceAccount{}
	objectKey := getObjectKey(etcd)
	if err := r.client.Get(ctx, objectKey, sa); err != nil {
		if errors.IsNotFound(err) {
			return resourceNames, nil
		}
		return resourceNames, druiderr.WrapError(err,
			ErrGetServiceAccount,
			"GetExistingResourceNames",
			fmt.Sprintf("Error getting service account: %v for etcd: %v", objectKey, etcd.GetNamespaceName()))
	}
	if metav1.IsControlledBy(sa, etcd) {
		resourceNames = append(resourceNames, sa.Name)
	}
	return resourceNames, nil
}

func (r _resource) Sync(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	objectKey := getObjectKey(etcd)
	sa := emptyServiceAccount(objectKey)
	opResult, err := controllerutils.GetAndCreateOrStrategicMergePatch(ctx, r.client, sa, func() error {
		buildResource(etcd, sa, !r.disableAutoMount)
		return nil
	})
	if err != nil {
		return druiderr.WrapError(err,
			ErrSyncServiceAccount,
			"Sync",
			fmt.Sprintf("Error during create or update of service account: %v for etcd: %v", objectKey, etcd.GetNamespaceName()),
		)
	}
	ctx.Logger.Info("synced", "component", "service-account", "objectKey", objectKey, "result", opResult)
	return nil
}

func (r _resource) TriggerDelete(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	ctx.Logger.Info("Triggering delete of service account")
	objectKey := getObjectKey(etcd)
	if err := r.client.Delete(ctx, emptyServiceAccount(objectKey)); err != nil {
		if errors.IsNotFound(err) {
			ctx.Logger.Info("No ServiceAccount found, Deletion is a No-Op", "objectKey", objectKey)
			return nil
		}
		return druiderr.WrapError(err,
			ErrDeleteServiceAccount,
			"TriggerDelete",
			fmt.Sprintf("Failed to delete service account: %v for etcd: %v", objectKey, etcd.GetNamespaceName()))
	}
	ctx.Logger.Info("deleted", "component", "service-account", "objectKey", objectKey)
	return nil
}

func New(client client.Client, disableAutomount bool) component.Operator {
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
		druidv1alpha1.LabelComponentKey: common.ServiceAccountComponentName,
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
