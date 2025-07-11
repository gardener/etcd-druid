// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"context"
	"errors"

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

// CreateBackupProviderLocalSecret creates a secret for the local backup provider using default hostPath.
func CreateBackupProviderLocalSecret(ctx context.Context, c client.Client, name, namespace string) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"hostPath": []byte("/etc/gardener/local-backupbuckets"),
		},
	}
	if err := c.Create(ctx, secret); err != nil {
		if apierrors.IsAlreadyExists(err) {
			return nil
		}
		return err
	}
	return nil
}
