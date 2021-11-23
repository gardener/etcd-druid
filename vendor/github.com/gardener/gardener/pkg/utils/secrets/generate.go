// Copyright (c) 2018 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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
	"sync"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GenerateClusterSecrets try to deploy in the k8s cluster each secret in the wantedSecretsList. If the secret already exist it jumps to the next one.
// The function returns a map with all of the successfully deployed wanted secrets plus those already deployed (only from the wantedSecretsList).
func GenerateClusterSecrets(ctx context.Context, c client.Client, existingSecretsMap map[string]*corev1.Secret, wantedSecretsList []ConfigInterface, namespace string) (map[string]*corev1.Secret, error) {
	return GenerateClusterSecretsWithFunc(ctx, c, existingSecretsMap, wantedSecretsList, namespace, func(s ConfigInterface) (DataInterface, error) {
		return s.Generate()
	})
}

// GenerateClusterSecretsWithFunc will try to deploy in the k8s cluster each secret in the wantedSecretsList. If the secret already exist it jumps to the next one.
// The function will used the SecretsGeneratorFunc to create the secret DataInterface from the wantedSecret configs.
func GenerateClusterSecretsWithFunc(ctx context.Context, k8sClusterClient client.Client, existingSecretsMap map[string]*corev1.Secret, wantedSecretsList []ConfigInterface, namespace string, SecretsGeneratorFunc func(s ConfigInterface) (DataInterface, error)) (map[string]*corev1.Secret, error) {
	type secretOutput struct {
		secret *corev1.Secret
		err    error
	}

	var (
		results                = make(chan *secretOutput)
		deployedClusterSecrets = map[string]*corev1.Secret{}
		wg                     sync.WaitGroup
		errorList              = []error{}
	)

	for _, s := range wantedSecretsList {
		name := s.GetName()

		if existingSecret, ok := existingSecretsMap[name]; ok {
			deployedClusterSecrets[name] = existingSecret
			continue
		}

		wg.Add(1)
		go func(s ConfigInterface) {
			defer wg.Done()

			obj, err := SecretsGeneratorFunc(s)
			if err != nil {
				results <- &secretOutput{err: err}
				return
			}

			secretType := corev1.SecretTypeOpaque
			if _, isTLSSecret := obj.(*Certificate); isTLSSecret {
				secretType = corev1.SecretTypeTLS
			}

			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      s.GetName(),
					Namespace: namespace,
				},
				Type: secretType,
				Data: obj.SecretData(),
			}
			err = k8sClusterClient.Create(ctx, secret)
			results <- &secretOutput{secret: secret, err: err}
		}(s)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	for out := range results {
		if out.err != nil {
			errorList = append(errorList, out.err)
			continue
		}

		deployedClusterSecrets[out.secret.Name] = out.secret
	}

	// Wait and check whether an error occurred during the parallel processing of the Secret creation.
	if len(errorList) > 0 {
		return deployedClusterSecrets, fmt.Errorf("errors occurred during shoot secrets generation: %+v", errorList)
	}

	return deployedClusterSecrets, nil
}
