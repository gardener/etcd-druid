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

package etcd

import (
	"context"
	"fmt"

	"github.com/gardener/etcd-druid/pkg/component"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Interface contains functions for a etcd deployer.
type Interface interface {
	component.Deployer
	// GetDeltaSnapshotLeaseName returns the `Lease` name for delta snapshots.
	GetDeltaSnapshotLeaseName() string
	// GetDeltaSnapshotLeaseName returns the `Lease` name for full snapshots.
	GetFullSnapshotLeaseName() string
}

type etcd struct {
	client    client.Client
	namespace string

	values Values
}

func (e *etcd) GetDeltaSnapshotLeaseName() string {
	return fmt.Sprintf("delta-snapshot-%s", e.values.EtcdName)
}

func (e *etcd) GetFullSnapshotLeaseName() string {
	return fmt.Sprintf("full-snapshot-%s", e.values.EtcdName)
}

func (e *etcd) Deploy(ctx context.Context) error {
	var (
		deltaSnapshotLease = e.emptyLease(e.GetDeltaSnapshotLeaseName())
		fullSnapshotLease  = e.emptyLease(e.GetFullSnapshotLeaseName())
	)

	if err := e.reconcileLease(ctx, deltaSnapshotLease, e.values.BackupEnabled); err != nil {
		return err
	}

	if err := e.reconcileLease(ctx, fullSnapshotLease, e.values.BackupEnabled); err != nil {
		return err
	}

	if err := e.reconcileMemberLease(ctx); err != nil {
		return err
	}

	return nil
}

func (e *etcd) Destroy(_ context.Context) error {
	return nil
}

// New creates a new etcd deployer instance.
func New(c client.Client, namespace string, values Values) Interface {
	return &etcd{
		client:    c,
		namespace: namespace,
		values:    values,
	}
}
