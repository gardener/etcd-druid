// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package assets

import (
	"errors"
	"fmt"
	"os"

	"github.com/gardener/etcd-druid/api/core/v1alpha1/crds"
	"github.com/gardener/etcd-druid/internal/images"
	"github.com/gardener/etcd-druid/internal/utils/imagevector"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/yaml"

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

// getCRD is a helper function that retrieves and unmarshals a CRD by its resource name.
func getCRD(k8sVersion, resourceName string) (*apiextensionsv1.CustomResourceDefinition, error) {
	allCRDs, err := crds.GetAll(k8sVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to get CRDs for k8s version %s: %w", k8sVersion, err)
	}

	crd := &apiextensionsv1.CustomResourceDefinition{}
	if err := yaml.Unmarshal([]byte(allCRDs[resourceName]), crd); err != nil {
		return nil, fmt.Errorf("failed to unmarshal CRD %s: %w", resourceName, err)
	}
	return crd, nil
}

// GetEtcdCrd returns the Etcd CRD object for k8s versions >= 1.29 or the Etcd CRD without CEL expressions (For versions < 1.29)
func GetEtcdCrd(k8sVersion string) (*apiextensionsv1.CustomResourceDefinition, error) {
	return getCRD(k8sVersion, crds.ResourceNameEtcd)
}

// GetEtcdCopyBackupsTaskCrd returns the EtcdCopyBackupsTask CRD object.
func GetEtcdCopyBackupsTaskCrd(k8sVersion string) (*apiextensionsv1.CustomResourceDefinition, error) {
	return getCRD(k8sVersion, crds.ResourceNameEtcdCopyBackupsTask)
}

// GetEtcdOpsTaskCrd returns the EtcdOpsTask CRD object.
func GetEtcdOpsTaskCrd(k8sVersion string) (*apiextensionsv1.CustomResourceDefinition, error) {
	return getCRD(k8sVersion, crds.ResourceNameEtcdOpsTask)
}

// CreateImageVector creates an image vector.
func CreateImageVector(g *WithT) imagevector.ImageVector {
	imageVector, err := images.CreateImageVector()
	g.Expect(err).To(BeNil())
	return imageVector
}
