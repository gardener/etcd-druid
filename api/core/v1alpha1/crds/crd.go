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
	//go:embed druid.gardener.cloud_etcdopstasks.yaml
	etcdOpsTaskCRD string
)

const (
	// ResourceNameEtcd is the name of the etcd CRD.
	ResourceNameEtcd = "etcds.druid.gardener.cloud"
	// ResourceNameEtcdCopyBackupsTask is the name of the etcd-copy-backup-task CRD.
	ResourceNameEtcdCopyBackupsTask = "etcdcopybackupstasks.druid.gardener.cloud"
	// ResourceNameEtcdOpsTask is the name of the etcd-ops-task CRD.
	ResourceNameEtcdOpsTask = "etcdopstasks.druid.gardener.cloud"
)

// GetAll returns all CRDs for the given k8s version.
// The function will return a map with the CRD name as key and the CRD content as value.
// There are currently two sets of CRDs maintained.
// 1. One set is for Kubernetes version 1.29 and above. These CRDs contain embedded CEL validation expressions. CEL expressions are only GA since Kubernetes 1.29 onwards.
// 2. The other set is for Kubernetes versions below 1.29. These CRDs do not contain embedded CEL validation expressions.
// If there are any errors in parsing the k8s version, the function will return an error.
func GetAll(k8sVersion string) (map[string]string, error) {
	k8sVersionAbove129, err := IsK8sVersionEqualToOrAbove129(k8sVersion)
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
		ResourceNameEtcd:                selectedEtcdCRD,
		ResourceNameEtcdCopyBackupsTask: etcdCopyBackupsTaskCRD,
		ResourceNameEtcdOpsTask:         etcdOpsTaskCRD,
	}, nil
}

// IsK8sVersionEqualToOrAbove129 returns true if the K8s version is 1.29 or higher, otherwise false.
func IsK8sVersionEqualToOrAbove129(k8sVersion string) (bool, error) {
	v, err := version.ParseMajorMinor(k8sVersion)
	if err != nil {
		return false, err
	}
	if v.LessThan(version.MajorMinor(1, 29)) {
		return false, nil
	}
	return true, nil
}
