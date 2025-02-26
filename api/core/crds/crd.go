// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package crds

import (
	_ "embed"
)

var (
	//go:embed druid.gardener.cloud_etcds.yaml
	etcdCRD string
	//go:embed druid.gardener.cloud_etcdcopybackupstasks.yaml
	etcdCopyBackupsTaskCRD string
)

// GetEtcdCRD returns the etcd CRD.
func GetEtcdCRD() string {
	return etcdCRD
}

// GetEtcdCopyBackupsTaskCRD returns the etcd-copy-backup-task CRD.
func GetEtcdCopyBackupsTaskCRD() string {
	return etcdCopyBackupsTaskCRD
}
