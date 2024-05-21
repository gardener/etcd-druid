// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package statefulset

import (
	"fmt"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"
	"github.com/gardener/etcd-druid/internal/component"
	druiderr "github.com/gardener/etcd-druid/internal/errors"
	"github.com/gardener/etcd-druid/internal/features"
	"github.com/gardener/etcd-druid/internal/utils"
	"github.com/gardener/gardener/pkg/controllerutils"
	"github.com/gardener/gardener/pkg/utils/imagevector"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/component-base/featuregate"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// ErrGetStatefulSet indicates an error in getting the statefulset resource.
	ErrGetStatefulSet druidv1alpha1.ErrorCode = "ERR_GET_STATEFULSET"
	// ErrSyncStatefulSet indicates an error in syncing the statefulset resource.
	ErrSyncStatefulSet druidv1alpha1.ErrorCode = "ERR_SYNC_STATEFULSET"
	// ErrDeleteStatefulSet indicates an error in deleting the statefulset resource.
	ErrDeleteStatefulSet druidv1alpha1.ErrorCode = "ERR_DELETE_STATEFULSET"
)

type _resource struct {
	client         client.Client
	imageVector    imagevector.ImageVector
	useEtcdWrapper bool
}

// New returns a new statefulset component operator.
func New(client client.Client, imageVector imagevector.ImageVector, featureGates map[featuregate.Feature]bool) component.Operator {
	return &_resource{
		client:         client,
		imageVector:    imageVector,
		useEtcdWrapper: featureGates[features.UseEtcdWrapper],
	}
}

// GetExistingResourceNames returns the name of the existing statefulset for the given Etcd.
func (r _resource) GetExistingResourceNames(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) ([]string, error) {
	resourceNames := make([]string, 0, 1)
	objectKey := getObjectKey(etcd)
	objMeta := &metav1.PartialObjectMetadata{}
	objMeta.SetGroupVersionKind(appsv1.SchemeGroupVersion.WithKind("StatefulSet"))
	if err := r.client.Get(ctx, objectKey, objMeta); err != nil {
		if apierrors.IsNotFound(err) {
			return resourceNames, nil
		}
		return nil, druiderr.WrapError(err,
			ErrGetStatefulSet,
			"GetExistingResourceNames",
			fmt.Sprintf("Error getting StatefulSet: %v for etcd: %v", objectKey, etcd.GetNamespaceName()))
	}
	if metav1.IsControlledBy(objMeta, etcd) {
		resourceNames = append(resourceNames, objMeta.Name)
	}
	return resourceNames, nil
}

// Sync creates or updates the statefulset for the given Etcd.
func (r _resource) Sync(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	var (
		existingSTS *appsv1.StatefulSet
		err         error
	)
	objectKey := getObjectKey(etcd)
	if existingSTS, err = r.getExistingStatefulSet(ctx, etcd); err != nil {
		return druiderr.WrapError(err,
			ErrSyncStatefulSet,
			"Sync",
			fmt.Sprintf("Error getting StatefulSet: %v for etcd: %v", objectKey, etcd.GetNamespaceName()))
	}
	// There is no StatefulSet present. Create one.
	if existingSTS == nil {
		return r.createOrPatch(ctx, etcd)
	}

	// StatefulSet exists, check if TLS has been enabled for peer communication, if yes then it is currently a multistep
	// process to ensure that all members are updated and establish peer TLS communication.
	if err = r.handlePeerTLSChanges(ctx, etcd, existingSTS); err != nil {
		return druiderr.WrapError(err,
			ErrSyncStatefulSet,
			"Sync",
			fmt.Sprintf("Error while handling peer URL TLS change for StatefulSet: %v, etcd: %v", objectKey, etcd.GetNamespaceName()))
	}
	return r.createOrPatch(ctx, etcd)
}

// TriggerDelete triggers the deletion of the statefulset for the given Etcd.
func (r _resource) TriggerDelete(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	objectKey := getObjectKey(etcd)
	ctx.Logger.Info("Triggering deletion of StatefulSet", "objectKey", objectKey)
	if err := r.client.Delete(ctx, emptyStatefulSet(etcd)); err != nil {
		if apierrors.IsNotFound(err) {
			ctx.Logger.Info("No StatefulSet found, Deletion is a No-Op", "objectKey", objectKey.Name)
			return nil
		}
		return druiderr.WrapError(err,
			ErrDeleteStatefulSet,
			"TriggerDelete",
			fmt.Sprintf("Failed to delete StatefulSet: %v for etcd %v", objectKey, etcd.GetNamespaceName()))
	}
	ctx.Logger.Info("deleted", "component", "statefulset", "objectKey", objectKey)
	return nil
}

// getExistingStatefulSet gets the existing statefulset if it exists.
// If it is not found, it simply returns nil. Any other errors are returned as is.
func (r _resource) getExistingStatefulSet(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) (*appsv1.StatefulSet, error) {
	sts := emptyStatefulSet(etcd)
	if err := r.client.Get(ctx, getObjectKey(etcd), sts); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return sts, nil
}

// createOrPatchWithReplicas ensures that the StatefulSet is updated with all changes from passed in etcd but the replicas set on the StatefulSet
// are taken from the passed in replicas and not from the etcd component.
func (r _resource) createOrPatchWithReplicas(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd, replicas int32) error {
	desiredStatefulSet := emptyStatefulSet(etcd)
	mutatingFn := func() error {
		if builder, err := newStsBuilder(r.client, ctx.Logger, etcd, replicas, r.useEtcdWrapper, r.imageVector, desiredStatefulSet); err != nil {
			return druiderr.WrapError(err,
				ErrSyncStatefulSet,
				"Sync",
				fmt.Sprintf("Error initializing StatefulSet builder for etcd %v", etcd.GetNamespaceName()))
		} else {
			return builder.Build(ctx)
		}
	}
	opResult, err := controllerutils.GetAndCreateOrStrategicMergePatch(ctx, r.client, desiredStatefulSet, mutatingFn)
	if err != nil {
		return druiderr.WrapError(err,
			ErrSyncStatefulSet,
			"Sync",
			fmt.Sprintf("Error creating or patching StatefulSet: %s for etcd: %v", desiredStatefulSet.Name, etcd.GetNamespaceName()))
	}

	ctx.Logger.Info("triggered creation of statefulSet", "statefulSet", getObjectKey(etcd), "operationResult", opResult)
	return nil
}

// createOrPatch updates StatefulSet taking changes from passed in etcd component.
func (r _resource) createOrPatch(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	return r.createOrPatchWithReplicas(ctx, etcd, etcd.Spec.Replicas)
}

func (r _resource) handlePeerTLSChanges(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd, existingSts *appsv1.StatefulSet) error {
	peerTLSEnabledForMembers, err := utils.IsPeerURLTLSEnabledForMembers(ctx, r.client, ctx.Logger, etcd.Namespace, etcd.Name, *existingSts.Spec.Replicas)
	if err != nil {
		return fmt.Errorf("error checking if peer URL TLS is enabled: %w", err)
	}

	if isPeerTLSEnablementPending(peerTLSEnabledForMembers, etcd) {
		if !isStatefulSetPatchedWithPeerTLSVolMount(existingSts) {
			// This step ensures that only STS is updated with secret volume mounts which gets added to the etcd component due to
			// enabling of TLS for peer communication. It preserves the current STS replicas.
			if err = r.createOrPatchWithReplicas(ctx, etcd, *existingSts.Spec.Replicas); err != nil {
				return fmt.Errorf("error creating or patching StatefulSet with TLS enabled: %w", err)
			}
		} else {
			ctx.Logger.Info("Secret volume mounts to enable Peer URL TLS have already been mounted. Skipping patching StatefulSet with secret volume mounts.")
		}
		return fmt.Errorf("peer URL TLS not enabled for #%d members for etcd: %v, requeuing reconcile request", *existingSts.Spec.Replicas, etcd.GetNamespaceName())
	}
	ctx.Logger.Info("Peer URL TLS has been enabled for all currently running members")
	return nil
}

func isStatefulSetPatchedWithPeerTLSVolMount(existingSts *appsv1.StatefulSet) bool {
	volumes := existingSts.Spec.Template.Spec.Volumes
	var peerURLCAEtcdVolPresent, peerURLEtcdServerTLSVolPresent bool
	for _, vol := range volumes {
		if vol.Name == common.VolumeNameEtcdPeerCA {
			peerURLCAEtcdVolPresent = true
		}
		if vol.Name == common.VolumeNameEtcdPeerServerTLS {
			peerURLEtcdServerTLSVolPresent = true
		}
	}
	return peerURLCAEtcdVolPresent && peerURLEtcdServerTLSVolPresent
}

// isPeerTLSEnablementPending checks if the peer URL TLS has been enabled for the etcd, but it has not yet reflected in all etcd members.
func isPeerTLSEnablementPending(peerTLSEnabledStatusFromMembers bool, etcd *druidv1alpha1.Etcd) bool {
	return !peerTLSEnabledStatusFromMembers && etcd.Spec.Etcd.PeerUrlTLS != nil
}

func emptyStatefulSet(etcd *druidv1alpha1.Etcd) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:            etcd.Name,
			Namespace:       etcd.Namespace,
			OwnerReferences: []metav1.OwnerReference{etcd.GetAsOwnerReference()},
		},
	}
}

func getObjectKey(etcd *druidv1alpha1.Etcd) client.ObjectKey {
	return client.ObjectKey{
		Name:      etcd.Name,
		Namespace: etcd.Namespace,
	}
}
