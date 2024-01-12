package snapshotlease

import (
	"context"
	"fmt"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"
	druiderr "github.com/gardener/etcd-druid/internal/errors"
	"github.com/gardener/etcd-druid/internal/operator/resource"
	"github.com/gardener/etcd-druid/internal/utils"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	"github.com/gardener/gardener/pkg/controllerutils"
	"github.com/hashicorp/go-multierror"
	coordinationv1 "k8s.io/api/coordination/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const purpose = "etcd-snapshot-lease"

const (
	ErrGetSnapshotLease    druidv1alpha1.ErrorCode = "ERR_GET_SNAPSHOT_LEASE"
	ErrDeleteSnapshotLease druidv1alpha1.ErrorCode = "ERR_DELETE_SNAPSHOT_LEASE"
	ErrSyncSnapshotLease   druidv1alpha1.ErrorCode = "ERR_SYNC_SNAPSHOT_LEASE"
)

type _resource struct {
	client client.Client
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
		return resourceNames, &druiderr.DruidError{
			Code:      ErrGetSnapshotLease,
			Cause:     err,
			Operation: "GetExistingResourceNames",
			Message:   fmt.Sprintf("Error getting delta snapshot lease: %s for etcd: %v", etcd.GetDeltaSnapshotLeaseName(), etcd.GetNamespaceName()),
		}
	}
	if deltaSnapshotLease != nil {
		resourceNames = append(resourceNames, deltaSnapshotLease.Name)
	}
	fullSnapshotLease, err := r.getLease(ctx,
		client.ObjectKey{Name: etcd.GetFullSnapshotLeaseName(), Namespace: etcd.Namespace})
	if err != nil {
		return resourceNames, &druiderr.DruidError{
			Code:      ErrGetSnapshotLease,
			Cause:     err,
			Operation: "GetExistingResourceNames",
			Message:   fmt.Sprintf("Error getting full snapshot lease: %s for etcd: %v", etcd.GetFullSnapshotLeaseName(), etcd.GetNamespaceName()),
		}
	}
	if fullSnapshotLease != nil {
		resourceNames = append(resourceNames, fullSnapshotLease.Name)
	}
	return resourceNames, nil
}

func (r _resource) Sync(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	if !etcd.IsBackupStoreEnabled() {
		ctx.Logger.Info("Backup has been disabled. Triggering delete of snapshot leases")
		return r.deleteAllSnapshotLeases(ctx, etcd, func(err error) error {
			return druiderr.WrapError(err,
				ErrSyncSnapshotLease,
				"Sync",
				fmt.Sprintf("Failed to delete existing snapshot leases due to backup being disabled for etcd: %v", etcd.GetNamespaceName()))
		})
	}

	objectKeys := getObjectKeys(etcd)
	createTasks := make([]utils.OperatorTask, len(objectKeys))
	var errs error

	for i, objKey := range objectKeys {
		objKey := objKey // capture the range variable
		createTasks[i] = utils.OperatorTask{
			Name: "CreateOrUpdate-" + objKey.String(),
			Fn: func(ctx resource.OperatorContext) error {
				return r.doCreateOrUpdate(ctx, etcd, objKey)
			},
		}
	}

	if errorList := utils.RunConcurrently(ctx, createTasks); len(errorList) > 0 {
		for _, err := range errorList {
			errs = multierror.Append(errs, err)
		}
	}
	return errs
}

func (r _resource) TriggerDelete(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	ctx.Logger.Info("Triggering delete of snapshot leases")
	if err := r.deleteAllSnapshotLeases(ctx, etcd, func(err error) error {
		return druiderr.WrapError(err,
			ErrDeleteSnapshotLease,
			"TriggerDelete",
			fmt.Sprintf("Failed to delete snapshot leases for etcd: %v", etcd.GetNamespaceName()))
	}); err != nil {
		return err
	}
	ctx.Logger.Info("deleted", "resource", "snapshot-leases")
	return nil
}

func (r _resource) deleteAllSnapshotLeases(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd, wrapErrFn func(error) error) error {
	if err := r.client.DeleteAllOf(ctx,
		&coordinationv1.Lease{},
		client.InNamespace(etcd.Namespace),
		client.MatchingLabels(getLabels(etcd))); err != nil {
		return wrapErrFn(err)
	}
	return nil
}

func New(client client.Client) resource.Operator {
	return &_resource{
		client: client,
	}
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

func (r _resource) doCreateOrUpdate(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd, leaseObjectKey client.ObjectKey) error {
	lease := emptySnapshotLease(leaseObjectKey)
	opResult, err := controllerutils.GetAndCreateOrMergePatch(ctx, r.client, lease, func() error {
		lease.Labels = getLabels(etcd)
		lease.OwnerReferences = []metav1.OwnerReference{etcd.GetAsOwnerReference()}
		return nil
	})
	if err != nil {
		return druiderr.WrapError(err,
			ErrSyncSnapshotLease,
			"Sync",
			fmt.Sprintf("Error syncing snapshot lease: %s for etcd: %v", leaseObjectKey.Name, etcd.GetNamespaceName()))
	}
	ctx.Logger.Info("triggered create or update of snapshot lease", "lease", leaseObjectKey, "operationResult", opResult)

	return nil
}

func getLabels(etcd *druidv1alpha1.Etcd) map[string]string {
	labels := make(map[string]string)
	labels[common.GardenerOwnedBy] = etcd.Name
	labels[v1beta1constants.GardenerPurpose] = purpose
	return utils.MergeMaps[string, string](etcd.GetDefaultLabels(), labels)
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
