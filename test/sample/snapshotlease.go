package sample

import (
	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/utils"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NewDeltaSnapshotLease creates a delta snapshot lease from the passed in etcd object.
func NewDeltaSnapshotLease(etcd *druidv1alpha1.Etcd) *coordinationv1.Lease {
	leaseName := etcd.GetDeltaSnapshotLeaseName()
	return createLease(etcd, leaseName)
}

// NewFullSnapshotLease creates a full snapshot lease from the passed in etcd object.
func NewFullSnapshotLease(etcd *druidv1alpha1.Etcd) *coordinationv1.Lease {
	leaseName := etcd.GetFullSnapshotLeaseName()
	return createLease(etcd, leaseName)
}

func createLease(etcd *druidv1alpha1.Etcd, leaseName string) *coordinationv1.Lease {
	return &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      leaseName,
			Namespace: etcd.Namespace,
			Labels: utils.MergeMaps[string, string](etcd.GetDefaultLabels(), map[string]string{
				"gardener.cloud/owned-by":        etcd.Name,
				v1beta1constants.GardenerPurpose: "etcd-snapshot-lease",
			}),
			OwnerReferences: []metav1.OwnerReference{etcd.GetAsOwnerReference()},
		},
	}
}
