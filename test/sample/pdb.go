package sample

import (
	"fmt"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// NewPodDisruptionBudget creates a new instance of PodDisruptionBudget from the passed etcd object.
func NewPodDisruptionBudget(etcd *druidv1alpha1.Etcd) *policyv1.PodDisruptionBudget {
	return &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      etcd.Name,
			Namespace: etcd.Namespace,
			Labels:    etcd.GetDefaultLabels(),
			Annotations: map[string]string{
				common.GardenerOwnedBy:   fmt.Sprintf("%s/%s", etcd.Namespace, etcd.Name),
				common.GardenerOwnerType: "etcd",
			},
			OwnerReferences: []metav1.OwnerReference{etcd.GetAsOwnerReference()},
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: etcd.GetDefaultLabels(),
			},
			MinAvailable: &intstr.IntOrString{
				IntVal: computePDBMinAvailable(etcd.Spec.Replicas),
				Type:   intstr.Int,
			},
		},
	}
}

func computePDBMinAvailable(etcdReplicas int32) int32 {
	if etcdReplicas <= 1 {
		return 0
	}
	return etcdReplicas/2 + 1
}
