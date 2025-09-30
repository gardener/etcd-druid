// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package statefulset

import (
	"fmt"
	"slices"

	druidconfigv1alpha1 "github.com/gardener/etcd-druid/api/config/v1alpha1"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"
	"github.com/gardener/etcd-druid/internal/component"
	druiderr "github.com/gardener/etcd-druid/internal/errors"
	"github.com/gardener/etcd-druid/internal/utils/imagevector"
	"github.com/gardener/etcd-druid/internal/utils/kubernetes"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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
	client      client.Client
	imageVector imagevector.ImageVector
	logger      logr.Logger
}

// New returns a new statefulset component operator.
func New(client client.Client, imageVector imagevector.ImageVector) component.Operator {
	return &_resource{
		client:      client,
		imageVector: imageVector,
	}
}

// GetExistingResourceNames returns the name of the existing statefulset for the given Etcd.
func (r _resource) GetExistingResourceNames(ctx component.OperatorContext, etcdObjMeta metav1.ObjectMeta) ([]string, error) {
	resourceNames := make([]string, 0, 1)
	objectKey := getObjectKey(etcdObjMeta)
	objMeta := &metav1.PartialObjectMetadata{}
	objMeta.SetGroupVersionKind(appsv1.SchemeGroupVersion.WithKind("StatefulSet"))
	if err := r.client.Get(ctx, objectKey, objMeta); err != nil {
		if apierrors.IsNotFound(err) {
			return resourceNames, nil
		}
		return nil, druiderr.WrapError(err,
			ErrGetStatefulSet,
			component.OperationGetExistingResourceNames,
			fmt.Sprintf("Error getting StatefulSet: %v for etcd: %v", objectKey, druidv1alpha1.GetNamespaceName(etcdObjMeta)))
	}
	if metav1.IsControlledBy(objMeta, &etcdObjMeta) {
		resourceNames = append(resourceNames, objMeta.Name)
	}
	return resourceNames, nil
}

// PreSync is a no-op for the statefulset component.
func (r _resource) PreSync(_ component.OperatorContext, _ *druidv1alpha1.Etcd) error {
	return nil
}

// Sync creates or updates the statefulset for the given Etcd.
func (r _resource) Sync(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	r.logger = ctx.Logger.WithValues("component", component.StatefulSetKind, "operation", component.OperationSync)
	objectKey := getObjectKey(etcd.ObjectMeta)
	existingSTS, err := r.getExistingStatefulSet(ctx, etcd.ObjectMeta)
	if err != nil {
		return druiderr.WrapError(err,
			ErrSyncStatefulSet,
			component.OperationSync,
			fmt.Sprintf("Error getting StatefulSet: %v for etcd: %v", objectKey, druidv1alpha1.GetNamespaceName(etcd.ObjectMeta)))
	}
	// There is no StatefulSet present. Create one.
	if existingSTS == nil {
		// Check etcd observed generation to determine if the etcd cluster is new or not.
		if etcd.Status.ObservedGeneration == nil {
			r.logger.Info("ObservedGeneration has not yet been set, triggering the creation of StatefulSet assuming a new etcd cluster")
			return r.createOrPatch(ctx, etcd)
		}
		// If Etcd resource has previously being reconciled successfully (indicated by a non-nil etcd.Status.ObservedGeneration)
		// then check if the STS is missing due to it being orphan deleted in the previous reconcile run. If so, recreate the STS.
		if err = r.checkAndRecreateOrphanDeletedSts(ctx, etcd); err != nil {
			return err
		}
	}

	if existingSTS != nil {
		if err = r.handleTLSChanges(ctx, etcd, existingSTS); err != nil {
			return err
		}
		if !labels.Equals(existingSTS.Spec.Selector.MatchLabels, druidv1alpha1.GetDefaultLabels(etcd.ObjectMeta)) {
			if err = r.handleStsLabelSelectorOnMismatch(ctx, etcd, existingSTS); err != nil {
				return err
			}
		}
		if druidconfigv1alpha1.DefaultFeatureGates.IsEnabled(druidconfigv1alpha1.AllowEmptyDir) {
			if err = r.handleEmptyDirVolumeChanges(ctx, etcd, existingSTS); err != nil {
				return err
			}
		}
	}

	return r.createOrPatch(ctx, etcd)
}

// TriggerDelete triggers the deletion of the statefulset for the given Etcd.
func (r _resource) TriggerDelete(ctx component.OperatorContext, etcdObjMeta metav1.ObjectMeta) error {
	r.logger = ctx.Logger.WithValues("component", component.StatefulSetKind, "operation", component.OperationTriggerDelete)
	objectKey := getObjectKey(etcdObjMeta)
	r.logger.Info("Triggering deletion of StatefulSet")
	if err := r.client.Delete(ctx, emptyStatefulSet(etcdObjMeta)); err != nil {
		if apierrors.IsNotFound(err) {
			ctx.Logger.Info("No StatefulSet found, Deletion is a No-Op", "objectKey", objectKey.Name)
			return nil
		}
		return druiderr.WrapError(err,
			ErrDeleteStatefulSet,
			component.OperationTriggerDelete,
			fmt.Sprintf("Failed to delete StatefulSet: %v for etcd %v", objectKey, druidv1alpha1.GetNamespaceName(etcdObjMeta)))
	}
	r.logger.Info("deletion successful")
	return nil
}

func (r _resource) handleStsLabelSelectorOnMismatch(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd, sts *appsv1.StatefulSet) error {
	r.logger.Info("Orphan deleting StatefulSet for recreation later, as label selector has changed", "oldSelector.MatchLabels", sts.Spec.Selector.MatchLabels, "newOldSelector.MatchLabels", druidv1alpha1.GetDefaultLabels(etcd.ObjectMeta))
	if err := r.client.Delete(ctx, sts, client.PropagationPolicy(metav1.DeletePropagationOrphan)); err != nil {
		return druiderr.WrapError(err,
			ErrSyncStatefulSet,
			component.OperationSync,
			fmt.Sprintf("Error orphan deleting StatefulSet: %v for etcd: %v", client.ObjectKeyFromObject(sts), client.ObjectKeyFromObject(sts)))
	}
	// Requeue the reconcile request to ensure that the STS is orphan deleted.
	return druiderr.New(
		druiderr.ErrRequeueAfter,
		component.OperationSync,
		fmt.Sprintf("StatefulSet has not been orphan deleted: %v for etcd: %v, requeuing reconcile request", client.ObjectKeyFromObject(sts), client.ObjectKeyFromObject(sts)))
}

// handleEmptyDirVolumeChanges checks if the etcd spec has changed to specify the EmptyDirVolumeSource field.
// If specified, and the statefulset currently uses CSI volumes, it is orphan deleted.
// The statefulset created in a subsequent Sync specifies emptyDir volumes for the pods, and a rolling update is triggered.
// The rolling update makes the etcd pods use emptyDir volumes.
func (r _resource) handleEmptyDirVolumeChanges(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd, existingSTS *appsv1.StatefulSet) error {
	emptyDirSpecified, emptyDirInUse := etcd.Spec.EmptyDirVolumeSource != nil, false
	for _, volume := range existingSTS.Spec.Template.Spec.Volumes {
		if volume.EmptyDir != nil {
			emptyDirInUse = true
		}
	}
	// The specification in the etcd spec matches that in the statefulset
	if emptyDirSpecified == emptyDirInUse {
		return nil
	}
	r.logger.Info("The etcd specification requests emptyDir volumes, while CSI volumes are currently in use. Triggering rolling update.")
	if err := r.client.Delete(ctx, existingSTS, client.PropagationPolicy(metav1.DeletePropagationOrphan)); err != nil {
		return druiderr.WrapError(
			err,
			ErrSyncStatefulSet,
			component.OperationSync,
			fmt.Sprintf("error orphan deleting StatefulSet: %v for etcd: %v", client.ObjectKeyFromObject(existingSTS), client.ObjectKeyFromObject(etcd)),
		)
	}
	// Requeue to ensure the orphan deletion, and the subsequent recreation of the statefulset.
	return druiderr.New(
		druiderr.ErrRequeueAfter,
		component.OperationSync,
		fmt.Sprintf("Requeue to ensure orphan deletion for StatefulSet: %v for etcd: %v", client.ObjectKeyFromObject(existingSTS), client.ObjectKeyFromObject(etcd)),
	)
}

func (r _resource) checkAndRecreateOrphanDeletedSts(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	numOrphanedPods, err := r.determineNumOrphanedPods(ctx, etcd)
	if err != nil {
		return err
	}
	if numOrphanedPods > 0 {
		r.logger.Info("Recreating StatefulSet with previous replicas to adopt orphan pods", "numOrphanedPods", numOrphanedPods)
		sts := emptyStatefulSet(etcd.ObjectMeta)
		// #nosec G115 -- numOrphanedPods will never cross the size of int32, so conversion is safe.
		if err = r.createOrPatchWithReplicas(ctx, etcd, sts, int32(numOrphanedPods), false); err != nil {
			return druiderr.WrapError(err,
				ErrSyncStatefulSet,
				component.OperationSync,
				fmt.Sprintf("Error recreating StatefulSet with previous replicas for orphan pods adoption for etcd: %v", druidv1alpha1.GetNamespaceName(etcd.ObjectMeta)))
		}
		return druiderr.New(
			druiderr.ErrRequeueAfter,
			component.OperationSync,
			fmt.Sprintf("StatefulSet has not yet been created or is not ready with previous replicas for etcd: %v, requeuing reconcile request", druidv1alpha1.GetNamespaceName(etcd.ObjectMeta)))
	}
	r.logger.Info("There is no STS and no orphaned pods found. Skipping recreation as none is required.")
	return nil
}

func (r _resource) determineNumOrphanedPods(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) (int, error) {
	orphanedPodsObjMeta, err := r.getStsPodsObjMeta(ctx, etcd)
	if err != nil {
		return 0, druiderr.WrapError(err,
			ErrSyncStatefulSet,
			component.OperationSync,
			fmt.Sprintf("Error getting orphaned pods for etcd: %v", client.ObjectKeyFromObject(etcd)))
	}
	if len(orphanedPodsObjMeta) > 0 {
		r.logger.Info("Orphaned pods found", "numOrphanedPods", len(orphanedPodsObjMeta))
		// If there are orphaned pods then determine the correct number of orphaned pods by first looking at etcd.status.members instead of depending
		// on the number of orphaned pods. It is quite possible that a subset of orphan pods have been evicted due to node crash or other reasons.
		return len(etcd.Status.Members), nil
	}
	return 0, nil
}

func (r _resource) getStsPodsObjMeta(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) ([]metav1.PartialObjectMetadata, error) {
	objMetaList := &metav1.PartialObjectMetadataList{}
	objMetaList.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("Pod"))
	if err := r.client.List(ctx,
		objMetaList,
		client.InNamespace(etcd.Namespace),
		client.MatchingLabels(druidv1alpha1.GetDefaultLabels(etcd.ObjectMeta)),
	); err != nil {
		if apierrors.IsNotFound(err) {
			return []metav1.PartialObjectMetadata{}, nil
		}
		return nil, err
	}
	return objMetaList.Items, nil
}

// getExistingStatefulSet gets the existing statefulset if it exists.
// If it is not found, it simply returns nil. Any other errors are returned as is.
func (r _resource) getExistingStatefulSet(ctx component.OperatorContext, etcdObjMeta metav1.ObjectMeta) (*appsv1.StatefulSet, error) {
	sts := emptyStatefulSet(etcdObjMeta)
	if err := r.client.Get(ctx, getObjectKey(etcdObjMeta), sts); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return sts, nil
}

// createOrPatchWithReplicas ensures that the StatefulSet is updated with all changes from passed in etcd but the replicas set on the StatefulSet
// are taken from the passed in replicas and not from the etcd component.
func (r _resource) createOrPatchWithReplicas(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd, sts *appsv1.StatefulSet, replicas int32, skipSetOrUpdateForbiddenFields bool) error {
	stsClone := sts.DeepCopy()
	mutatingFn := func() error {
		if builder, err := newStsBuilder(r.client, ctx.Logger, etcd, replicas, r.imageVector, skipSetOrUpdateForbiddenFields, stsClone); err != nil {
			return err
		} else {
			return builder.Build(ctx)
		}
	}

	opResult, err := controllerutil.CreateOrPatch(ctx, r.client, stsClone, mutatingFn)
	if err != nil {
		return err
	}
	r.logger.Info("triggered create/patch of statefulSet", "operationResult", opResult)
	return nil
}

// createOrPatch updates StatefulSet taking changes from passed in etcd component.
func (r _resource) createOrPatch(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	sts := emptyStatefulSet(etcd.ObjectMeta)
	if err := r.createOrPatchWithReplicas(ctx, etcd, sts, etcd.Spec.Replicas, false); err != nil {
		return druiderr.WrapError(err,
			ErrSyncStatefulSet,
			component.OperationSync,
			fmt.Sprintf("Error creating or patching [StatefulSet: %v, Replicas: %d] for etcd: %v", client.ObjectKeyFromObject(etcd), etcd.Spec.Replicas, client.ObjectKeyFromObject(etcd)))
	}
	return nil
}

func (r _resource) handleTLSChanges(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd, existingSts *appsv1.StatefulSet) error {
	// There are no replicas and there is no need to handle any TLS changes. Once replicas are increased then new pods will automatically have the TLS changes.
	if etcd.Spec.Replicas == 0 {
		r.logger.Info("Skipping handling TLS changes for StatefulSet as replicas are set to 0")
		return nil
	}

	isSTSTLSConfigInSync := isStatefulSetTLSConfigInSync(etcd, existingSts)
	if !isSTSTLSConfigInSync {
		// check if the etcd cluster is in a state where it can handle TLS changes.
		// If the peer URL TLS has changed and there are more than 1 replicas in the etcd cluster. Then wait for all members to be ready.
		// If we do not wait for all members to be ready patching STS to reflect peer TLS changes will cause rolling update which will never finish
		// and the cluster will be stuck in a bad state. Updating peer URL is a cluster wide operation as all members will need to know that a peer TLS has changed.
		// If not all members are ready then rolling-update of StatefulSet can potentially cause a healthy node to be restarted causing loss of quorum from which
		// there will not be an automatic recovery.
		if !r.hasTLSEnablementForPeerURLReflectedOnSTS(etcd, existingSts) && shouldRequeueForMultiNodeEtcdIfPodsNotReady(existingSts) {
			return druiderr.New(
				druiderr.ErrRequeueAfter,
				component.OperationSync,
				fmt.Sprintf("Not all etcd cluster members are ready. It is not safe to patch STS for Peer URL TLS changes. Replicas: %d, ReadyReplicas: %d", *existingSts.Spec.Replicas, existingSts.Status.ReadyReplicas))
		}
		r.logger.Info("TLS configuration is not in sync, updating StatefulSet with TLS changes")
		if err := r.createOrPatchWithReplicas(ctx, etcd, existingSts, *existingSts.Spec.Replicas, true); err != nil {
			return druiderr.WrapError(err,
				ErrSyncStatefulSet,
				component.OperationSync,
				fmt.Sprintf("Error creating or patching StatefulSet with TLS changes for StatefulSet: %v, etcd: %v", client.ObjectKeyFromObject(existingSts), client.ObjectKeyFromObject(etcd)))
		}
		return druiderr.New(
			druiderr.ErrRequeueAfter,
			component.OperationSync,
			fmt.Sprintf("Updated TLS config for etcd: %v, requeuing reconcile request", client.ObjectKeyFromObject(etcd)))
	}
	peerTLSInSyncForAllMembers, err := kubernetes.IsPeerURLInSyncForAllMembers(ctx, r.client, ctx.Logger, etcd, *existingSts.Spec.Replicas)
	if err != nil {
		return druiderr.WrapError(err,
			ErrSyncStatefulSet,
			component.OperationSync,
			fmt.Sprintf("Error checking if peer TLS is enabled for statefulset: %v, etcd: %v", client.ObjectKeyFromObject(existingSts), client.ObjectKeyFromObject(etcd)))
	}
	if peerTLSInSyncForAllMembers {
		r.logger.Info("Peer URL TLS configuration is reflected on all currently running members")
		return nil
	} else {
		return druiderr.New(
			druiderr.ErrRequeueAfter,
			component.OperationSync,
			fmt.Sprintf("Peer URL TLS not enabled for #%d members for etcd: %v, requeuing reconcile request", *existingSts.Spec.Replicas, client.ObjectKeyFromObject(etcd)))
	}
}

func shouldRequeueForMultiNodeEtcdIfPodsNotReady(sts *appsv1.StatefulSet) bool {
	return sts.Spec.Replicas != nil &&
		*sts.Spec.Replicas > 1 &&
		sts.Status.ReadyReplicas > 0 &&
		sts.Status.ReadyReplicas < *sts.Spec.Replicas
}

func (r _resource) hasTLSEnablementForPeerURLReflectedOnSTS(etcd *druidv1alpha1.Etcd, existingSts *appsv1.StatefulSet) bool {
	newEtcdWrapperPeerTLSVolMounts := getEtcdContainerPeerVolumeMounts(etcd)
	containerPeerTLSVolMounts := kubernetes.GetEtcdContainerPeerTLSVolumeMounts(existingSts)
	r.logger.Info("Checking if peer URL TLS enablement is reflected on StatefulSet", "len(newEtcdWrapperPeerTLSVolMounts)", len(newEtcdWrapperPeerTLSVolMounts), "len(containerPeerTLSVolMounts)", len(containerPeerTLSVolMounts))
	return len(containerPeerTLSVolMounts) == len(newEtcdWrapperPeerTLSVolMounts)
}

func isStatefulSetTLSConfigInSync(etcd *druidv1alpha1.Etcd, existingSts *appsv1.StatefulSet) bool {
	newEtcdbrTLSVolMounts := getBackupRestoreContainerSecretVolumeMounts(etcd)
	newEtcdWrapperTLSVolMounts := getEtcdContainerSecretVolumeMounts(etcd)
	containerTLSVolMounts := kubernetes.GetStatefulSetContainerTLSVolumeMounts(existingSts)
	return !hasTLSVolumeMountsChanged(containerTLSVolMounts[common.ContainerNameEtcd], newEtcdWrapperTLSVolMounts) &&
		!hasTLSVolumeMountsChanged(containerTLSVolMounts[common.ContainerNameEtcdBackupRestore], newEtcdbrTLSVolMounts)
}

func hasTLSVolumeMountsChanged(existingVolMounts, newVolMounts []corev1.VolumeMount) bool {
	if len(existingVolMounts) != len(newVolMounts) {
		return true
	}
	for _, newVolMount := range newVolMounts {
		if !slices.ContainsFunc(existingVolMounts, func(existingVolMount corev1.VolumeMount) bool {
			return existingVolMount.Name == newVolMount.Name && existingVolMount.MountPath == newVolMount.MountPath
		}) {
			return true
		}
	}
	return false
}

func emptyStatefulSet(obj metav1.ObjectMeta) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:            druidv1alpha1.GetStatefulSetName(obj),
			Namespace:       obj.Namespace,
			OwnerReferences: []metav1.OwnerReference{druidv1alpha1.GetAsOwnerReference(obj)},
		},
	}
}

func getObjectKey(obj metav1.ObjectMeta) client.ObjectKey {
	return client.ObjectKey{
		Name:      obj.Name,
		Namespace: obj.Namespace,
	}
}
