// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"

	gardenercomponent "github.com/gardener/gardener/pkg/component"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type component struct {
	client    client.Client
	namespace string

	values Values
}

func (c *component) Deploy(ctx context.Context) error {
	var (
		clientService = c.emptyService(c.values.ClientServiceName)
		peerService   = c.emptyService(c.values.PeerServiceName)
	)

	if err := c.syncClientService(ctx, clientService); err != nil {
		return err
	}

	return c.syncPeerService(ctx, peerService)
}

func (c *component) Destroy(ctx context.Context) error {
	var (
		clientService = c.emptyService(c.values.ClientServiceName)
		peerService   = c.emptyService(c.values.PeerServiceName)
	)

	if err := client.IgnoreNotFound(c.client.Delete(ctx, clientService)); err != nil {
		return err
	}

	return client.IgnoreNotFound(c.client.Delete(ctx, peerService))
}

// New creates a new service deployer instance.
func New(c client.Client, namespace string, values Values) gardenercomponent.Deployer {
	return &component{
		client:    c,
		namespace: namespace,
		values:    values,
	}
}

func (c *component) emptyService(name string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: c.namespace,
		},
	}
}
