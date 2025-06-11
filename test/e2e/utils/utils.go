// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"fmt"
	"os"
	"slices"
	"strings"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var knownBackupProviders = []druidv1alpha1.StorageProvider{
	"local",
	"none",
	// TODO: enable once emulator support is added to e2e tests
	//"aws",
	//"azure",
	//"gcp",
	//"openstack",
}

func GetEnvOrError(key string) (string, error) {
	if value, ok := os.LookupEnv(key); ok {
		return value, nil
	}

	return "", fmt.Errorf("environment variable not found: %s", key)
}

func getKubeconfig(kubeconfigPath string) (*rest.Config, error) {
	return clientcmd.BuildConfigFromFlags("", kubeconfigPath)
}

// GetKubernetesClient creates a Kubernetes client using the provided kubeconfig path.
func GetKubernetesClient(kubeconfigPath string) (client.Client, error) {
	config, err := getKubeconfig(kubeconfigPath)
	if err != nil {
		return nil, err
	}

	return client.New(config, client.Options{})
}

// ParseBackupProviders parses a comma-separated string of backup providers
func ParseBackupProviders(providers string) []druidv1alpha1.StorageProvider {
	if providers == "" {
		return []druidv1alpha1.StorageProvider{"none"}
	}

	providerList := make([]druidv1alpha1.StorageProvider, 0)
	for _, p := range strings.Split(providers, ",") {
		provider := druidv1alpha1.StorageProvider(p)
		if slices.Contains(knownBackupProviders, provider) && !slices.Contains(providerList, provider) {
			providerList = append(providerList, provider)
		}
	}

	if len(providerList) == 0 {
		providerList = append(providerList, "none")
	}

	return providerList
}
