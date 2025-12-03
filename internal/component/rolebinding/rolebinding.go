// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package rolebinding

import (
	"fmt"
	"strings"

	druidapicommon "github.com/gardener/etcd-druid/api/common"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"
	"github.com/gardener/etcd-druid/internal/component"
	druiderr "github.com/gardener/etcd-druid/internal/errors"
	"github.com/gardener/etcd-druid/internal/utils"

	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	// ErrGetRoleBinding indicates an error in getting the role binding resource.
	ErrGetRoleBinding druidapicommon.ErrorCode = "ERR_GET_ROLE_BINDING"
	// ErrSyncRoleBinding indicates an error in syncing the role binding resource.
	ErrSyncRoleBinding druidapicommon.ErrorCode = "ERR_SYNC_ROLE_BINDING"
	// ErrDeleteRoleBinding indicates an error in deleting the role binding resource.
	ErrDeleteRoleBinding druidapicommon.ErrorCode = "ERR_DELETE_ROLE_BINDING"
)

type _resource struct {
	client client.Client
}

// New returns a new role binding component operator.
func New(client client.Client) component.Operator {
	return &_resource{
		client: client,
	}
}

// GetExistingResourceNames returns the name of the existing role binding for the given Etcd.
func (r _resource) GetExistingResourceNames(ctx component.OperatorContext, etcdObjMeta metav1.ObjectMeta) ([]string, error) {
	resourceNames := make([]string, 0, 1)
	objectKey := getObjectKey(etcdObjMeta)
	objMeta := &metav1.PartialObjectMetadata{}
	objMeta.SetGroupVersionKind(rbacv1.SchemeGroupVersion.WithKind("RoleBinding"))
	if err := r.client.Get(ctx, objectKey, objMeta); err != nil {
		if errors.IsNotFound(err) {
			return resourceNames, nil
		}
		return resourceNames, druiderr.WrapError(err,
			ErrGetRoleBinding,
			component.OperationGetExistingResourceNames,
			fmt.Sprintf("Error getting role-binding: %v for etcd: %v", objectKey, druidv1alpha1.GetNamespaceName(etcdObjMeta)))
	}
	if metav1.IsControlledBy(objMeta, &etcdObjMeta) {
		resourceNames = append(resourceNames, objMeta.Name)
	}
	return resourceNames, nil
}

// PreSync is a no-op for the role binding component.
func (r _resource) PreSync(_ component.OperatorContext, _ *druidv1alpha1.Etcd) error { return nil }

// Sync creates or updates the role binding for the given Etcd.
func (r _resource) Sync(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	objectKey := getObjectKey(etcd.ObjectMeta)
	rb := emptyRoleBinding(objectKey)
	result, err := controllerutil.CreateOrPatch(ctx, r.client, rb, func() error {
		buildResource(etcd, rb)
		return nil
	})
	if err != nil {
		return druiderr.WrapError(err,
			ErrSyncRoleBinding,
			component.OperationSync,
			fmt.Sprintf("Error during create or update of role-binding %v for etcd: %v", objectKey, druidv1alpha1.GetNamespaceName(etcd.ObjectMeta)),
		)
	}
	ctx.Logger.Info("synced", "component", "role-binding", "objectKey", objectKey, "result", result)
	return nil
}

// TriggerDelete triggers the deletion of the role binding for the given Etcd.
func (r _resource) TriggerDelete(ctx component.OperatorContext, etcdObjMeta metav1.ObjectMeta) error {
	objectKey := getObjectKey(etcdObjMeta)
	ctx.Logger.Info("Triggering deletion of role-binding", "objectKey", objectKey)
	if err := r.client.Delete(ctx, emptyRoleBinding(objectKey)); err != nil {
		if errors.IsNotFound(err) {
			ctx.Logger.Info("No RoleBinding found, Deletion is a No-Op", "objectKey", objectKey)
			return nil
		}
		return druiderr.WrapError(err,
			ErrDeleteRoleBinding,
			component.OperationTriggerDelete,
			fmt.Sprintf("Failed to delete role-binding: %v for etcd: %v", objectKey, druidv1alpha1.GetNamespaceName(etcdObjMeta)),
		)
	}
	ctx.Logger.Info("deleted", "component", "role-binding", "objectKey", objectKey)
	return nil
}

func getObjectKey(obj metav1.ObjectMeta) client.ObjectKey {
	return client.ObjectKey{Name: druidv1alpha1.GetRoleBindingName(obj), Namespace: obj.Namespace}
}

func emptyRoleBinding(objKey client.ObjectKey) *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      objKey.Name,
			Namespace: objKey.Namespace,
		},
	}
}

func buildResource(etcd *druidv1alpha1.Etcd, rb *rbacv1.RoleBinding) {
	rb.Labels = getLabels(etcd)
	rb.OwnerReferences = []metav1.OwnerReference{druidv1alpha1.GetAsOwnerReference(etcd.ObjectMeta)}
	rb.RoleRef = rbacv1.RoleRef{
		APIGroup: "rbac.authorization.k8s.io",
		Kind:     "Role",
		Name:     druidv1alpha1.GetRoleName(etcd.ObjectMeta),
	}
	rb.Subjects = []rbacv1.Subject{
		{
			Kind:      "ServiceAccount",
			Name:      druidv1alpha1.GetServiceAccountName(etcd.ObjectMeta),
			Namespace: etcd.Namespace,
		},
	}
}

func getLabels(etcd *druidv1alpha1.Etcd) map[string]string {
	roleLabels := map[string]string{
		druidv1alpha1.LabelComponentKey: common.ComponentNameRoleBinding,
		druidv1alpha1.LabelAppNameKey:   strings.ReplaceAll(druidv1alpha1.GetRoleBindingName(etcd.ObjectMeta), ":", "-"), // role-binding name contains `:` which is not an allowed character as a label value.
	}
	return utils.MergeMaps(druidv1alpha1.GetDefaultLabels(etcd.ObjectMeta), roleLabels)
}
