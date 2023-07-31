// Copyright (c) 2021 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

	if err := c.syncPeerService(ctx, peerService); err != nil {
		return err
	}

	return nil
}

func (c *component) Destroy(ctx context.Context) error {
	var (
		clientService = c.emptyService(c.values.ClientServiceName)
		peerService   = c.emptyService(c.values.PeerServiceName)
	)

	if err := client.IgnoreNotFound(c.client.Delete(ctx, clientService)); err != nil {
		return err
	}

	if err := client.IgnoreNotFound(c.client.Delete(ctx, peerService)); err != nil {
		return err
	}

	return nil
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
