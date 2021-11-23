// Copyright (c) 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package predicate

import (
	"context"
	"errors"

	extensionscontroller "github.com/gardener/gardener/extensions/pkg/controller"
	gardencore "github.com/gardener/gardener/pkg/api/core"
	"github.com/gardener/gardener/pkg/api/extensions"
	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	extensionsv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	predicateutils "github.com/gardener/gardener/pkg/controllerutils/predicate"
	contextutil "github.com/gardener/gardener/pkg/utils/context"
	"github.com/gardener/gardener/pkg/utils/version"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// Log is the logger for predicates.
var Log logr.Logger = log.Log

type shootNotFailedMapper struct {
	ctx    context.Context
	log    logr.Logger
	client client.Client
	cache  cache.Cache
}

func (s *shootNotFailedMapper) InjectClient(c client.Client) error {
	s.client = c
	return nil
}

func (s *shootNotFailedMapper) InjectStopChannel(stopChan <-chan struct{}) error {
	s.ctx = contextutil.FromStopChannel(stopChan)
	return nil
}

func (s *shootNotFailedMapper) InjectCache(cache cache.Cache) error {
	s.cache = cache
	return nil
}

func (s *shootNotFailedMapper) Map(e event.GenericEvent) bool {
	// Return true for resources in the garden namespace, as they are not associated with a shoot
	if e.Object.GetNamespace() == v1beta1constants.GardenNamespace {
		return true
	}

	// Wait for cache sync because of backing client cache.
	if !s.cache.WaitForCacheSync(s.ctx) {
		err := errors.New("failed to wait for caches to sync")
		s.log.Error(err, "Could not wait for Cache to sync", "predicate", "ShootNotFailed")
		return false
	}

	cluster, err := extensionscontroller.GetCluster(s.ctx, s.client, e.Object.GetNamespace())
	if err != nil {
		s.log.Error(err, "Could not retrieve corresponding cluster")
		return false
	}

	if extensionscontroller.IsFailed(cluster) {
		return cluster.Shoot.Generation != cluster.Shoot.Status.ObservedGeneration
	}

	return true
}

// ShootNotFailed is a predicate for failed shoots.
func ShootNotFailed() predicate.Predicate {
	return predicateutils.FromMapper(&shootNotFailedMapper{log: Log.WithName("shoot-not-failed")},
		predicateutils.CreateTrigger, predicateutils.UpdateNewTrigger, predicateutils.DeleteTrigger, predicateutils.GenericTrigger)
}

// HasType filters the incoming OperatingSystemConfigs for ones that have the same type
// as the given type.
func HasType(typeName string) predicate.Predicate {
	return predicateutils.FromMapper(predicateutils.MapperFunc(func(e event.GenericEvent) bool {
		acc, err := extensions.Accessor(e.Object)
		if err != nil {
			return false
		}

		return acc.GetExtensionSpec().GetExtensionType() == typeName
	}), predicateutils.CreateTrigger, predicateutils.UpdateNewTrigger, predicateutils.DeleteTrigger, predicateutils.GenericTrigger)
}

// LastOperationNotSuccessful is a predicate for unsuccessful last operations **only** for creation events.
func LastOperationNotSuccessful() predicate.Predicate {
	operationNotSucceeded := func(obj client.Object) bool {
		acc, err := extensions.Accessor(obj)
		if err != nil {
			return false
		}

		lastOp := acc.GetExtensionStatus().GetLastOperation()
		return lastOp == nil ||
			lastOp.State != gardencorev1beta1.LastOperationStateSucceeded
	}

	return predicate.Funcs{
		CreateFunc: func(event event.CreateEvent) bool {
			return operationNotSucceeded(event.Object)
		},
		UpdateFunc: func(event event.UpdateEvent) bool {
			return false
		},
		GenericFunc: func(event event.GenericEvent) bool {
			return false
		},
		DeleteFunc: func(event event.DeleteEvent) bool {
			return false
		},
	}
}

// IsDeleting is an alias for a predicate which checks if the passed object has a deletion timestamp.
var IsDeleting = predicateutils.IsDeleting

// AddTypePredicate returns a new slice which contains a type predicate and the given `predicates`.
// if more than one extensionTypes is given all given types are or combined
func AddTypePredicate(predicates []predicate.Predicate, extensionTypes ...string) []predicate.Predicate {
	resultPredicates := make([]predicate.Predicate, 0, len(predicates)+1)
	resultPredicates = append(resultPredicates, predicates...)

	if len(extensionTypes) == 1 {
		resultPredicates = append(resultPredicates, HasType(extensionTypes[0]))
		return resultPredicates
	}

	orPreds := make([]predicate.Predicate, 0, len(extensionTypes))
	for _, extensionType := range extensionTypes {
		orPreds = append(orPreds, HasType(extensionType))
	}

	return append(resultPredicates, predicate.Or(orPreds...))
}

// HasPurpose filters the incoming Controlplanes  for the given spec.purpose
func HasPurpose(purpose extensionsv1alpha1.Purpose) predicate.Predicate {
	return predicateutils.FromMapper(predicateutils.MapperFunc(func(e event.GenericEvent) bool {
		controlPlane, ok := e.Object.(*extensionsv1alpha1.ControlPlane)
		if !ok {
			return false
		}

		// needed because ControlPlane of type "normal" has the spec.purpose field not set
		if controlPlane.Spec.Purpose == nil && purpose == extensionsv1alpha1.Normal {
			return true
		}

		if controlPlane.Spec.Purpose == nil {
			return false
		}

		return *controlPlane.Spec.Purpose == purpose
	}), predicateutils.CreateTrigger, predicateutils.UpdateNewTrigger, predicateutils.DeleteTrigger, predicateutils.GenericTrigger)
}

// ClusterShootProviderType is a predicate for the provider type of the shoot in the cluster resource.
func ClusterShootProviderType(providerType string) predicate.Predicate {
	f := func(obj client.Object) bool {
		if obj == nil {
			return false
		}

		cluster, ok := obj.(*extensionsv1alpha1.Cluster)
		if !ok {
			return false
		}

		shoot, err := extensionscontroller.ShootFromCluster(cluster)
		if err != nil {
			return false
		}

		return shoot.Spec.Provider.Type == providerType
	}

	return predicate.Funcs{
		CreateFunc: func(event event.CreateEvent) bool {
			return f(event.Object)
		},
		UpdateFunc: func(event event.UpdateEvent) bool {
			return f(event.ObjectNew)
		},
		GenericFunc: func(event event.GenericEvent) bool {
			return f(event.Object)
		},
		DeleteFunc: func(event event.DeleteEvent) bool {
			return f(event.Object)
		},
	}
}

// GardenCoreProviderType is a predicate for the provider type of a `gardencore.Object` implementation.
func GardenCoreProviderType(providerType string) predicate.Predicate {
	f := func(obj client.Object) bool {
		if obj == nil {
			return false
		}

		accessor, err := gardencore.Accessor(obj)
		if err != nil {
			return false
		}

		return accessor.GetProviderType() == providerType
	}

	return predicate.Funcs{
		CreateFunc: func(event event.CreateEvent) bool {
			return f(event.Object)
		},
		UpdateFunc: func(event event.UpdateEvent) bool {
			return f(event.ObjectNew)
		},
		GenericFunc: func(event event.GenericEvent) bool {
			return f(event.Object)
		},
		DeleteFunc: func(event event.DeleteEvent) bool {
			return f(event.Object)
		},
	}
}

// ClusterShootKubernetesVersionForCSIMigrationAtLeast is a predicate for the kubernetes version of the shoot in the cluster resource.
func ClusterShootKubernetesVersionForCSIMigrationAtLeast(kubernetesVersion string) predicate.Predicate {
	f := func(obj client.Object) bool {
		if obj == nil {
			return false
		}

		cluster, ok := obj.(*extensionsv1alpha1.Cluster)
		if !ok {
			return false
		}

		shoot, err := extensionscontroller.ShootFromCluster(cluster)
		if err != nil {
			return false
		}

		kubernetesVersionForCSIMigration := kubernetesVersion
		if overwrite, ok := shoot.Annotations[extensionsv1alpha1.ShootAlphaCSIMigrationKubernetesVersion]; ok {
			kubernetesVersionForCSIMigration = overwrite
		}

		constraint, err := version.CompareVersions(shoot.Spec.Kubernetes.Version, ">=", kubernetesVersionForCSIMigration)
		if err != nil {
			return false
		}

		return constraint
	}

	return predicate.Funcs{
		CreateFunc: func(event event.CreateEvent) bool {
			return f(event.Object)
		},
		UpdateFunc: func(event event.UpdateEvent) bool {
			return f(event.ObjectNew)
		},
		GenericFunc: func(event event.GenericEvent) bool {
			return f(event.Object)
		},
		DeleteFunc: func(event event.DeleteEvent) bool {
			return f(event.Object)
		},
	}
}
