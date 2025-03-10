// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package assets

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/gardener/etcd-druid/internal/images"
	"github.com/gardener/etcd-druid/internal/utils/imagevector"

	. "github.com/onsi/gomega"
)

// GetK8sVersionFromEnv returns the kubernetes version used from the environment variable
func GetK8sVersionFromEnv() (string, error) {
	k8sVersion, isPresent := os.LookupEnv("ENVTEST_K8S_VERSION")
	if isPresent {
		return k8sVersion, nil
	} else {
		return "", errors.New("error fetching k8s version from environment")
	}
}

// Duplicating some code here that can be replaced with a call to the API module of etcd-druid.
// This will be removed once Kubernetes 1.29 becomes the minimum supported version.

// GetEtcdCrdPath returns the path to the Etcd CRD for k8s versions >= 1.29 or the path to the Etcd CRD without CEL expressions (For versions < 1.29)
func GetEtcdCrdPath(k8sVersionAbove129 bool) string {
	if k8sVersionAbove129 {
		return filepath.Join("..", "..", "..", "..", "api", "core", "crds", "druid.gardener.cloud_etcds.yaml")
	}
	return filepath.Join("..", "..", "..", "..", "api", "core", "crds", "druid.gardener.cloud_etcds_without_cel.yaml")
}

// GetEtcdCopyBackupsTaskCrdPath returns the path to the EtcdCopyBackupsTask CRD.
func GetEtcdCopyBackupsTaskCrdPath() string {
	return filepath.Join("..", "..", "..", "..", "api", "core", "crds", "druid.gardener.cloud_etcdcopybackupstasks.yaml")
}

// CreateImageVector creates an image vector.
func CreateImageVector(g *WithT) imagevector.ImageVector {
	imageVector, err := images.CreateImageVector()
	g.Expect(err).To(BeNil())
	return imageVector
}
