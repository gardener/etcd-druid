// Copyright (c) 2020 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package secrets

import (
	"context"
	"fmt"

	gardenerkubernetes "github.com/gardener/gardener/pkg/client/kubernetes"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Interface represents a set of secrets that can be deployed and deleted.
type Interface interface {
	// Deploy generates and deploys the secrets into the given namespace, taking into account existing secrets.
	Deploy(context.Context, kubernetes.Interface, gardenerkubernetes.Interface, string) (map[string]*corev1.Secret, error)
	// Delete deletes the secrets from the given namespace.
	Delete(context.Context, kubernetes.Interface, string) error
}

// Secrets represents a set of secrets that can be deployed and deleted.
type Secrets struct {
	CertificateSecretConfigs map[string]*CertificateSecretConfig
	SecretConfigsFunc        func(map[string]*Certificate, string) []ConfigInterface
}

// Deploy generates and deploys the secrets into the given namespace, taking into account existing secrets.
func (s *Secrets) Deploy(
	ctx context.Context,
	cs kubernetes.Interface,
	gcs gardenerkubernetes.Interface,
	namespace string,
) (
	map[string]*corev1.Secret,
	error,
) {
	// Get existing secrets in the namespace
	existingSecrets, err := getSecrets(ctx, cs, namespace)
	if err != nil {
		return nil, err
	}

	// Generate CAs
	_, cas, err := GenerateCertificateAuthorities(ctx, gcs.Client(), existingSecrets, s.CertificateSecretConfigs, namespace)
	if err != nil {
		return nil, fmt.Errorf("could not generate CA secrets in namespace '%s': %w", namespace, err)
	}

	// Generate cluster secrets
	secretConfigs := s.SecretConfigsFunc(cas, namespace)
	clusterSecrets, err := GenerateClusterSecrets(ctx, gcs.Client(), existingSecrets, secretConfigs, namespace)
	if err != nil {
		return nil, fmt.Errorf("could not generate cluster secrets in namespace '%s': %w", namespace, err)
	}

	return clusterSecrets, nil
}

// Delete deletes the secrets from the given namespace.
func (s *Secrets) Delete(ctx context.Context, cs kubernetes.Interface, namespace string) error {
	for _, sc := range s.SecretConfigsFunc(nil, namespace) {
		if err := deleteSecret(ctx, cs, namespace, sc.GetName()); err != nil {
			return err
		}
	}
	return nil
}

func getSecrets(ctx context.Context, cs kubernetes.Interface, namespace string) (map[string]*corev1.Secret, error) {
	secretList, err := cs.CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("could not list secrets in namespace '%s': %w", namespace, err)
	}
	result := make(map[string]*corev1.Secret, len(secretList.Items))
	for _, secret := range secretList.Items {
		func(secret corev1.Secret) {
			result[secret.Name] = &secret
		}(secret)
	}
	return result, nil
}

func deleteSecret(ctx context.Context, cs kubernetes.Interface, namespace, name string) error {
	return cs.CoreV1().Secrets(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}
