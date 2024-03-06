// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package role

import (
	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	rbacv1 "k8s.io/api/rbac/v1"
)

// GenerateValues generates `role.Values` for the role component with the given `etcd` object.
func GenerateValues(etcd *druidv1alpha1.Etcd) *Values {
	return &Values{
		Name:           etcd.GetRoleName(),
		Namespace:      etcd.Namespace,
		Labels:         etcd.GetDefaultLabels(),
		OwnerReference: etcd.GetAsOwnerReference(),
		Rules: []rbacv1.PolicyRule{
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
		},
	}
}
