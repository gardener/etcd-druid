// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	druidstore "github.com/gardener/etcd-druid/internal/store"
)

// GetBackupStoreProvider returns the provider name for the backup store. If the provider is not known, an error is
// returned.
func GetBackupStoreProvider(etcd *druidv1alpha1.Etcd) (*string, error) {
	if !etcd.IsBackupStoreEnabled() {
		return nil, nil
	}
	provider, err := druidstore.StorageProviderFromInfraProvider(etcd.Spec.Backup.Store.Provider)
	if err != nil {
		return nil, err
	}
	return &provider, nil
}
