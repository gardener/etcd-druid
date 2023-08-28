// Copyright (c) 2023 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package serviceaccount

import (
	"context"

	"github.com/gardener/gardener/pkg/controllerutils"

	gardenercomponent "github.com/gardener/gardener/pkg/component"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type component struct {
	client client.Client
	values *Values
}

func (c component) Deploy(ctx context.Context) error {
	serviceAccount := c.emptyServiceAccount()
	_, err := controllerutils.GetAndCreateOrStrategicMergePatch(ctx, c.client, serviceAccount, func() error {
		serviceAccount.Name = c.values.Name
		serviceAccount.Namespace = c.values.Namespace
		serviceAccount.Labels = c.values.Labels
		serviceAccount.OwnerReferences = []metav1.OwnerReference{c.values.OwnerReference}
		serviceAccount.AutomountServiceAccountToken = pointer.Bool(!c.values.DisableAutomount)
		return nil
	})
	return err
}

func (c component) Destroy(ctx context.Context) error {
	return client.IgnoreNotFound(c.client.Delete(ctx, c.emptyServiceAccount()))
}

func (c component) emptyServiceAccount() *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      c.values.Name,
			Namespace: c.values.Namespace,
		},
	}
}

// New creates a new service account deployer instance.
func New(c client.Client, value *Values) gardenercomponent.Deployer {
	return &component{
		client: c,
		values: value,
	}
}
