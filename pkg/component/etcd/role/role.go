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

package role

import (
	"context"

	"github.com/gardener/gardener/pkg/controllerutils"

	gardenercomponent "github.com/gardener/gardener/pkg/operation/botanist/component"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type component struct {
	client client.Client
	values *Values
}

func (c component) Deploy(ctx context.Context) error {
	role := c.emptyRole()
	_, err := controllerutils.GetAndCreateOrStrategicMergePatch(ctx, c.client, role, func() error {
		role.Name = c.values.Name
		role.Namespace = c.values.Namespace
		role.Labels = c.values.Labels
		role.OwnerReferences = []metav1.OwnerReference{c.values.OwnerReference}
		role.Rules = c.values.Rules
		return nil
	})
	return err
}

func (c component) Destroy(ctx context.Context) error {
	return client.IgnoreNotFound(c.client.Delete(ctx, c.emptyRole()))
}

func (c component) emptyRole() *rbacv1.Role {
	return &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      c.values.Name,
			Namespace: c.values.Namespace,
		},
	}
}

// New creates a new role deployer instance.
func New(c client.Client, values *Values) gardenercomponent.Deployer {
	return &component{
		client: c,
		values: values,
	}
}
