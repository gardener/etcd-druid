// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"fmt"

	"github.com/gardener/etcd-druid/druidctl/internal/client"
)

// ClientBundle provides lazy-loaded clients to improve performance
type ClientBundle struct {
	factory    client.Factory
	etcdClient client.EtcdClientInterface
	genClient  client.GenericClientInterface
}

// NewClientBundle creates a new ClientBundle with the given factory
func NewClientBundle(factory client.Factory) *ClientBundle {
	return &ClientBundle{factory: factory}
}

// EtcdClient returns the etcd client, creating it if necessary
func (c *ClientBundle) EtcdClient() (client.EtcdClientInterface, error) {
	if c.etcdClient == nil {
		var err error
		c.etcdClient, err = c.factory.CreateEtcdClient()
		if err != nil {
			return nil, fmt.Errorf("failed to create etcd client: %w", err)
		}
	}
	return c.etcdClient, nil
}

// GenericClient returns the generic client, creating it if necessary
func (c *ClientBundle) GenericClient() (client.GenericClientInterface, error) {
	if c.genClient == nil {
		var err error
		c.genClient, err = c.factory.CreateGenericClient()
		if err != nil {
			return nil, fmt.Errorf("failed to create generic client: %w", err)
		}
	}
	return c.genClient, nil
}
