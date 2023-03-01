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

package predicate

import (
	"context"
	"reflect"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/gardener/gardener/pkg/api/extensions"
	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	v1beta1helper "github.com/gardener/gardener/pkg/apis/core/v1beta1/helper"
	resourcesv1alpha1 "github.com/gardener/gardener/pkg/apis/resources/v1alpha1"
	contextutils "github.com/gardener/gardener/pkg/utils/context"
	gardenerutils "github.com/gardener/gardener/pkg/utils/gardener"
)

// IsDeleting is a predicate for objects having a deletion timestamp.
func IsDeleting() predicate.Predicate {
	return predicate.NewPredicateFuncs(func(obj client.Object) bool {
		return obj.GetDeletionTimestamp() != nil
	})
}

// HasName returns a predicate which returns true when the object has the provided name.
func HasName(name string) predicate.Predicate {
	return predicate.NewPredicateFuncs(func(obj client.Object) bool {
		return obj.GetName() == name
	})
}

// EventType is an alias for byte.
type EventType byte

const (
	// Create is a constant for an event of type 'create'.
	Create EventType = iota
	// Update is a constant for an event of type 'update'.
	Update
	// Delete is a constant for an event of type 'delete'.
	Delete
	// Generic is a constant for an event of type 'generic'.
	Generic
)

// ForEventTypes is a predicate which returns true only for the provided event types.
func ForEventTypes(events ...EventType) predicate.Predicate {
	has := func(event EventType) bool {
		for _, e := range events {
			if e == event {
				return true
			}
		}
		return false
	}

	return predicate.Funcs{
		CreateFunc:  func(e event.CreateEvent) bool { return has(Create) },
		UpdateFunc:  func(e event.UpdateEvent) bool { return has(Update) },
		DeleteFunc:  func(e event.DeleteEvent) bool { return has(Delete) },
		GenericFunc: func(e event.GenericEvent) bool { return has(Generic) },
	}
}

// EvalGeneric returns true if all predicates match for the given object.
func EvalGeneric(obj client.Object, predicates ...predicate.Predicate) bool {
	e := event.GenericEvent{Object: obj}
	for _, p := range predicates {
		if !p.Generic(e) {
			return false
		}
	}

	return true
}

// RelevantConditionsChanged returns true for all events except for 'UPDATE'. Here, true is only returned when the
// status, reason or message of a relevant condition has changed.
func RelevantConditionsChanged(
	getConditionsFromObject func(obj client.Object) []gardencorev1beta1.Condition,
	relevantConditionTypes ...gardencorev1beta1.ConditionType,
) predicate.Predicate {
	wasConditionStatusReasonOrMessageUpdated := func(oldCondition, newCondition *gardencorev1beta1.Condition) bool {
		return (oldCondition == nil && newCondition != nil) ||
			(oldCondition != nil && newCondition == nil) ||
			(oldCondition != nil && newCondition != nil &&
				(oldCondition.Status != newCondition.Status || oldCondition.Reason != newCondition.Reason || oldCondition.Message != newCondition.Message))
	}

	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			var (
				oldConditions = getConditionsFromObject(e.ObjectOld)
				newConditions = getConditionsFromObject(e.ObjectNew)
			)

			for _, condition := range relevantConditionTypes {
				if wasConditionStatusReasonOrMessageUpdated(
					v1beta1helper.GetCondition(oldConditions, condition),
					v1beta1helper.GetCondition(newConditions, condition),
				) {
					return true
				}
			}

			return false
		},
	}
}

// ManagedResourceConditionsChanged returns a predicate which returns true if the status/reason/message of the
// Resources{Applied,Healthy,Progressing} condition of the ManagedResource changes.
func ManagedResourceConditionsChanged() predicate.Predicate {
	return RelevantConditionsChanged(
		func(obj client.Object) []gardencorev1beta1.Condition {
			managedResource, ok := obj.(*resourcesv1alpha1.ManagedResource)
			if !ok {
				return nil
			}
			return managedResource.Status.Conditions
		},
		resourcesv1alpha1.ResourcesApplied,
		resourcesv1alpha1.ResourcesHealthy,
		resourcesv1alpha1.ResourcesProgressing,
	)
}

// ExtensionStatusChanged returns a predicate which returns true when the status of the extension object has changed.
func ExtensionStatusChanged() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			// If the object has the operation annotation reconcile, this means it's not picked up by the extension controller.
			// For restore and migrate operations, we remove the annotation only at the end, so we don't stop enqueueing it.
			if e.Object.GetAnnotations()[v1beta1constants.GardenerOperation] == v1beta1constants.GardenerOperationReconcile {
				return false
			}

			// If lastOperation State is failed then we admit reconciliation.
			// This is not possible during create but possible during a controller restart.
			return lastOperationStateFailed(e.Object)
		},

		UpdateFunc: func(e event.UpdateEvent) bool {
			// If the object has the operation annotation, this means it's not picked up by the extension controller.
			// migrate and restore annotations are removed for the extensions only at the end of the operation,
			// so if the oldObject doesn't have the same annotation, don't enqueue it.
			if v1beta1helper.HasOperationAnnotation(e.ObjectNew.GetAnnotations()) {
				operation := e.ObjectNew.GetAnnotations()[v1beta1constants.GardenerOperation]
				if operation == v1beta1constants.GardenerOperationMigrate || operation == v1beta1constants.GardenerOperationRestore {
					// if the oldObject doesn't have the same annotation skip
					if e.ObjectOld.GetAnnotations()[v1beta1constants.GardenerOperation] != operation {
						return false
					}
				} else {
					return false
				}
			}

			// If lastOperation State has changed to Succeeded or Error then we admit reconciliation.
			return lastOperationStateChanged(e.ObjectOld, e.ObjectNew)
		},

		DeleteFunc:  func(event.DeleteEvent) bool { return false },
		GenericFunc: func(event.GenericEvent) bool { return false },
	}
}

func lastOperationStateFailed(obj client.Object) bool {
	acc, err := extensions.Accessor(obj)
	if err != nil {
		return false
	}

	if acc.GetExtensionStatus().GetLastOperation() == nil {
		return false
	}

	return acc.GetExtensionStatus().GetLastOperation().State == gardencorev1beta1.LastOperationStateFailed
}

func lastOperationStateChanged(oldObj, newObj client.Object) bool {
	newAcc, err := extensions.Accessor(newObj)
	if err != nil {
		return false
	}

	oldAcc, err := extensions.Accessor(oldObj)
	if err != nil {
		return false
	}

	if newAcc.GetExtensionStatus().GetLastOperation() == nil {
		return false
	}

	lastOperationState := newAcc.GetExtensionStatus().GetLastOperation().State
	newLastOperationStateSucceededOrErroneous := lastOperationState == gardencorev1beta1.LastOperationStateSucceeded || lastOperationState == gardencorev1beta1.LastOperationStateError || lastOperationState == gardencorev1beta1.LastOperationStateFailed

	if newLastOperationStateSucceededOrErroneous {
		if oldAcc.GetExtensionStatus().GetLastOperation() != nil {
			return !reflect.DeepEqual(oldAcc.GetExtensionStatus().GetLastOperation(), newAcc.GetExtensionStatus().GetLastOperation())
		}
		return true
	}

	return false
}

// IsBeingMigratedPredicate returns a predicate which returns true for objects that are being migrated to a different
// seed cluster.
func IsBeingMigratedPredicate(reader client.Reader, seedName string, getSeedNamesFromObject func(client.Object) (*string, *string)) predicate.Predicate {
	return &isBeingMigratedPredicate{
		reader:                 reader,
		seedName:               seedName,
		getSeedNamesFromObject: getSeedNamesFromObject,
	}
}

type isBeingMigratedPredicate struct {
	ctx                    context.Context
	reader                 client.Reader
	seedName               string
	getSeedNamesFromObject func(client.Object) (*string, *string)
}

func (p *isBeingMigratedPredicate) InjectStopChannel(stopChan <-chan struct{}) error {
	p.ctx = contextutils.FromStopChannel(stopChan)
	return nil
}

// IsObjectBeingMigrated is an alias for gardenerutils.IsObjectBeingMigrated.
var IsObjectBeingMigrated = gardenerutils.IsObjectBeingMigrated

func (p *isBeingMigratedPredicate) Create(e event.CreateEvent) bool {
	return IsObjectBeingMigrated(p.ctx, p.reader, e.Object, p.seedName, p.getSeedNamesFromObject)
}

func (p *isBeingMigratedPredicate) Update(e event.UpdateEvent) bool {
	return IsObjectBeingMigrated(p.ctx, p.reader, e.ObjectNew, p.seedName, p.getSeedNamesFromObject)
}

func (p *isBeingMigratedPredicate) Delete(e event.DeleteEvent) bool {
	return IsObjectBeingMigrated(p.ctx, p.reader, e.Object, p.seedName, p.getSeedNamesFromObject)
}

func (p *isBeingMigratedPredicate) Generic(e event.GenericEvent) bool {
	return IsObjectBeingMigrated(p.ctx, p.reader, e.Object, p.seedName, p.getSeedNamesFromObject)
}

// SeedNamePredicate returns a predicate which returns true for objects that are being migrated to a different
// seed cluster.
func SeedNamePredicate(seedName string, getSeedNamesFromObject func(client.Object) (*string, *string)) predicate.Predicate {
	return predicate.NewPredicateFuncs(func(obj client.Object) bool {
		specSeedName, statusSeedName := getSeedNamesFromObject(obj)
		return gardenerutils.GetResponsibleSeedName(specSeedName, statusSeedName) == seedName
	})
}
