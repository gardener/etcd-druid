// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package serviceaccount

import (
	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
)

// GenerateValues generates `serviceaccount.Values` for the serviceaccount component for the given `etcd` object.
func GenerateValues(etcd *druidv1alpha1.Etcd, disableEtcdServiceAccountAutomount bool) *Values {
	return &Values{
		Name:             etcd.GetServiceAccountName(),
		Namespace:        etcd.Namespace,
		Labels:           etcd.GetDefaultLabels(),
		OwnerReference:   etcd.GetAsOwnerReference(),
		DisableAutomount: disableEtcdServiceAccountAutomount,
	}
}
