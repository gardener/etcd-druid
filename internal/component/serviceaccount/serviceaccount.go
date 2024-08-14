// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package serviceaccount

import (
	"fmt"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"
	"github.com/gardener/etcd-druid/internal/component"
	druiderr "github.com/gardener/etcd-druid/internal/errors"
	"github.com/gardener/etcd-druid/internal/utils"

	"github.com/gardener/gardener/pkg/controllerutils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// ErrGetServiceAccount indicates an error in getting the service account resource.
	ErrGetServiceAccount druidv1alpha1.ErrorCode = "ERR_GET_SERVICE_ACCOUNT"
	// ErrSyncServiceAccount indicates an error in syncing the service account resource.
	ErrSyncServiceAccount druidv1alpha1.ErrorCode = "ERR_SYNC_SERVICE_ACCOUNT"
	// ErrDeleteServiceAccount indicates an error in deleting the service account resource.
	ErrDeleteServiceAccount druidv1alpha1.ErrorCode = "ERR_DELETE_SERVICE_ACCOUNT"
)

type _resource struct {
	client           client.Client
	disableAutoMount bool
}

// New returns a new service account component operator.
func New(client client.Client, disableAutomount bool) component.Operator {
	return &_resource{
		client:           client,
		disableAutoMount: disableAutomount,
	}
}

// GetExistingResourceNames returns the name of the existing service account for the given Etcd.
func (r _resource) GetExistingResourceNames(ctx component.OperatorContext, etcdObjMeta metav1.ObjectMeta) ([]string, error) {
	resourceNames := make([]string, 0, 1)
	objectKey := getObjectKey(etcdObjMeta)
	objMeta := &metav1.PartialObjectMetadata{}
	objMeta.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("ServiceAccount"))
	if err := r.client.Get(ctx, objectKey, objMeta); err != nil {
		if errors.IsNotFound(err) {
			return resourceNames, nil
		}
		return resourceNames, druiderr.WrapError(err,
			ErrGetServiceAccount,
			"GetExistingResourceNames",
			fmt.Sprintf("Error getting service account: %v for etcd: %v", objectKey, druidv1alpha1.GetNamespaceName(etcdObjMeta)))
	}
	if metav1.IsControlledBy(objMeta, &etcdObjMeta) {
		resourceNames = append(resourceNames, objMeta.Name)
	}
	return resourceNames, nil
}

// PreSync is a no-op for the service account component.
func (r _resource) PreSync(_ component.OperatorContext, _ *druidv1alpha1.Etcd) error { return nil }

// Sync creates or updates the service account for the given Etcd.
func (r _resource) Sync(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	objectKey := getObjectKey(etcd.ObjectMeta)
	sa := emptyServiceAccount(objectKey)
	opResult, err := controllerutils.GetAndCreateOrStrategicMergePatch(ctx, r.client, sa, func() error {
		buildResource(etcd, sa, !r.disableAutoMount)
		return nil
	})
	if err != nil {
		return druiderr.WrapError(err,
			ErrSyncServiceAccount,
			"Sync",
			fmt.Sprintf("Error during create or update of service account: %v for etcd: %v", objectKey, druidv1alpha1.GetNamespaceName(etcd.ObjectMeta)),
		)
	}
	ctx.Logger.Info("synced", "component", "service-account", "objectKey", objectKey, "result", opResult)
	return nil
}

// TriggerDelete triggers the deletion of the service account for the given Etcd.
func (r _resource) TriggerDelete(ctx component.OperatorContext, etcdObjMeta metav1.ObjectMeta) error {
	ctx.Logger.Info("Triggering deletion of service account")
	objectKey := getObjectKey(etcdObjMeta)
	if err := r.client.Delete(ctx, emptyServiceAccount(objectKey)); err != nil {
		if errors.IsNotFound(err) {
			ctx.Logger.Info("No ServiceAccount found, Deletion is a No-Op", "objectKey", objectKey)
			return nil
		}
		return druiderr.WrapError(err,
			ErrDeleteServiceAccount,
			"TriggerDelete",
			fmt.Sprintf("Failed to delete service account: %v for etcd: %v", objectKey, druidv1alpha1.GetNamespaceName(etcdObjMeta)))
	}
	ctx.Logger.Info("deleted", "component", "service-account", "objectKey", objectKey)
	return nil
}

func buildResource(etcd *druidv1alpha1.Etcd, sa *corev1.ServiceAccount, autoMountServiceAccountToken bool) {
	sa.Labels = getLabels(etcd)
	sa.OwnerReferences = []metav1.OwnerReference{druidv1alpha1.GetAsOwnerReference(etcd.ObjectMeta)}
	sa.AutomountServiceAccountToken = pointer.Bool(autoMountServiceAccountToken)
}

func getLabels(etcd *druidv1alpha1.Etcd) map[string]string {
	roleLabels := map[string]string{
		druidv1alpha1.LabelComponentKey: common.ComponentNameServiceAccount,
		druidv1alpha1.LabelAppNameKey:   druidv1alpha1.GetServiceAccountName(etcd.ObjectMeta),
	}
	return utils.MergeMaps(druidv1alpha1.GetDefaultLabels(etcd.ObjectMeta), roleLabels)
}

func getObjectKey(obj metav1.ObjectMeta) client.ObjectKey {
	return client.ObjectKey{Name: druidv1alpha1.GetServiceAccountName(obj), Namespace: obj.Namespace}
}

func emptyServiceAccount(objectKey client.ObjectKey) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      objectKey.Name,
			Namespace: objectKey.Namespace,
		},
	}
}
