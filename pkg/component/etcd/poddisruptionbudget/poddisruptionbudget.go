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

	"github.com/Masterminds/semver"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/pkg/utils"

	"github.com/gardener/gardener/pkg/controllerutils"
	gardenercomponent "github.com/gardener/gardener/pkg/operation/botanist/component"
	policyv1 "k8s.io/api/policy/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type component struct {
	client     client.Client
	namespace  string
	values     *Values
	k8sVersion *semver.Version
}

// New creates a new poddisruptionbudget deployer instance.
func New(c client.Client, namespace string, values *Values, k8sVersion semver.Version) gardenercomponent.Deployer {
	return &component{
		client:     c,
		namespace:  namespace,
		values:     values,
		k8sVersion: &k8sVersion,
	}
}

// getOwnerReferences returns owner references for the PDB object
func getOwnerReferences(val *Values) []metav1.OwnerReference {
	return []metav1.OwnerReference{
		{
			APIVersion:         druidv1alpha1.GroupVersion.String(),
			Kind:               "Etcd",
			Name:               val.EtcdName,
			UID:                val.EtcdUID,
			Controller:         pointer.BoolPtr(true),
			BlockOwnerDeletion: pointer.BoolPtr(true),
		},
	}
}

// emptyPodDisruptionBudget returns an empty PDB object with only the name and namespace as part of the object meta
func (c *component) emptyPodDisruptionBudget(name string, k8sversion *semver.Version) (client.Object, error) {
	k8sVersionGreaterEqual121, err := utils.CompareVersions(k8sversion.String(), ">=", "1.21")
	if err != nil {
		return nil, err
	}
	if k8sVersionGreaterEqual121 {
		return &policyv1.PodDisruptionBudget{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: c.namespace,
			},
		}, nil
	} else {
		//TODO (@aaronfern): remove v1beta1 PDB when k8s clusters less then 1.21 are not supported
		return &policyv1beta1.PodDisruptionBudget{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: c.namespace,
			},
		}, nil
	}
}

// Deploy creates a PDB or synchronizes the PDB spec based on the etcd spec
func (c *component) Deploy(ctx context.Context) error {
	pdb, err := c.emptyPodDisruptionBudget(c.values.EtcdName, c.k8sVersion)
	if err != nil {
		return err
	}
	if err := c.syncPodDisruptionBudget(ctx, pdb); err != nil {
		return err
	}

	return nil
}

// syncPodDisruptionBudget Creates a PDB if it does not exist
// Patches the PDB spec if a PDB already exist
func (c *component) syncPodDisruptionBudget(ctx context.Context, pdb client.Object) error {
	switch pdb := pdb.(type) {
	case *policyv1.PodDisruptionBudget:
		_, err := controllerutils.GetAndCreateOrMergePatch(ctx, c.client, pdb, func() error {
			pdb.Annotations = c.values.Annotations
			pdb.Labels = c.values.Labels
			pdb.OwnerReferences = getOwnerReferences(c.values)
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
	case *policyv1beta1.PodDisruptionBudget:
		_, err := controllerutils.GetAndCreateOrMergePatch(ctx, c.client, pdb, func() error {
			pdb.Annotations = c.values.Annotations
			pdb.Labels = c.values.Labels
			pdb.OwnerReferences = getOwnerReferences(c.values)
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
	return nil
}

// Destroy deletes a PDB. Ignores if PDB does not exist
func (c *component) Destroy(ctx context.Context) error {
	pdb, err := c.emptyPodDisruptionBudget(c.values.EtcdName, c.k8sVersion)
	if err != nil {
		return err
	}
	return client.IgnoreNotFound(c.client.Delete(ctx, pdb))
}
