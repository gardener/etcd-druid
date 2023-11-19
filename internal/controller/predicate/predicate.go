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
	"strings"

	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
)

func hasOperationAnnotation(obj client.Object) bool {
	return obj.GetAnnotations()[v1beta1constants.GardenerOperation] == v1beta1constants.GardenerOperationReconcile
}

// HasOperationAnnotation is a predicate for the operation annotation.
func HasOperationAnnotation() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(event event.CreateEvent) bool {
			return hasOperationAnnotation(event.Object)
		},
		UpdateFunc: func(event event.UpdateEvent) bool {
			return hasOperationAnnotation(event.ObjectNew)
		},
		GenericFunc: func(event event.GenericEvent) bool {
			return hasOperationAnnotation(event.Object)
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

// EtcdReconciliationFinished is a predicate to use for etcd resources whose reconciliation has finished.
func EtcdReconciliationFinished(ignoreOperationAnnotation bool) predicate.Predicate {
	reconciliationFinished := func(obj client.Object) bool {
		etcd, ok := obj.(*druidv1alpha1.Etcd)
		if !ok {
			return false
		}

		if etcd.Status.ObservedGeneration == nil {
			return false
		}

		condition := *etcd.Status.ObservedGeneration == etcd.Generation

		if !ignoreOperationAnnotation {
			condition = condition && !hasOperationAnnotation(etcd)
		}

		return condition
	}

	return predicate.Funcs{
		CreateFunc: func(event event.CreateEvent) bool {
			return reconciliationFinished(event.Object)
		},
		UpdateFunc: func(event event.UpdateEvent) bool {
			return reconciliationFinished(event.ObjectNew)
		},
		GenericFunc: func(event event.GenericEvent) bool {
			return reconciliationFinished(event.Object)
		},
		DeleteFunc: func(event event.DeleteEvent) bool {
			return false
		},
	}
}

// SnapshotRevisionChanged is a predicate that is `true` if the passed lease object is a snapshot lease and if the lease
// object's holderIdentity is updated.
func SnapshotRevisionChanged() predicate.Predicate {
	isSnapshotLease := func(obj client.Object) bool {
		lease, ok := obj.(*coordinationv1.Lease)
		if !ok {
			return false
		}

		return strings.HasSuffix(lease.Name, "full-snap") || strings.HasSuffix(lease.Name, "delta-snap")
	}

	holderIdentityChange := func(objOld, objNew client.Object) bool {
		leaseOld, ok := objOld.(*coordinationv1.Lease)
		if !ok {
			return false
		}
		leaseNew, ok := objNew.(*coordinationv1.Lease)
		if !ok {
			return false
		}

		return !reflect.DeepEqual(leaseOld.Spec.HolderIdentity, leaseNew.Spec.HolderIdentity)
	}

	return predicate.Funcs{
		CreateFunc: func(event event.CreateEvent) bool {
			return isSnapshotLease(event.Object)
		},
		UpdateFunc: func(event event.UpdateEvent) bool {
			return isSnapshotLease(event.ObjectNew) && holderIdentityChange(event.ObjectOld, event.ObjectNew)
		},
		GenericFunc: func(event event.GenericEvent) bool {
			return false
		},
		DeleteFunc: func(event event.DeleteEvent) bool {
			return false
		},
	}
}

// JobStatusChanged is a predicate that is `true` if the status of a job changes.
func JobStatusChanged() predicate.Predicate {
	statusChange := func(objOld, objNew client.Object) bool {
		jobOld, ok := objOld.(*batchv1.Job)
		if !ok {
			return false
		}
		jobNew, ok := objNew.(*batchv1.Job)
		if !ok {
			return false
		}
		return !apiequality.Semantic.DeepEqual(jobOld.Status, jobNew.Status)
	}

	return predicate.Funcs{
		CreateFunc: func(event event.CreateEvent) bool {
			return false
		},
		UpdateFunc: func(event event.UpdateEvent) bool {
			return statusChange(event.ObjectOld, event.ObjectNew)
		},
		GenericFunc: func(event event.GenericEvent) bool {
			return false
		},
		DeleteFunc: func(event event.DeleteEvent) bool {
			return false
		},
	}
}

// StatefulSetSpecChange is a predicate for status changes of `StatefulSet` resources.
func StatefulSetSpecChange() predicate.Predicate {
	specChange := func(objOld, objNew client.Object) bool {
		stsOld, ok := objOld.(*appsv1.StatefulSet)
		if !ok {
			return false
		}
		stsNew, ok := objNew.(*appsv1.StatefulSet)
		if !ok {
			return false
		}
		return !apiequality.Semantic.DeepEqual(stsOld.Spec, stsNew.Spec)
	}

	return predicate.Funcs{
		CreateFunc: func(event event.CreateEvent) bool {
			return true
		},
		UpdateFunc: func(event event.UpdateEvent) bool {
			return specChange(event.ObjectOld, event.ObjectNew)
		},
		GenericFunc: func(event event.GenericEvent) bool {
			return true
		},
		DeleteFunc: func(event event.DeleteEvent) bool {
			return true
		},
	}
}

func StatefulSetContainerResourcesChange() predicate.Predicate {
	containerResourcesChange := func(objOld, objNew client.Object) bool {
		stsOld, ok := objOld.(*appsv1.StatefulSet)
		if !ok {
			return false
		}
		stsNew, ok := objNew.(*appsv1.StatefulSet)
		if !ok {
			return false
		}

		containersOld := make(map[string]*corev1.Container)
		containersNew := make(map[string]*corev1.Container)

		if len(stsOld.Spec.Template.Spec.Containers) != len(stsNew.Spec.Template.Spec.Containers) {
			return true
		}
		for _, c := range stsOld.Spec.Template.Spec.Containers {
			containersOld[c.Name] = &c
		}
		for _, cNew := range stsNew.Spec.Template.Spec.Containers {
			containersNew[cNew.Name] = &cNew
			cOld, ok := containersOld[cNew.Name]
			if !ok {
				return true
			}
			if !apiequality.Semantic.DeepEqual(cNew.Resources, cOld.Resources) {
				return true
			}
		}

		return false
	}

	return predicate.Funcs{
		CreateFunc: func(event event.CreateEvent) bool {
			return false
		},
		UpdateFunc: func(event event.UpdateEvent) bool {
			return containerResourcesChange(event.ObjectOld, event.ObjectNew)
		},
		GenericFunc: func(event event.GenericEvent) bool {
			return false
		},
		DeleteFunc: func(event event.DeleteEvent) bool {
			return false
		},
	}
}
