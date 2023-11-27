// Copyright 2023 SAP SE or an SAP affiliate company
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

package etcd

import (
	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/operator/resource"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
)

func (r *Reconciler) triggerReconcileSpec() error {
	return nil
}

func (r *Reconciler) canReconcileSpec(etcd *druidv1alpha1.Etcd) bool {
	//Check if spec reconciliation has been suspended, if yes, then record the event and return false.
	if suspendReconcileAnnotKey := r.getSuspendEtcdSpecReconcileAnnotationKey(etcd); suspendReconcileAnnotKey != nil {
		r.recordEtcdSpecReconcileSuspension(etcd, *suspendReconcileAnnotKey)
		return false
	}
	// Check if
	return true
}

// getSuspendEtcdSpecReconcileAnnotationKey gets the annotation key set on an etcd resource signalling the intent
// to suspend spec reconciliation for this etcd resource. If no annotation is set then it will return nil.
func (r *Reconciler) getSuspendEtcdSpecReconcileAnnotationKey(etcd *druidv1alpha1.Etcd) *string {
	var annotationKey *string
	if metav1.HasAnnotation(etcd.ObjectMeta, druidv1alpha1.SuspendEtcdSpecReconcileAnnotation) {
		annotationKey = pointer.String(druidv1alpha1.SuspendEtcdSpecReconcileAnnotation)
	} else if metav1.HasAnnotation(etcd.ObjectMeta, druidv1alpha1.IgnoreReconciliationAnnotation) {
		annotationKey = pointer.String(druidv1alpha1.IgnoreReconciliationAnnotation)
	}
	return annotationKey
}

func (r *Reconciler) recordEtcdSpecReconcileSuspension(etcd *druidv1alpha1.Etcd, annotationKey string) {
	r.recorder.Eventf(
		etcd,
		corev1.EventTypeWarning,
		"SpecReconciliationSkipped",
		"spec reconciliation of %s/%s is skipped by etcd-druid due to the presence of annotation %s on the etcd resource",
		etcd.Namespace,
		etcd.Name,
		annotationKey,
	)
}

func (r *Reconciler) getOrderedOperatorsForSync() []resource.Operator {
	return []resource.Operator{
		r.operatorRegistry.MemberLeaseOperator(),
		r.operatorRegistry.SnapshotLeaseOperator(),
		r.operatorRegistry.ClientServiceOperator(),
		r.operatorRegistry.PeerServiceOperator(),
		r.operatorRegistry.ConfigMapOperator(),
		r.operatorRegistry.PodDisruptionBudgetOperator(),
		r.operatorRegistry.ServiceAccountOperator(),
		r.operatorRegistry.RoleOperator(),
		r.operatorRegistry.RoleBindingOperator(),
		r.operatorRegistry.StatefulSetOperator(),
	}
}
