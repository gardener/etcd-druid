// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/store"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var knownBackupProviders = []druidv1alpha1.StorageProvider{
	"none",
	store.Local, druidv1alpha1.StorageProvider(strings.ToLower(store.Local)),
	store.S3, druidv1alpha1.StorageProvider(strings.ToLower(store.S3)),
	store.ABS, druidv1alpha1.StorageProvider(strings.ToLower(store.ABS)),
	store.GCS, druidv1alpha1.StorageProvider(strings.ToLower(store.GCS)),
}

// getKubeconfig returns a Kubeconfig built from the config file in the specified path.
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
func ParseBackupProviders(providers string) ([]druidv1alpha1.StorageProvider, error) {
	var (
		providerList []druidv1alpha1.StorageProvider
		errs         error
	)

	if providers != "" {
		for _, p := range strings.Split(providers, ",") {
			provider := druidv1alpha1.StorageProvider("none")
			if p != "none" {
				prov, err := store.StorageProviderFromInfraProvider(ptr.To(druidv1alpha1.StorageProvider(strings.TrimSpace(p))))
				if err != nil {
					errs = errors.Join(errs, fmt.Errorf("failed to parse provider %s: %w", p, err))
				}
				provider = druidv1alpha1.StorageProvider(prov)
			}
			if slices.Contains(knownBackupProviders, provider) && !slices.Contains(providerList, provider) {
				providerList = append(providerList, provider)
			}
		}
	}

	if len(providerList) == 0 {
		providerList = append(providerList, "none")
	}

	return providerList, nil
}
