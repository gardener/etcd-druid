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
		err := c.Create(ctx, &secret)
		if apierrors.IsAlreadyExists(err) {
			continue
		}
		if err != nil {
			createErrs = errors.Join(createErrs, err)
		}
	}
	return createErrs
}
