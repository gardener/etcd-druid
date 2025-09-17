// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package compaction

import (
	"reflect"
	"strings"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"

	batchv1 "k8s.io/api/batch/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const controllerName = "compaction-controller"

// RegisterWithManager registers the Compaction Controller with the given controller manager.
func (r *Reconciler) RegisterWithManager(mgr ctrl.Manager) error {
	return ctrl.
		NewControllerManagedBy(mgr).
		Named(controllerName).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: r.config.Workers,
		}).
		For(&druidv1alpha1.Etcd{}).
		WithEventFilter(predicate.
			Or(snapshotRevisionChanged(), jobStatusChanged())).
		Owns(&coordinationv1.Lease{}).
		Owns(&batchv1.Job{}).
		Complete(r)
}

// snapshotRevisionChanged is a predicate that is `true` if the passed lease object is a snapshot lease and if the lease
// object's holderIdentity is updated.
func snapshotRevisionChanged() predicate.Predicate {
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
		GenericFunc: func(_ event.GenericEvent) bool {
			return false
		},
		DeleteFunc: func(_ event.DeleteEvent) bool {
			return false
		},
	}
}

// jobStatusChanged is a predicate that is `true` if the status of a job changes.
func jobStatusChanged() predicate.Predicate {
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
		CreateFunc: func(_ event.CreateEvent) bool {
			return false
		},
		UpdateFunc: func(event event.UpdateEvent) bool {
			return statusChange(event.ObjectOld, event.ObjectNew)
		},
		GenericFunc: func(_ event.GenericEvent) bool {
			return false
		},
		DeleteFunc: func(_ event.DeleteEvent) bool {
			return false
		},
	}
}
