// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package predicate

import (
	"reflect"
	"strings"

	batchv1 "k8s.io/api/batch/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

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
