// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package role

import (
	"context"

	"github.com/gardener/gardener/pkg/controllerutils"

	gardenercomponent "github.com/gardener/gardener/pkg/component"
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
