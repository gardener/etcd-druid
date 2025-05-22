// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	druidstore "github.com/gardener/etcd-druid/internal/store"

	"k8s.io/utils/ptr"
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

// MountPathLocalStore returns the mount path for the local store if the provider matches.
func MountPathLocalStore(etcd *druidv1alpha1.Etcd, provider *string) string {
	if !etcd.IsBackupStoreEnabled() || provider == nil || *provider != druidstore.Local {
		return ""
	}

	homeDir := "/home/nonroot"
	if ptr.Deref(etcd.Spec.RunAsRoot, false) {
		homeDir = "/root"
	}

	path := ""
	if container := etcd.Spec.Backup.Store.Container; container != nil {
		path = "/" + *container
	}

	return homeDir + path
}
