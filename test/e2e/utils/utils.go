// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"fmt"
	"os"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func GetEnvOrError(key string) (string, error) {
	if value, ok := os.LookupEnv(key); ok {
		return value, nil
	}

	return "", fmt.Errorf("environment variable not found: %s", key)
}

const (
	providerLocal = "local"
)

func getKubeconfig(kubeconfigPath string) (*rest.Config, error) {
	return clientcmd.BuildConfigFromFlags("", kubeconfigPath)
}

func GetKubernetesClient(kubeconfigPath string) (client.Client, error) {
	config, err := getKubeconfig(kubeconfigPath)
	if err != nil {
		return nil, err
	}

	return client.New(config, client.Options{})
}
