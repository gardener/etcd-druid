package snapshotlease

import (
	"context"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"
	"github.com/gardener/etcd-druid/internal/operator/resource"
	"github.com/gardener/etcd-druid/internal/utils"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	"github.com/gardener/gardener/pkg/controllerutils"
	"github.com/go-logr/logr"
	coordinationv1 "k8s.io/api/coordination/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const purpose = "etcd-snapshot-lease"

type _resource struct {
	client client.Client
	logger logr.Logger
}

func (r _resource) GetExistingResourceNames(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd) ([]string, error) {
	resourceNames := make([]string, 0, 2)
	// We have to get snapshot leases one lease at a time and cannot use label-selector based listing
	// because currently snapshot lease do not have proper labels on them. In this new code
	// we will add the labels.
	// TODO: Once all snapshot leases have a purpose label on them, then we can use List instead of individual Get calls.
	deltaSnapshotLease, err := r.getLease(ctx,
		client.ObjectKey{Name: etcd.GetDeltaSnapshotLeaseName(), Namespace: etcd.Namespace})
	if err != nil {
		return resourceNames, err
	}
	resourceNames = append(resourceNames, deltaSnapshotLease.Name)
	fullSnapshotLease, err := r.getLease(ctx,
		client.ObjectKey{Name: etcd.GetFullSnapshotLeaseName(), Namespace: etcd.Namespace})
	if err != nil {
		return resourceNames, err
	}
	resourceNames = append(resourceNames, fullSnapshotLease.Name)
	return resourceNames, nil
}

func (r _resource) getLease(ctx context.Context, objectKey client.ObjectKey) (*coordinationv1.Lease, error) {
	lease := &coordinationv1.Lease{}
	if err := r.client.Get(ctx, objectKey, lease); err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return lease, nil
}

func (r _resource) Sync(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	if !etcd.IsBackupStoreEnabled() {
		r.logger.Info("Backup has been disabled. Triggering delete of snapshot leases")
		return r.TriggerDelete(ctx, etcd)
	}
	for _, objKey := range getObjectKeys(etcd) {
		opResult, err := r.doSync(ctx, etcd, objKey)
		if err != nil {
			return err
		}
		r.logger.Info("Triggered create or update", "lease", objKey, "result", opResult)
	}
	return nil
}

func (r _resource) doSync(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd, leaseObjectKey client.ObjectKey) (controllerutil.OperationResult, error) {
	lease := emptySnapshotLease(leaseObjectKey)
	opResult, err := controllerutils.GetAndCreateOrMergePatch(ctx, r.client, lease, func() error {
		lease.Labels = getLabels(etcd)
		lease.OwnerReferences = []metav1.OwnerReference{etcd.GetAsOwnerReference()}
		return nil
	})
	return opResult, err
}

func (r _resource) TriggerDelete(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	for _, objKey := range getObjectKeys(etcd) {
		err := client.IgnoreNotFound(r.client.Delete(ctx, emptySnapshotLease(objKey)))
		if err != nil {
			return err
		}
	}
	return nil
}

func New(client client.Client, logger logr.Logger) resource.Operator {
	return &_resource{
		client: client,
		logger: logger,
	}
}

func getLabels(etcd *druidv1alpha1.Etcd) map[string]string {
	labels := make(map[string]string)
	labels[common.GardenerOwnedBy] = etcd.Name
	labels[v1beta1constants.GardenerPurpose] = purpose
	return utils.MergeMaps[string, string](labels, etcd.GetDefaultLabels())
}

func getObjectKeys(etcd *druidv1alpha1.Etcd) []client.ObjectKey {
	return []client.ObjectKey{
		{
			Name:      etcd.GetDeltaSnapshotLeaseName(),
			Namespace: etcd.Namespace,
		},
		{
			Name:      etcd.GetFullSnapshotLeaseName(),
			Namespace: etcd.Namespace,
		},
	}
}

func emptySnapshotLease(objectKey client.ObjectKey) *coordinationv1.Lease {
	return &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      objectKey.Name,
			Namespace: objectKey.Namespace,
		},
	}
}
