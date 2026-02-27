// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"context"
	"errors"
	"fmt"
	"os"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/store"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CreateSecrets creates all the given secrets
func CreateSecrets(ctx context.Context, c client.Client, namespace string, secrets ...string) error {
	var createErrs error
	for _, name := range secrets {
		secret := corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Data: map[string][]byte{
				"test": []byte("test"),
			},
		}
		if err := c.Create(ctx, &secret); err != nil {
			if apierrors.IsAlreadyExists(err) {
				continue
			}
			createErrs = errors.Join(createErrs, err)
		}
	}
	return createErrs
}

// GenerateBackupSecretData generates backup secret data based on the specified provider.
func GenerateBackupSecretData(provider druidv1alpha1.StorageProvider) (map[string][]byte, error) {
	switch provider {
	case store.Local:
		return map[string][]byte{
			"hostPath": []byte(store.LocalProviderDefaultMountPath),
		}, nil

	case store.S3:
		awsAccessKeyID, err := GetEnvOrError("AWS_ACCESS_KEY_ID")
		if err != nil {
			return nil, err
		}
		awsSecretAccessKey, err := GetEnvOrError("AWS_SECRET_ACCESS_KEY")
		if err != nil {
			return nil, err
		}
		awsRegion, err := GetEnvOrError("AWS_REGION")
		if err != nil {
			return nil, err
		}
		localstackHost := GetEnvOrDefault("LOCALSTACK_HOST", "")

		data := map[string][]byte{
			"accessKeyID":     []byte(awsAccessKeyID),
			"secretAccessKey": []byte(awsSecretAccessKey),
			"region":          []byte(awsRegion),
		}
		if localstackHost != "" {
			data["s3ForcePathStyle"] = []byte("true")
		}
		return data, nil

	case store.ABS:
		azureStorageAccountName, err := GetEnvOrError("AZ_STORAGE_ACCOUNT_NAME")
		if err != nil {
			return nil, err
		}
		azureStorageAccountKey, err := GetEnvOrError("AZ_STORAGE_ACCOUNT_KEY")
		if err != nil {
			return nil, err
		}

		data := map[string][]byte{
			"storageAccountName": []byte(azureStorageAccountName),
			"storageAccountKey":  []byte(azureStorageAccountKey),
		}
		return data, nil

	case store.GCS:
		gcpServiceAccountJSONPath, err := GetEnvOrError("GCP_SERVICEACCOUNT_JSON_PATH")
		if err != nil {
			return nil, err
		}

		gcsSA, err := os.ReadFile(gcpServiceAccountJSONPath) // #nosec: G304 -- test code reading local file
		if err != nil {
			return nil, fmt.Errorf("failed to read GCP service account JSON file: %w", err)
		}
		return map[string][]byte{
			"serviceaccount.json": gcsSA,
		}, nil
	}
	return nil, fmt.Errorf("unsupported backup provider")
}

// CreateBackupSecret creates a backup secret based on the specified provider.
func CreateBackupSecret(ctx context.Context, c client.Client, name, namespace string, provider druidv1alpha1.StorageProvider) error {
	if provider == "none" {
		return nil
	}

	secretData, err := GenerateBackupSecretData(provider)
	if err != nil {
		return fmt.Errorf("failed to generate backup secret: %w", err)
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: secretData,
	}
	if err := c.Create(ctx, secret); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return nil
		}
		return err
	}
	return nil
}
