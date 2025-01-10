package crds

import (
	_ "embed"
)

var (
	//go:embed crd-druid.gardener.cloud_etcds.yaml
	etcdCRD string
	//go:embed crd-druid.gardener.cloud_etcdcopybackupstasks.yaml
	etcdCopyBackupTaskCRD string
)

// GetEtcdCRD returns the etcd CRD.
func GetEtcdCRD() string {
	return etcdCRD
}

// GetEtcdCopyBackupTaskCRD returns the etcd-copy-backup-task CRD.
func GetEtcdCopyBackupTaskCRD() string {
	return etcdCopyBackupTaskCRD
}
