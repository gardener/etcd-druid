package sample

import (
	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
)

// NewServiceAccount creates a new sample ServiceAccount using the passed in etcd.
func NewServiceAccount(etcd *druidv1alpha1.Etcd, disableAutoMount bool) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:            etcd.GetServiceAccountName(),
			Namespace:       etcd.Namespace,
			Labels:          etcd.GetDefaultLabels(),
			OwnerReferences: []metav1.OwnerReference{etcd.GetAsOwnerReference()},
		},
		AutomountServiceAccountToken: pointer.Bool(!disableAutoMount),
	}
}
