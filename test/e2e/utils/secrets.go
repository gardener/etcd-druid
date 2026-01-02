// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"context"
	"os"
	"path/filepath"

	testutils "github.com/gardener/etcd-druid/test/utils"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// createSecret creates a secret in the specified namespace from the given cert and key files.
func createSecret(ctx context.Context, cl client.Client, name, namespace, certsDir, certFileName, keyFileName, certDataKey, keyDataKey string) error {
	cert, err := os.ReadFile(filepath.Join(certsDir, certFileName)) // #nosec: G304 -- test files.
	if err != nil {
		return err
	}
	key, err := os.ReadFile(filepath.Join(certsDir, keyFileName)) // #nosec: G304 -- test files.
	if err != nil {
		return err
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			certDataKey: cert,
			keyDataKey:  key,
		},
	}
	return cl.Create(ctx, secret)
}

func CreateCASecret(ctx context.Context, cl client.Client, name, namespace, certsDir string) error {
	return createSecret(ctx, cl, name, namespace, certsDir, testutils.CACertFileName, testutils.CAKeyFileName, "ca.crt", "ca.key")
}

func CreateServerTLSSecret(ctx context.Context, cl client.Client, name, namespace, certsDir string) error {
	return createSecret(ctx, cl, name, namespace, certsDir, testutils.ServerCertFileName, testutils.ServerKeyFileName, "tls.crt", "tls.key")
}

func CreateClientTLSSecret(ctx context.Context, cl client.Client, name, namespace, certsDir string) error {
	return createSecret(ctx, cl, name, namespace, certsDir, testutils.ClientCertFileName, testutils.ClientKeyFileName, "tls.crt", "tls.key")
}

// CreateAllSecrets creates all secrets (CA, server, and client) in the specified namespace from the given certificate directory.
func CreateAllSecrets(ctx context.Context, cl client.Client, name, namespace, certsDir string) error {
	if err := CreateCASecret(ctx, cl, name, namespace, certsDir); err != nil {
		return err
	}
	if err := CreateServerTLSSecret(ctx, cl, name, namespace, certsDir); err != nil {
		return err
	}
	if err := CreateClientTLSSecret(ctx, cl, name, namespace, certsDir); err != nil {
		return err
	}
	return nil
}
