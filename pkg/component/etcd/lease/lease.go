// Copyright (c) 2021 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package lease

import (
	"context"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/pkg/component"
	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
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

type leases struct {
	client    client.Client
	namespace string

	values Values
}

func (l *leases) Deploy(ctx context.Context) error {
	var (
		deltaSnapshotLease = l.emptyLease(l.GetDeltaSnapshotLeaseName())
		fullSnapshotLease  = l.emptyLease(l.GetFullSnapshotLeaseName())
	)

	if err := l.syncSnapshotLease(ctx, deltaSnapshotLease, l.values.BackupEnabled); err != nil {
		return err
	}

	if err := l.syncSnapshotLease(ctx, fullSnapshotLease, l.values.BackupEnabled); err != nil {
		return err
	}

	if err := l.syncMemberLeases(ctx); err != nil {
		return err
	}

	return nil
}

func (l *leases) Destroy(_ context.Context) error {
	return nil
}

// New creates a new etcd deployer instance.
func New(c client.Client, namespace string, values Values) Interface {
	return &leases{
		client:    c,
		namespace: namespace,
		values:    values,
	}
}

func (l *leases) emptyLease(name string) *coordinationv1.Lease {
	return &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: l.namespace,
		},
	}
}

func getOwnerReferences(val Values) []metav1.OwnerReference {
	return []metav1.OwnerReference{
		{
			APIVersion:         druidv1alpha1.GroupVersion.String(),
			Kind:               "etcd",
			Name:               val.EtcdName,
			UID:                val.EtcdUID,
			Controller:         pointer.BoolPtr(true),
			BlockOwnerDeletion: pointer.BoolPtr(true),
		},
	}
}
