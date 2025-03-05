// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package crds

import (
	"k8s.io/apimachinery/pkg/util/version"

	_ "embed"
)

var (
	//go:embed druid.gardener.cloud_etcds.yaml
	etcdCRD string
	//go:embed druid.gardener.cloud_etcds_without_cel.yaml
	etcdCRDWithoutCEL string
	//go:embed druid.gardener.cloud_etcdcopybackupstasks.yaml
	etcdCopyBackupsTaskCRD string
)

const (
	// KindEtcd is the kind of the etcd CRD.
	KindEtcd = "Etcd"
	// KindEtcdCopyBackupsTask is the kind of the etcd-copy-backup-task CRD.
	KindEtcdCopyBackupsTask = "EtcdCopyBackupsTask"
)

// GetAll returns all CRDs for the given k8s version.
// There are currently two sets of CRDs maintained.
// 1. One set is for Kubernetes version 1.29 and above. These CRDs contain embedded CEL validation expressions. CEL expressions are only GA since Kubernetes 1.29 onwards.
// 2. The other set is for Kubernetes versions below 1.29. These CRDs do not contain embedded CEL validation expressions.
func GetAll(k8sVersion string) (map[string]string, error) {
	k8sVersionAbove129, err := IsK8sVersionAbove129(k8sVersion)
	if err != nil {
		return nil, err
	}
	var selectedEtcdCRD string
	if k8sVersionAbove129 {
		selectedEtcdCRD = etcdCRD
	} else {
		selectedEtcdCRD = etcdCRDWithoutCEL
	}
	return map[string]string{
		KindEtcd:                selectedEtcdCRD,
		KindEtcdCopyBackupsTask: etcdCopyBackupsTaskCRD,
	}, nil
}

// Returns true if the K8s version is 1.29 or higher, otherwise false.
func IsK8sVersionAbove129(k8sVersion string) (bool, error) {
	v, err := version.ParseMajorMinor(k8sVersion)
	if err != nil {
		return false, err
	}
	if v.LessThan(version.MajorMinor(1, 29)) {
		return false, nil
	}
	return true, nil
}
