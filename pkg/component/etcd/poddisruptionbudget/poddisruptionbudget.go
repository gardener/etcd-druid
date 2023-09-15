// Copyright (c) 2022 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file.
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

package poddisruptionbudget

import (
	"context"

	gardenercomponent "github.com/gardener/gardener/pkg/component"
	"github.com/gardener/gardener/pkg/controllerutils"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type component struct {
	client    client.Client
	namespace string
	values    *Values
}

// New creates a new poddisruptionbudget deployer instance.
func New(c client.Client, namespace string, values *Values) gardenercomponent.Deployer {
	return &component{
		client:    c,
		namespace: namespace,
		values:    values,
	}
}

// emptyPodDisruptionBudget returns an empty PDB object with only the name and namespace as part of the object meta
func (c *component) emptyPodDisruptionBudget() *policyv1.PodDisruptionBudget {
	return &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      c.values.Name,
			Namespace: c.namespace,
		},
	}
}

// Deploy creates a PDB or synchronizes the PDB spec based on the etcd spec
func (c *component) Deploy(ctx context.Context) error {
	pdb := c.emptyPodDisruptionBudget()

	return c.syncPodDisruptionBudget(ctx, pdb)
}

// syncPodDisruptionBudget Creates a PDB if it does not exist
// Patches the PDB spec if a PDB already exist
func (c *component) syncPodDisruptionBudget(ctx context.Context, pdb *policyv1.PodDisruptionBudget) error {
	_, err := controllerutils.GetAndCreateOrMergePatch(ctx, c.client, pdb, func() error {
		pdb.Labels = c.values.Labels
		pdb.Annotations = c.values.Annotations
		pdb.OwnerReferences = []metav1.OwnerReference{c.values.OwnerReference}
		pdb.Spec.MinAvailable = &intstr.IntOrString{
			IntVal: c.values.MinAvailable,
			Type:   intstr.Int,
		}
		pdb.Spec.Selector = &metav1.LabelSelector{
			MatchLabels: c.values.SelectorLabels,
		}
		return nil
	})
	return err
}

// Destroy deletes a PDB. Ignores if PDB does not exist
func (c *component) Destroy(ctx context.Context) error {
	pdb := c.emptyPodDisruptionBudget()

	return client.IgnoreNotFound(c.client.Delete(ctx, pdb))
}
