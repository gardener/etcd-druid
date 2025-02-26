// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package role

import (
	"fmt"
	"strings"

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
	// ErrGetRole indicates an error in getting the role resource.
	ErrGetRole druidv1alpha1.ErrorCode = "ERR_GET_ROLE"
	// ErrSyncRole indicates an error in syncing the role resource.
	ErrSyncRole druidv1alpha1.ErrorCode = "ERR_SYNC_ROLE"
	// ErrDeleteRole indicates an error in deleting the role resource.
	ErrDeleteRole druidv1alpha1.ErrorCode = "ERR_DELETE_ROLE"
)

type _resource struct {
	client client.Client
}

// New returns a new role component operator.
func New(client client.Client) component.Operator {
	return &_resource{
		client: client,
	}
}

// GetExistingResourceNames returns the name of the existing role for the given Etcd.
func (r _resource) GetExistingResourceNames(ctx component.OperatorContext, etcdObjMeta metav1.ObjectMeta) ([]string, error) {
	resourceNames := make([]string, 0, 1)
	objectKey := getObjectKey(etcdObjMeta)
	objMeta := &metav1.PartialObjectMetadata{}
	objMeta.SetGroupVersionKind(rbacv1.SchemeGroupVersion.WithKind("Role"))
	if err := r.client.Get(ctx, objectKey, objMeta); err != nil {
		if errors.IsNotFound(err) {
			return resourceNames, nil
		}
		return resourceNames, druiderr.WrapError(err,
			ErrGetRole,
			component.OperationGetExistingResourceNames,
			fmt.Sprintf("Error getting role: %v for etcd: %v", objectKey, druidv1alpha1.GetNamespaceName(etcdObjMeta)))
	}
	if metav1.IsControlledBy(objMeta, &etcdObjMeta) {
		resourceNames = append(resourceNames, objMeta.Name)
	}
	return resourceNames, nil
}

// PreSync is a no-op for the role component.
func (r _resource) PreSync(_ component.OperatorContext, _ *druidv1alpha1.Etcd) error { return nil }

// Sync creates or updates the role for the given Etcd.
func (r _resource) Sync(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	objectKey := getObjectKey(etcd.ObjectMeta)
	role := emptyRole(objectKey)
	result, err := controllerutil.CreateOrPatch(ctx, r.client, role, func() error {
		buildResource(etcd, role)
		return nil
	})
	if err != nil {
		return druiderr.WrapError(err,
			ErrSyncRole,
			component.OperationSync,
			fmt.Sprintf("Error during create or update of role %v for etcd: %v", objectKey, druidv1alpha1.GetNamespaceName(etcd.ObjectMeta)),
		)
	}
	ctx.Logger.Info("synced", "component", "role", "objectKey", objectKey, "result", result)
	return nil
}

// TriggerDelete triggers the deletion of the role for the given Etcd.
func (r _resource) TriggerDelete(ctx component.OperatorContext, etcdObjMeta metav1.ObjectMeta) error {
	objectKey := getObjectKey(etcdObjMeta)
	ctx.Logger.Info("Triggering deletion of role", "objectKey", objectKey)
	if err := r.client.Delete(ctx, emptyRole(objectKey)); err != nil {
		if errors.IsNotFound(err) {
			ctx.Logger.Info("No Role found, Deletion is a No-Op", "objectKey", objectKey)
			return nil
		}
		return druiderr.WrapError(err,
			ErrDeleteRole,
			component.OperationTriggerDelete,
			fmt.Sprintf("Failed to delete role: %v for etcd: %v", objectKey, druidv1alpha1.GetNamespaceName(etcdObjMeta)),
		)
	}
	ctx.Logger.Info("deleted", "component", "role", "objectKey", objectKey)
	return nil
}

func getObjectKey(obj metav1.ObjectMeta) client.ObjectKey {
	return client.ObjectKey{Name: druidv1alpha1.GetRoleName(obj), Namespace: obj.Namespace}
}

func emptyRole(objectKey client.ObjectKey) *rbacv1.Role {
	return &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      objectKey.Name,
			Namespace: objectKey.Namespace,
		},
	}
}

func buildResource(etcd *druidv1alpha1.Etcd, role *rbacv1.Role) {
	role.Labels = getLabels(etcd)
	role.OwnerReferences = []metav1.OwnerReference{druidv1alpha1.GetAsOwnerReference(etcd.ObjectMeta)}
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
		druidv1alpha1.LabelComponentKey: common.ComponentNameRole,
		druidv1alpha1.LabelAppNameKey:   strings.ReplaceAll(druidv1alpha1.GetRoleName(etcd.ObjectMeta), ":", "-"), // role name contains `:` which is not an allowed character as a label value.
	}
	return utils.MergeMaps(druidv1alpha1.GetDefaultLabels(etcd.ObjectMeta), roleLabels)
}
