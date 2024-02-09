package utils

import (
	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
)

// GetSuspendEtcdSpecReconcileAnnotationKey gets the annotation key set on an etcd resource signalling the intent
// to suspend spec reconciliation for this etcd resource. If no annotation is set then it will return nil.
func GetSuspendEtcdSpecReconcileAnnotationKey(etcd *druidv1alpha1.Etcd) *string {
	var annotationKey *string
	if metav1.HasAnnotation(etcd.ObjectMeta, druidv1alpha1.SuspendEtcdSpecReconcileAnnotation) {
		annotationKey = pointer.String(druidv1alpha1.SuspendEtcdSpecReconcileAnnotation)
	} else if metav1.HasAnnotation(etcd.ObjectMeta, druidv1alpha1.IgnoreReconciliationAnnotation) {
		annotationKey = pointer.String(druidv1alpha1.IgnoreReconciliationAnnotation)
	}
	return annotationKey
}
