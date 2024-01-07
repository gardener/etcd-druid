package sample

import (
	"errors"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func NewMemberLeases(etcd *druidv1alpha1.Etcd, numLeases int) ([]*coordinationv1.Lease, error) {
	if numLeases > int(etcd.Spec.Replicas) {
		return nil, errors.New("number of requested leases is greater than the etcd replicas")
	}
	memberLeaseNames := etcd.GetMemberLeaseNames()
	leases := make([]*coordinationv1.Lease, 0, numLeases)
	for i := 0; i < numLeases; i++ {
		lease := &coordinationv1.Lease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      memberLeaseNames[i],
				Namespace: etcd.Namespace,
			},
		}
		leases = append(leases, lease)
	}
	return leases, nil
}
