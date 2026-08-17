// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package memberlease

import (
	"fmt"
	"slices"

	druidapicommon "github.com/gardener/etcd-druid/api/common"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"
	"github.com/gardener/etcd-druid/internal/component"
	druiderr "github.com/gardener/etcd-druid/internal/errors"
	"github.com/gardener/etcd-druid/internal/utils"

	"github.com/hashicorp/go-multierror"
	coordinationv1 "k8s.io/api/coordination/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	// ErrListMemberLease indicates an error in listing the member lease resources.
	ErrListMemberLease druidapicommon.ErrorCode = "ERR_LIST_MEMBER_LEASE"
	// ErrSyncMemberLease indicates an error in syncing the member lease resources.
	ErrSyncMemberLease druidapicommon.ErrorCode = "ERR_SYNC_MEMBER_LEASE"
	// ErrDeleteMemberLease indicates an error in deleting the member lease resources.
	ErrDeleteMemberLease druidapicommon.ErrorCode = "ERR_DELETE_MEMBER_LEASE"
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

// Sync creates or updates the member leases for the given Etcd and deletes any
// surplus leases whose ordinal is no longer within spec.replicas (e.g. after
// a scale-in).
func (r _resource) Sync(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	objectKeys := getObjectKeys(etcd)
	if err := r.createOrUpdateLeases(ctx, etcd, objectKeys); err != nil {
		return err
	}
	return r.deleteSurplusLeases(ctx, etcd, objectKeys)
}

// createOrUpdateLeases concurrently creates or updates the leases for all desired member ordinals.
func (r _resource) createOrUpdateLeases(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd, objectKeys []client.ObjectKey) error {
	tasks := make([]utils.OperatorTask, len(objectKeys))
	for i, objKey := range objectKeys {
		tasks[i] = utils.OperatorTask{
			Name: "CreateOrUpdate-" + objKey.String(),
			Fn: func(ctx component.OperatorContext) error {
				return r.doCreateOrUpdate(ctx, etcd, objKey)
			},
		}
	}
	var errs error
	for _, err := range utils.RunConcurrently(ctx, tasks) {
		errs = multierror.Append(errs, err)
	}

	if !druidv1alpha1.ArePodsManagedByEtcdDruid(etcd) {
		if err := r.deleteStaleMemberLeases(ctx, etcd); err != nil {
			errs = multierror.Append(errs, err)
		}
	}
	return errs
}

// deleteStaleMemberLeases deletes member leases that exist but are no longer required.
// This can happen if a member is removed/replaced when configured with externally managed members.
func (r _resource) deleteStaleMemberLeases(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	existingLeaseNames, err := r.GetExistingResourceNames(ctx, etcd.ObjectMeta)
	if err != nil {
		return err
	}
	desiredLeaseNames := druidv1alpha1.GetMemberLeaseNames(etcd)
	deleteTasks := make([]utils.OperatorTask, 0)
	for _, existingLeaseName := range existingLeaseNames {
		if !slices.Contains(desiredLeaseNames, existingLeaseName) {
			leaseObjKey := client.ObjectKey{Name: existingLeaseName, Namespace: etcd.Namespace}
			deleteTasks = append(deleteTasks, utils.OperatorTask{
				Name: "Delete-" + leaseObjKey.String(),
				Fn: func(ctx component.OperatorContext) error {
					return r.doDelete(ctx, leaseObjKey)
				},
			})
		}
	}
	var errs error
	if errorList := utils.RunConcurrently(ctx, deleteTasks); len(errorList) > 0 {
		for _, err := range errorList {
			errs = multierror.Append(errs, err)
		}
	}
	return errs
}

// deleteSurplusLeases concurrently removes leases that are no longer in the desired set (e.g. after a scale-in).
func (r _resource) deleteSurplusLeases(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd, objectKeys []client.ObjectKey) error {
	// A transition to zero replicas is hibernation, not a scale-in. The desired
	// lease set is empty at replicas=0, so deleting surplus leases here would
	// wipe every member lease and lose the state needed to wake the cluster up.
	// Leave the leases intact, mirroring the guard in deleteSurplusPVCs.
	if etcd.Spec.Replicas <= 0 {
		return nil
	}
	desired := desiredLeaseNames(objectKeys)
	existing, err := r.GetExistingResourceNames(ctx, etcd.ObjectMeta)
	if err != nil {
		return err
	}
	var tasks []utils.OperatorTask
	for _, name := range existing {
		if _, ok := desired[name]; !ok {
			tasks = append(tasks, utils.OperatorTask{
				Name: "Delete-" + name,
				Fn: func(ctx component.OperatorContext) error {
					return r.doDeleteLease(ctx, etcd, name)
				},
			})
		}
	}
	var errs error
	for _, err := range utils.RunConcurrently(ctx, tasks) {
		errs = multierror.Append(errs, err)
	}
	return errs
}

// doDeleteLease deletes a single surplus member lease and logs on success.
func (r _resource) doDeleteLease(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd, name string) error {
	lease := emptyMemberLease(client.ObjectKey{Name: name, Namespace: etcd.Namespace})
	if err := r.client.Delete(ctx, lease); client.IgnoreNotFound(err) != nil {
		return druiderr.WrapError(err,
			ErrDeleteMemberLease,
			component.OperationSync,
			fmt.Sprintf("Failed to delete surplus member lease %s for etcd: %v", name, druidv1alpha1.GetNamespaceName(etcd.ObjectMeta)))
	}
	ctx.Logger.Info("deleted surplus member lease", "name", name)
	return nil
}

func (r _resource) doCreateOrUpdate(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd, objKey client.ObjectKey) error {
	lease := emptyMemberLease(objKey)
	opResult, err := controllerutil.CreateOrPatch(ctx, r.client, lease, func() error {
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

func (r _resource) doDelete(ctx component.OperatorContext, objectKey client.ObjectKey) error {
	if err := r.client.Delete(ctx, emptyMemberLease(objectKey)); err != nil {
		if errors.IsNotFound(err) {
			ctx.Logger.Info("No member lease found, Deletion is a No-Op", "objectKey", objectKey)
			return nil
		}
		return druiderr.WrapError(err,
			ErrDeleteMemberLease,
			component.OperationTriggerDelete,
			fmt.Sprintf("Failed to delete member lease: %v", objectKey))
	}
	ctx.Logger.Info("deleted", "component", "member-lease", "objectKey", objectKey)
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

func desiredLeaseNames(objectKeys []client.ObjectKey) map[string]struct{} {
	names := make(map[string]struct{}, len(objectKeys))
	for _, k := range objectKeys {
		names[k.Name] = struct{}{}
	}
	return names
}

func getObjectKeys(etcd *druidv1alpha1.Etcd) []client.ObjectKey {
	leaseNames := druidv1alpha1.GetMemberLeaseNames(etcd)
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
