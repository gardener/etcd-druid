package utils

import (
	"context"
	"os"
	"path/filepath"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// createSecret creates a secret in the specified namespace from the given cert and key files.
func createSecret(ctx context.Context, cl client.Client, name, namespace, certsDir, certFileName, keyFileName, certDataKey, keyDataKey string) error {
	cert, err := os.ReadFile(filepath.Join(certsDir, certFileName))
	if err != nil {
		return err
	}
	key, err := os.ReadFile(filepath.Join(certsDir, keyFileName))
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
	return createSecret(ctx, cl, name, namespace, certsDir, caCertFileName, caKeyFileName, "ca.crt", "ca.key")
}

func CreateServerTLSSecret(ctx context.Context, cl client.Client, name, namespace, certsDir string) error {
	return createSecret(ctx, cl, name, namespace, certsDir, serverCertFileName, serverKeyFileName, "tls.crt", "tls.key")
}

func CreateClientTLSSecret(ctx context.Context, cl client.Client, name, namespace, certsDir string) error {
	return createSecret(ctx, cl, name, namespace, certsDir, clientCertFileName, clientKeyFileName, "tls.crt", "tls.key")
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
