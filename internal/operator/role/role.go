// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package role

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
	ErrGetRole    druidv1alpha1.ErrorCode = "ERR_GET_ROLE"
	ErrSyncRole   druidv1alpha1.ErrorCode = "ERR_SYNC_ROLE"
	ErrDeleteRole druidv1alpha1.ErrorCode = "ERR_DELETE_ROLE"
)

type _resource struct {
	client client.Client
}

func (r _resource) GetExistingResourceNames(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) ([]string, error) {
	resourceNames := make([]string, 0, 1)
	objectKey := getObjectKey(etcd)
	role := &rbacv1.Role{}
	if err := r.client.Get(ctx, objectKey, role); err != nil {
		if errors.IsNotFound(err) {
			return resourceNames, nil
		}
		return resourceNames, druiderr.WrapError(err,
			ErrGetRole,
			"GetExistingResourceNames",
			fmt.Sprintf("Error getting role: %v for etcd: %v", objectKey, etcd.GetNamespaceName()))
	}
	if metav1.IsControlledBy(role, etcd) {
		resourceNames = append(resourceNames, role.Name)
	}
	return resourceNames, nil
}

func (r _resource) Sync(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	objectKey := getObjectKey(etcd)
	role := emptyRole(objectKey)
	result, err := controllerutils.GetAndCreateOrStrategicMergePatch(ctx, r.client, role, func() error {
		buildResource(etcd, role)
		return nil
	})
	if err != nil {
		return druiderr.WrapError(err,
			ErrSyncRole,
			"Sync",
			fmt.Sprintf("Error during create or update of role %v for etcd: %v", objectKey, etcd.GetNamespaceName()),
		)
	}
	ctx.Logger.Info("synced", "component", "role", "objectKey", objectKey, "result", result)
	return nil
}

func (r _resource) TriggerDelete(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	objectKey := getObjectKey(etcd)
	ctx.Logger.Info("Triggering delete of role", "objectKey", objectKey)
	if err := r.client.Delete(ctx, emptyRole(objectKey)); err != nil {
		if errors.IsNotFound(err) {
			ctx.Logger.Info("No Role found, Deletion is a No-Op", "objectKey", objectKey)
			return nil
		}
		return druiderr.WrapError(err,
			ErrDeleteRole,
			"TriggerDelete",
			fmt.Sprintf("Failed to delete role: %v for etcd: %v", objectKey, etcd.GetNamespaceName()),
		)
	}
	ctx.Logger.Info("deleted", "component", "role", "objectKey", objectKey)
	return nil
}

func New(client client.Client) component.Operator {
	return &_resource{
		client: client,
	}
}

func getObjectKey(etcd *druidv1alpha1.Etcd) client.ObjectKey {
	return client.ObjectKey{Name: etcd.GetRoleName(), Namespace: etcd.Namespace}
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
		druidv1alpha1.LabelComponentKey: common.RoleComponentName,
		druidv1alpha1.LabelAppNameKey:   strings.ReplaceAll(etcd.GetRoleName(), ":", "-"), // role name contains `:` which is not an allowed character as a label value.
	}
	return utils.MergeMaps[string, string](etcd.GetDefaultLabels(), roleLabels)
}
