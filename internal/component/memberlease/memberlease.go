// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package memberlease

import (
	"fmt"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"

	"github.com/gardener/etcd-druid/internal/common"
	"github.com/gardener/etcd-druid/internal/component"
	druiderr "github.com/gardener/etcd-druid/internal/errors"
	"github.com/gardener/etcd-druid/internal/utils"

	"github.com/gardener/gardener/pkg/controllerutils"
	"github.com/hashicorp/go-multierror"
	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// ErrListMemberLease indicates an error in listing the member lease resources.
	ErrListMemberLease druidv1alpha1.ErrorCode = "ERR_LIST_MEMBER_LEASE"
	// ErrSyncMemberLease indicates an error in syncing the member lease resources.
	ErrSyncMemberLease druidv1alpha1.ErrorCode = "ERR_SYNC_MEMBER_LEASE"
	// ErrDeleteMemberLease indicates an error in deleting the member lease resources.
	ErrDeleteMemberLease druidv1alpha1.ErrorCode = "ERR_DELETE_MEMBER_LEASE"
)

type _resource struct {
	client client.Client
}

// New returns a new member lease component operator.
func New(client client.Client) component.Operator {
	return &_resource{
		client: client,
	}
}

// GetExistingResourceNames returns the names of the existing member leases for the given Etcd.
func (r _resource) GetExistingResourceNames(ctx component.OperatorContext, etcdObjMeta metav1.ObjectMeta) ([]string, error) {
	resourceNames := make([]string, 0, 1)

	objMetaList := &metav1.PartialObjectMetadataList{}
	objMetaList.SetGroupVersionKind(coordinationv1.SchemeGroupVersion.WithKind("Lease"))
	if err := r.client.List(ctx,
		objMetaList,
		client.InNamespace(etcdObjMeta.Namespace),
		client.MatchingLabels(getSelectorLabelsForAllMemberLeases(etcdObjMeta)),
	); err != nil {
		return resourceNames, druiderr.WrapError(err,
			ErrListMemberLease,
			component.OperationGetExistingResourceNames,
			fmt.Sprintf("Error listing member leases for etcd: %v", druidv1alpha1.GetNamespaceName(etcdObjMeta)))
	}
	for _, lease := range objMetaList.Items {
		if metav1.IsControlledBy(&lease, &etcdObjMeta) {
			resourceNames = append(resourceNames, lease.Name)
		}
	}
	return resourceNames, nil
}

// PreSync is a no-op for the member lease component.
func (r _resource) PreSync(_ component.OperatorContext, _ *druidv1alpha1.Etcd) error { return nil }

// Sync creates or updates the member leases for the given Etcd.
func (r _resource) Sync(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	objectKeys := getObjectKeys(etcd)
	createTasks := make([]utils.OperatorTask, len(objectKeys))
	var errs error

	for i, objKey := range objectKeys {
		createTasks[i] = utils.OperatorTask{
			Name: "CreateOrUpdate-" + objKey.String(),
			Fn: func(ctx component.OperatorContext) error {
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

func (r _resource) doCreateOrUpdate(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd, objKey client.ObjectKey) error {
	lease := emptyMemberLease(objKey)
	opResult, err := controllerutils.GetAndCreateOrMergePatch(ctx, r.client, lease, func() error {
		buildResource(etcd, lease)
		return nil
	})
	if err != nil {
		return druiderr.WrapError(err,
			ErrSyncMemberLease,
			component.OperationSync,
			fmt.Sprintf("Error syncing member lease: %v for etcd: %v", objKey, druidv1alpha1.GetNamespaceName(etcd.ObjectMeta)))
	}
	ctx.Logger.Info("triggered create or update of member lease", "objectKey", objKey, "operationResult", opResult)
	return nil
}

// TriggerDelete deletes the member leases for the given Etcd.
func (r _resource) TriggerDelete(ctx component.OperatorContext, etcdObjMeta metav1.ObjectMeta) error {
	ctx.Logger.Info("Triggering deletion of member leases")
	if err := r.client.DeleteAllOf(ctx,
		&coordinationv1.Lease{},
		client.InNamespace(etcdObjMeta.Namespace),
		client.MatchingLabels(getSelectorLabelsForAllMemberLeases(etcdObjMeta))); err != nil {
		return druiderr.WrapError(err,
			ErrDeleteMemberLease,
			component.OperationTriggerDelete,
			fmt.Sprintf("Failed to delete member leases for etcd: %v", druidv1alpha1.GetNamespaceName(etcdObjMeta)))
	}
	ctx.Logger.Info("deleted", "component", "member-leases")
	return nil
}

func buildResource(etcd *druidv1alpha1.Etcd, lease *coordinationv1.Lease) {
	lease.Labels = getLabels(etcd, lease.Name)
	lease.OwnerReferences = []metav1.OwnerReference{druidv1alpha1.GetAsOwnerReference(etcd.ObjectMeta)}
}

func getObjectKeys(etcd *druidv1alpha1.Etcd) []client.ObjectKey {
	leaseNames := druidv1alpha1.GetMemberLeaseNames(etcd.ObjectMeta, etcd.Spec.Replicas)
	objectKeys := make([]client.ObjectKey, 0, len(leaseNames))
	for _, leaseName := range leaseNames {
		objectKeys = append(objectKeys, client.ObjectKey{Name: leaseName, Namespace: etcd.Namespace})
	}
	return objectKeys
}

func getSelectorLabelsForAllMemberLeases(etcdObjMeta metav1.ObjectMeta) map[string]string {
	leaseMatchingLabels := map[string]string{
		druidv1alpha1.LabelComponentKey: common.ComponentNameMemberLease,
	}
	return utils.MergeMaps(druidv1alpha1.GetDefaultLabels(etcdObjMeta), leaseMatchingLabels)
}

func getLabels(etcd *druidv1alpha1.Etcd, leaseName string) map[string]string {
	leaseLabels := map[string]string{
		druidv1alpha1.LabelComponentKey: common.ComponentNameMemberLease,
		druidv1alpha1.LabelAppNameKey:   leaseName,
	}
	return utils.MergeMaps(leaseLabels, druidv1alpha1.GetDefaultLabels(etcd.ObjectMeta))
}

func emptyMemberLease(objectKey client.ObjectKey) *coordinationv1.Lease {
	return &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      objectKey.Name,
			Namespace: objectKey.Namespace,
		},
	}
}
