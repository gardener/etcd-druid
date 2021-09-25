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
	"reflect"

	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	appsv1 "k8s.io/api/apps/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
)

// HasOperationAnnotation is a predicate for the operation annotation.
func HasOperationAnnotation() predicate.Predicate {
	f := func(obj runtime.Object) bool {
		etcd, ok := obj.(*druidv1alpha1.Etcd)
		if !ok {
			return false
		}
		return etcd.Annotations[v1beta1constants.GardenerOperation] == v1beta1constants.GardenerOperationReconcile
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
			return true
		},
	}
}

// LastOperationNotSuccessful is a predicate for unsuccessful last operations for creation events.
func LastOperationNotSuccessful() predicate.Predicate {
	operationNotSucceeded := func(obj runtime.Object) bool {
		etcd, ok := obj.(*druidv1alpha1.Etcd)
		if !ok {
			return false
		}
		if etcd.Status.LastError != nil {
			return true
		}
		return false
	}

	return predicate.Funcs{
		CreateFunc: func(event event.CreateEvent) bool {
			return operationNotSucceeded(event.Object)
		},
		UpdateFunc: func(event event.UpdateEvent) bool {
			return operationNotSucceeded(event.ObjectNew)
		},
		GenericFunc: func(event event.GenericEvent) bool {
			return operationNotSucceeded(event.Object)
		},
		DeleteFunc: func(event event.DeleteEvent) bool {
			return operationNotSucceeded(event.Object)
		},
	}
}

// StatefulSetStatusChange is a predicate for status changes of `StatefulSet` resources.
func StatefulSetStatusChange() predicate.Predicate {
	statusChange := func(objOld, objNew client.Object) bool {
		stsOld, ok := objOld.(*appsv1.StatefulSet)
		if !ok {
			return false
		}
		stsNew, ok := objNew.(*appsv1.StatefulSet)
		if !ok {
			return false
		}
		return !reflect.DeepEqual(stsOld.Status, stsNew.Status)
	}

	return predicate.Funcs{
		CreateFunc: func(event event.CreateEvent) bool {
			return true
		},
		UpdateFunc: func(event event.UpdateEvent) bool {
			return statusChange(event.ObjectOld, event.ObjectNew)
		},
		GenericFunc: func(event event.GenericEvent) bool {
			return true
		},
		DeleteFunc: func(event event.DeleteEvent) bool {
			return true
		},
	}
}

// LeaseHolderIdentityChange is a predicate for holderIdentity changes of `Lease` resources.
func LeaseHolderIdentityChange() predicate.Predicate {
	holderIdentityChange := func(objOld, objNew client.Object) bool {
		leaseOld, ok := objOld.(*coordinationv1.Lease)
		if !ok {
			return false
		}
		leaseNew, ok := objNew.(*coordinationv1.Lease)
		if !ok {
			return false
		}
		return *leaseOld.Spec.HolderIdentity != *leaseNew.Spec.HolderIdentity
	}

	return predicate.Funcs{
		CreateFunc: func(event event.CreateEvent) bool {
			return true
		},
		UpdateFunc: func(event event.UpdateEvent) bool {
			return holderIdentityChange(event.ObjectOld, event.ObjectNew)
		},
		GenericFunc: func(event event.GenericEvent) bool {
			return true
		},
		DeleteFunc: func(event event.DeleteEvent) bool {
			return true
		},
	}
}
