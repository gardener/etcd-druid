// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package snapshotlease

import (
	"context"
	"errors"
	"fmt"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"
	"github.com/gardener/etcd-druid/internal/component"
	druiderr "github.com/gardener/etcd-druid/internal/errors"
	"github.com/gardener/etcd-druid/internal/utils"

	"github.com/gardener/gardener/pkg/controllerutils"
	coordinationv1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// ErrGetSnapshotLease indicates an error in getting the snapshot lease resources.
	ErrGetSnapshotLease druidv1alpha1.ErrorCode = "ERR_GET_SNAPSHOT_LEASE"
	// ErrSyncSnapshotLease indicates an error in syncing the snapshot lease resources.
	ErrSyncSnapshotLease druidv1alpha1.ErrorCode = "ERR_SYNC_SNAPSHOT_LEASE"
	// ErrDeleteSnapshotLease indicates an error in deleting the snapshot lease resources.
	ErrDeleteSnapshotLease druidv1alpha1.ErrorCode = "ERR_DELETE_SNAPSHOT_LEASE"
)

type _resource struct {
	client client.Client
}

// New returns a new snapshot lease component operator.
func New(client client.Client) component.Operator {
	return &_resource{
		client: client,
	}
}

// GetExistingResourceNames returns the names of the existing snapshot leases for the given Etcd.
func (r _resource) GetExistingResourceNames(ctx component.OperatorContext, etcdObjMeta metav1.ObjectMeta) ([]string, error) {
	resourceNames := make([]string, 0, 2)
	// We have to get snapshot leases one lease at a time and cannot use label-selector based listing
	// because currently snapshot lease do not have proper labels on them. In this new code
	// we will add the labels.
	// TODO: Once all snapshot leases have a purpose label on them, then we can use List instead of individual Get calls.
	deltaSnapshotObjectKey := client.ObjectKey{Name: druidv1alpha1.GetDeltaSnapshotLeaseName(etcdObjMeta), Namespace: etcdObjMeta.Namespace}
	deltaSnapshotLease, err := r.getLeasePartialObjectMetadata(ctx, deltaSnapshotObjectKey)
	if err != nil {
		return resourceNames, druiderr.WrapError(err,
			ErrGetSnapshotLease,
			"GetExistingResourceNames",
			fmt.Sprintf("Error getting delta snapshot lease: %v for etcd: %v", deltaSnapshotObjectKey, druidv1alpha1.GetNamespaceName(etcdObjMeta)),
		)
	}
	if deltaSnapshotLease != nil && metav1.IsControlledBy(deltaSnapshotLease, &etcdObjMeta) {
		resourceNames = append(resourceNames, deltaSnapshotLease.Name)
	}
	fullSnapshotObjectKey := client.ObjectKey{Name: druidv1alpha1.GetFullSnapshotLeaseName(etcdObjMeta), Namespace: etcdObjMeta.Namespace}
	fullSnapshotLease, err := r.getLeasePartialObjectMetadata(ctx, fullSnapshotObjectKey)
	if err != nil {
		return resourceNames, druiderr.WrapError(err,
			ErrGetSnapshotLease,
			"GetExistingResourceNames",
			fmt.Sprintf("Error getting full snapshot lease: %v for etcd: %v", fullSnapshotObjectKey, druidv1alpha1.GetNamespaceName(etcdObjMeta)),
		)
	}
	if fullSnapshotLease != nil && metav1.IsControlledBy(fullSnapshotLease, &etcdObjMeta) {
		resourceNames = append(resourceNames, fullSnapshotLease.Name)
	}
	return resourceNames, nil
}

// PreSync is a no-op for the snapshot lease component.
func (r _resource) PreSync(_ component.OperatorContext, _ *druidv1alpha1.Etcd) error { return nil }

// Sync creates or updates the snapshot leases for the given Etcd.
// If backups are disabled for the Etcd resource, any existing snapshot leases are deleted.
func (r _resource) Sync(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	if !etcd.IsBackupStoreEnabled() {
		ctx.Logger.Info("Backup has been disabled. Triggering deletion of snapshot leases")
		return r.deleteAllSnapshotLeases(ctx, etcd.ObjectMeta, func(err error) error {
			return druiderr.WrapError(err,
				ErrSyncSnapshotLease,
				"Sync",
				fmt.Sprintf("Failed to delete existing snapshot leases (due to backup being disabled for etcd) due to reason: %v", druidv1alpha1.GetNamespaceName(etcd.ObjectMeta)))
		})
	}

	objectKeys := getObjectKeys(etcd)
	syncTasks := make([]utils.OperatorTask, len(objectKeys))

	for i, objKey := range objectKeys {
		syncTasks[i] = utils.OperatorTask{
			Name: "CreateOrUpdate-" + objKey.String(),
			Fn: func(ctx component.OperatorContext) error {
				return r.doCreateOrUpdate(ctx, etcd, objKey)
			},
		}
	}
	return errors.Join(utils.RunConcurrently(ctx, syncTasks)...)
}

// TriggerDelete triggers the deletion of the snapshot leases for the given Etcd.
func (r _resource) TriggerDelete(ctx component.OperatorContext, etcdObjMeta metav1.ObjectMeta) error {
	ctx.Logger.Info("Triggering deletion of snapshot leases")
	if err := r.deleteAllSnapshotLeases(ctx, etcdObjMeta, func(err error) error {
		return druiderr.WrapError(err,
			ErrDeleteSnapshotLease,
			"TriggerDelete",
			fmt.Sprintf("Failed to delete snapshot leases for etcd: %v", druidv1alpha1.GetNamespaceName(etcdObjMeta)))
	}); err != nil {
		return err
	}
	ctx.Logger.Info("deleted", "component", "snapshot-leases")
	return nil
}

func (r _resource) deleteAllSnapshotLeases(ctx component.OperatorContext, etcdObjMeta metav1.ObjectMeta, wrapErrFn func(error) error) error {
	if err := r.client.DeleteAllOf(ctx,
		&coordinationv1.Lease{},
		client.InNamespace(etcdObjMeta.Namespace),
		client.MatchingLabels(getSelectorLabelsForAllSnapshotLeases(etcdObjMeta))); err != nil {
		return wrapErrFn(err)
	}
	return nil
}

func (r _resource) getLeasePartialObjectMetadata(ctx context.Context, objectKey client.ObjectKey) (*metav1.PartialObjectMetadata, error) {
	objMeta := &metav1.PartialObjectMetadata{}
	objMeta.SetGroupVersionKind(coordinationv1.SchemeGroupVersion.WithKind("Lease"))
	if err := r.client.Get(ctx, objectKey, objMeta); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return objMeta, nil
}

func (r _resource) doCreateOrUpdate(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd, leaseObjectKey client.ObjectKey) error {
	lease := emptySnapshotLease(leaseObjectKey)
	opResult, err := controllerutils.GetAndCreateOrMergePatch(ctx, r.client, lease, func() error {
		buildResource(etcd, lease)
		return nil
	})
	if err != nil {
		return druiderr.WrapError(err,
			ErrSyncSnapshotLease,
			"Sync",
			fmt.Sprintf("Error syncing snapshot lease: %v for etcd: %v", leaseObjectKey, druidv1alpha1.GetNamespaceName(etcd.ObjectMeta)))
	}
	ctx.Logger.Info("triggered create or update of snapshot lease", "objectKey", leaseObjectKey, "operationResult", opResult)

	return nil
}

func buildResource(etcd *druidv1alpha1.Etcd, lease *coordinationv1.Lease) {
	lease.Labels = getLabels(etcd, lease.Name)
	lease.OwnerReferences = []metav1.OwnerReference{druidv1alpha1.GetAsOwnerReference(etcd.ObjectMeta)}
}

func getSelectorLabelsForAllSnapshotLeases(etcdObjMeta metav1.ObjectMeta) map[string]string {
	leaseMatchingLabels := map[string]string{
		druidv1alpha1.LabelComponentKey: common.ComponentNameSnapshotLease,
	}
	return utils.MergeMaps(druidv1alpha1.GetDefaultLabels(etcdObjMeta), leaseMatchingLabels)
}

func getLabels(etcd *druidv1alpha1.Etcd, leaseName string) map[string]string {
	leaseLabels := map[string]string{
		druidv1alpha1.LabelComponentKey: common.ComponentNameSnapshotLease,
		druidv1alpha1.LabelAppNameKey:   leaseName,
	}
	return utils.MergeMaps(leaseLabels, druidv1alpha1.GetDefaultLabels(etcd.ObjectMeta))
}

func getObjectKeys(etcd *druidv1alpha1.Etcd) []client.ObjectKey {
	return []client.ObjectKey{
		{
			Name:      druidv1alpha1.GetDeltaSnapshotLeaseName(etcd.ObjectMeta),
			Namespace: etcd.Namespace,
		},
		{
			Name:      druidv1alpha1.GetFullSnapshotLeaseName(etcd.ObjectMeta),
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
