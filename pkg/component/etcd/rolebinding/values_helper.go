// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package rolebinding

import (
	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
)

// GenerateValues generates `serviceaccount.Values` for the serviceaccount component with the given `etcd` object.
func GenerateValues(etcd *druidv1alpha1.Etcd) *Values {
	return &Values{
		Name:               etcd.GetRoleBindingName(),
		Namespace:          etcd.Namespace,
		Labels:             etcd.GetDefaultLabels(),
		OwnerReference:     etcd.GetAsOwnerReference(),
		RoleName:           etcd.GetRoleName(),
		ServiceAccountName: etcd.GetServiceAccountName(),
	}
}
