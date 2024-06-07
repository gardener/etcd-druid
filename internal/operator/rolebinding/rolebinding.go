// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package rolebinding

import (
	"fmt"
	"strings"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"
	druiderr "github.com/gardener/etcd-druid/internal/errors"
	"github.com/gardener/etcd-druid/internal/operator/component"
	"github.com/gardener/etcd-druid/internal/utils"
	"github.com/gardener/gardener/pkg/controllerutils"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// ErrGetRoleBinding indicates an error in getting the role binding resource.
	ErrGetRoleBinding druidv1alpha1.ErrorCode = "ERR_GET_ROLE_BINDING"
	// ErrSyncRoleBinding indicates an error in syncing the role binding resource.
	ErrSyncRoleBinding druidv1alpha1.ErrorCode = "ERR_SYNC_ROLE_BINDING"
	// ErrDeleteRoleBinding indicates an error in deleting the role binding resource.
	ErrDeleteRoleBinding druidv1alpha1.ErrorCode = "ERR_DELETE_ROLE_BINDING"
)

type _resource struct {
	client client.Client
}

// New returns a new role binding operator.
func New(client client.Client) component.Operator {
	return &_resource{
		client: client,
	}
}

// GetExistingResourceNames returns the name of the existing role binding for the given Etcd.
func (r _resource) GetExistingResourceNames(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) ([]string, error) {
	resourceNames := make([]string, 0, 1)
	objectKey := getObjectKey(etcd)
	rb := &rbacv1.RoleBinding{}
	if err := r.client.Get(ctx, objectKey, rb); err != nil {
		if errors.IsNotFound(err) {
			return resourceNames, nil
		}
		return resourceNames, druiderr.WrapError(err,
			ErrGetRoleBinding,
			"GetExistingResourceNames",
			fmt.Sprintf("Error getting role-binding: %v for etcd: %v", objectKey, etcd.GetNamespaceName()))
	}
	if metav1.IsControlledBy(rb, etcd) {
		resourceNames = append(resourceNames, rb.Name)
	}
	return resourceNames, nil
}

// Sync creates or updates the role binding for the given Etcd.
func (r _resource) Sync(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	objectKey := getObjectKey(etcd)
	rb := emptyRoleBinding(objectKey)
	result, err := controllerutils.GetAndCreateOrStrategicMergePatch(ctx, r.client, rb, func() error {
		buildResource(etcd, rb)
		return nil
	})
	if err != nil {
		return druiderr.WrapError(err,
			ErrSyncRoleBinding,
			"Sync",
			fmt.Sprintf("Error during create or update of role-binding %v for etcd: %v", objectKey, etcd.GetNamespaceName()),
		)
	}
	ctx.Logger.Info("synced", "component", "role", "objectKey", objectKey, "result", result)
	return nil
}

// TriggerDelete triggers the deletion of the role binding for the given Etcd.
func (r _resource) TriggerDelete(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	objectKey := getObjectKey(etcd)
	ctx.Logger.Info("Triggering delete of role", "objectKey", objectKey)
	err := r.client.Delete(ctx, emptyRoleBinding(objectKey))
	if err != nil {
		if errors.IsNotFound(err) {
			ctx.Logger.Info("No RoleBinding found, Deletion is a No-Op", "objectKey", objectKey)
			return nil
		}
		return druiderr.WrapError(err,
			ErrDeleteRoleBinding,
			"TriggerDelete",
			fmt.Sprintf("Failed to delete role-binding: %v for etcd: %v", objectKey, etcd.GetNamespaceName()),
		)
	}
	ctx.Logger.Info("deleted", "component", "role-binding", "objectKey", objectKey)
	return nil
}

func getObjectKey(etcd *druidv1alpha1.Etcd) client.ObjectKey {
	return client.ObjectKey{Name: etcd.GetRoleBindingName(), Namespace: etcd.Namespace}
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
		druidv1alpha1.LabelAppNameKey:   strings.ReplaceAll(etcd.GetRoleBindingName(), ":", "-"), // role-binding name contains `:` which is not an allowed character as a label value.
	}
	return utils.MergeMaps[string, string](etcd.GetDefaultLabels(), roleLabels)
}
