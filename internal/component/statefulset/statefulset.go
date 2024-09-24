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
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/component-base/featuregate"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// ErrGetStatefulSet indicates an error in getting the statefulset resource.
	ErrGetStatefulSet druidv1alpha1.ErrorCode = "ERR_GET_STATEFULSET"
	// ErrPreSyncStatefulSet indicates an error in pre-sync operations for the statefulset resource.
	ErrPreSyncStatefulSet druidv1alpha1.ErrorCode = "ERR_PRESYNC_STATEFULSET"
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

// PreSync recreates the statefulset for the given Etcd, if label selector for the existing statefulset
// is different from the label selector required to be applied on it. This is because the statefulset's
// spec.selector field is immutable and cannot be updated on the existing statefulset.
func (r _resource) PreSync(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	ctx.Logger.Info("Running pre-sync for StatefulSet", "name", druidv1alpha1.GetStatefulSetName(etcd.ObjectMeta), "namespace", druidv1alpha1.GetNamespaceName(etcd.ObjectMeta))
	sts, err := r.getExistingStatefulSet(ctx, etcd.ObjectMeta)
	if err != nil {
		return druiderr.WrapError(err,
			ErrPreSyncStatefulSet,
			"PreSync",
			fmt.Sprintf("Error getting StatefulSet: %v for etcd: %v", getObjectKey(etcd.ObjectMeta), druidv1alpha1.GetNamespaceName(etcd.ObjectMeta)))
	}
	// If there is no STS then there is no need to handle any peer TLS change.
	if sts != nil {
		if err = r.handlePeerTLSChanges(ctx, etcd, sts); err != nil {
			return err
		}
	}
	if err = r.handleStsPodLabelsOnMismatch(ctx, etcd, sts); err != nil {
		return err
	}
	return nil
}

// Sync creates or updates the statefulset for the given Etcd.
func (r _resource) Sync(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	var (
	//existingSTS *appsv1.StatefulSet
	//err         error
	)
	//objectKey := getObjectKey(etcd.ObjectMeta)
	//if existingSTS, err = r.getExistingStatefulSet(ctx, etcd.ObjectMeta); err != nil {
	//	return druiderr.WrapError(err,
	//		ErrSyncStatefulSet,
	//		component.OperationSync,
	//		fmt.Sprintf("Error getting StatefulSet: %v for etcd: %v", objectKey, druidv1alpha1.GetNamespaceName(etcd.ObjectMeta)))
	//}
	// There is no StatefulSet present. Create one.
	//if existingSTS == nil {
	//	return r.createOrPatch(ctx, etcd)
	//}

	// StatefulSet exists, check if TLS has been enabled for peer communication, if yes then it is currently a multistep
	// process to ensure that all members are updated and establish peer TLS communication.
	//if err = r.handlePeerTLSChanges(ctx, etcd, existingSTS); err != nil {
	//	return err
	//}
	return r.createOrPatch(ctx, etcd)
}

// TriggerDelete triggers the deletion of the statefulset for the given Etcd.
func (r _resource) TriggerDelete(ctx component.OperatorContext, etcdObjMeta metav1.ObjectMeta) error {
	objectKey := getObjectKey(etcdObjMeta)
	ctx.Logger.Info("Triggering deletion of StatefulSet", "objectKey", objectKey)
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
	ctx.Logger.Info("deleted", "component", "statefulset", "objectKey", objectKey)
	return nil
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

func (r _resource) updateStsWithPeerTLSConfig(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd, sts *appsv1.StatefulSet) error {
	mutatingFn := func() error {
		if builder, err := newStsBuilder(r.client, ctx.Logger, etcd, *sts.Spec.Replicas, r.useEtcdWrapper, r.imageVector, sts); err != nil {
			return druiderr.WrapError(err,
				ErrPreSyncStatefulSet,
				component.OperationPreSync,
				fmt.Sprintf("Error initializing StatefulSet builder for etcd %v", druidv1alpha1.GetNamespaceName(etcd.ObjectMeta)))
		} else {
			return builder.UpdateWithPeerTLSEnabled(ctx)
		}
	}
	opResult, err := controllerutils.GetAndCreateOrStrategicMergePatch(ctx, r.client, sts, mutatingFn)
	if err != nil {
		return druiderr.WrapError(err,
			ErrPreSyncStatefulSet,
			component.OperationPreSync,
			fmt.Sprintf("Error updating StatefulSet: %s for etcd: %v", sts.Name, druidv1alpha1.GetNamespaceName(etcd.ObjectMeta)))
	}

	ctx.Logger.Info("triggered update of statefulSet to patch Peer TLS Volumes and Volume Mounts", "statefulSet", getObjectKey(etcd.ObjectMeta), "operationResult", opResult)
	return nil
}

// createOrPatchWithReplicas ensures that the StatefulSet is updated with all changes from passed in etcd but the replicas set on the StatefulSet
// are taken from the passed in replicas and not from the etcd component.
func (r _resource) createOrPatchWithReplicas(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd, replicas int32) error {
	desiredStatefulSet := emptyStatefulSet(etcd.ObjectMeta)
	mutatingFn := func() error {
		if builder, err := newStsBuilder(r.client, ctx.Logger, etcd, replicas, r.useEtcdWrapper, r.imageVector, desiredStatefulSet); err != nil {
			return druiderr.WrapError(err,
				ErrSyncStatefulSet,
				component.OperationSync,
				fmt.Sprintf("Error initializing StatefulSet builder for etcd %v", druidv1alpha1.GetNamespaceName(etcd.ObjectMeta)))
		} else {
			return builder.Build(ctx)
		}
	}
	opResult, err := controllerutils.GetAndCreateOrStrategicMergePatch(ctx, r.client, desiredStatefulSet, mutatingFn)
	if err != nil {
		return druiderr.WrapError(err,
			ErrSyncStatefulSet,
			component.OperationSync,
			fmt.Sprintf("Error creating or patching StatefulSet: %s for etcd: %v", desiredStatefulSet.Name, druidv1alpha1.GetNamespaceName(etcd.ObjectMeta)))
	}

	ctx.Logger.Info("triggered create/patch of statefulSet", "statefulSet", getObjectKey(etcd.ObjectMeta), "operationResult", opResult)
	return nil
}

// createOrPatch updates StatefulSet taking changes from passed in etcd component.
func (r _resource) createOrPatch(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	return r.createOrPatchWithReplicas(ctx, etcd, etcd.Spec.Replicas)
}

// At this point of time we do not expect that once TLS has been enabled it will be disabled thereafter.
// So the assumption here is that consumers will not disable TLS for peer communication once it has been enabled.
// If this assumption is no longer valid then we should change the implementation.
func (r _resource) handlePeerTLSChanges(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd, existingSts *appsv1.StatefulSet) error {
	if etcd.Spec.Etcd.PeerUrlTLS == nil {
		return nil
	}

	peerTLSEnabledForMembers, err := utils.IsPeerURLTLSEnabledForMembers(ctx, r.client, ctx.Logger, etcd)
	if err != nil {
		return druiderr.WrapError(err,
			ErrSyncStatefulSet,
			component.OperationSync,
			fmt.Sprintf("Error checking if peer TLS is enabled for statefulset: %v, etcd: %v", client.ObjectKeyFromObject(existingSts), client.ObjectKeyFromObject(etcd)))
	}

	if !peerTLSEnabledForMembers {
		if !isStatefulSetPatchedWithPeerTLSVolMount(existingSts) {
			// This step ensures that only STS is updated with secret volume mounts which gets added to the etcd component due to
			// enabling of TLS for peer communication. It preserves the current STS replicas.
			if err = r.updateStsWithPeerTLSConfig(ctx, etcd, existingSts); err != nil {
				return err
			}
		} else {
			ctx.Logger.Info("Secret volume mounts to enable Peer URL TLS have already been mounted. Skipping patching StatefulSet with secret volume mounts.")
		}
		return druiderr.New(
			druiderr.ErrRequeueAfter,
			component.OperationSync,
			fmt.Sprintf("Peer URL TLS not enabled for #%d members for etcd: %v, requeuing reconcile request", existingSts.Spec.Replicas, client.ObjectKeyFromObject(etcd)))
	}
	ctx.Logger.Info("Peer URL TLS has been enabled for all currently running members")
	return nil
}

func (r _resource) handleStsPodLabelsOnMismatch(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd, sts *appsv1.StatefulSet) error {
	if sts == nil {
		return nil
		// check if there is no STS because it got orphan deleted in the previous PreSync step and the subsequent creation of STS is pending or failed.
		//orphanPodsObjMeta, err := r.getOrphanedPodsPartialObjMeta(ctx, etcd)
		//if err != nil {
		//	return err
		//}
		//numOrphanedPods := len(orphanPodsObjMeta)
		//// there is nothing to be done. Let the creation of STS be done by Sync step.
		//if numOrphanedPods == 0 {
		//	return nil
		//}
		//// STS is pending creation, so we need to create it with the previous replicas. We derive the previous replicas from the number of existing member leases.
		//// We cannot depend upon the number of pods as a potential node crash or node drain would evict this pod.
		//previousReplicas, err := r.getPreviousReplicasFromMemberLeases(ctx, etcd)
		//if err != nil {
		//	return err
		//}
		//return r.createOrPatchWithReplicas(ctx, etcd, int32(previousReplicas))
	}
	if hasPodSelectorLabelOrPodTemplateLabelChanged(etcd, sts) {
		//existingReplicas := *sts.Spec.Replicas
		if err := r.orphanDeleteSts(ctx, sts); err != nil {
			return err
		}
		//return r.createOrPatchWithReplicas(ctx, etcd, existingReplicas)
	}
	return nil
}

func (r _resource) getOrphanedPodsPartialObjMeta(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) ([]metav1.PartialObjectMetadata, error) {
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

func (r _resource) getPreviousReplicasFromMemberLeases(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) (int, error) {
	memberLeasesObjMeta, err := utils.ListAllMemberLeaseObjectMeta(ctx, r.client, etcd)
	if err != nil {
		return 0, druiderr.WrapError(err,
			ErrPreSyncStatefulSet,
			component.OperationPreSync,
			fmt.Sprintf("Error determining previous replicas from member leases for etcd: %v", druidv1alpha1.GetNamespaceName(etcd.ObjectMeta)))
	}
	return len(memberLeasesObjMeta), nil
}

func hasPodSelectorLabelOrPodTemplateLabelChanged(etcd *druidv1alpha1.Etcd, sts *appsv1.StatefulSet) bool {
	desiredPodTemplateLabels := getDesiredPodTemplateLabels(etcd)
	podTemplateSpecLabelsChanged := !utils.ContainsAllDesiredLabels(sts.Spec.Template.Labels, desiredPodTemplateLabels)
	podSelectorLabelsChanged := !labels.Equals(sts.Spec.Selector.MatchLabels, druidv1alpha1.GetDefaultLabels(etcd.ObjectMeta))
	return podTemplateSpecLabelsChanged || podSelectorLabelsChanged
}

func (r _resource) orphanDeleteSts(ctx component.OperatorContext, sts *appsv1.StatefulSet) error {
	ctx.Logger.Info("Deleting StatefulSet for recreation later, as label selector has changed", "objectKey", client.ObjectKeyFromObject(sts))
	if err := r.client.Delete(ctx, sts, client.PropagationPolicy(metav1.DeletePropagationOrphan)); err != nil {
		return druiderr.WrapError(err,
			ErrPreSyncStatefulSet,
			component.OperationPreSync,
			fmt.Sprintf("Error orphan deleting StatefulSet: %v for etcd: %v", client.ObjectKeyFromObject(sts), client.ObjectKeyFromObject(sts)))
	}
	// check if sts has been orphan delete. If not then requeue.
	if _, err := r.getExistingStatefulSet(ctx, sts.ObjectMeta); err != nil {
		if apierrors.IsNotFound(err) {
			ctx.Logger.Info("StatefulSet has been orphan deleted", "objectKey", client.ObjectKeyFromObject(sts))
			return nil
		}
		return druiderr.WrapError(err,
			ErrPreSyncStatefulSet,
			component.OperationPreSync,
			fmt.Sprintf("Error checking if StatefulSet has been orphan deleted: %v for etcd: %v", client.ObjectKeyFromObject(sts), client.ObjectKeyFromObject(sts)))
	}
	return druiderr.New(
		druiderr.ErrRequeueAfter,
		component.OperationPreSync,
		fmt.Sprintf("StatefulSet has not been orphan deleted: %v for etcd: %v, requeuing reconcile request", client.ObjectKeyFromObject(sts), client.ObjectKeyFromObject(sts)))
}

func isStatefulSetPatchedWithPeerTLSVolMount(sts *appsv1.StatefulSet) bool {
	volumes := sts.Spec.Template.Spec.Volumes
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

func (r _resource) checkAndPatchStsPodLabelsOnMismatch(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd, sts *appsv1.StatefulSet) error {
	desiredPodTemplateLabels := getDesiredPodTemplateLabels(etcd)
	if !utils.ContainsAllDesiredLabels(sts.Spec.Template.Labels, desiredPodTemplateLabels) {
		ctx.Logger.Info("Patching StatefulSet with new pod labels", "objectKey", getObjectKey(etcd.ObjectMeta))
		originalSts := sts.DeepCopy()
		sts.Spec.Template.Labels = utils.MergeMaps(sts.Spec.Template.Labels, desiredPodTemplateLabels)
		if err := r.client.Patch(ctx, sts, client.MergeFrom(originalSts)); err != nil {
			return err
		}
	}
	return nil
}

func getDesiredPodTemplateLabels(etcd *druidv1alpha1.Etcd) map[string]string {
	return utils.MergeMaps(etcd.Spec.Labels, getStatefulSetLabels(etcd.Name))
}

func (r _resource) doStatefulSetPodsHaveDesiredLabels(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd, sts *appsv1.StatefulSet) (bool, error) {
	// sts.spec.replicas is more accurate than Etcd.spec.replicas, specifically when
	// Etcd.spec.replicas is updated but not yet reflected in the etcd cluster
	if sts.Spec.Replicas == nil {
		return false, fmt.Errorf("statefulset %s does not have a replicas count defined", sts.Name)
	}
	podNames := druidv1alpha1.GetAllPodNames(etcd.ObjectMeta, *sts.Spec.Replicas)
	desiredLabels := getDesiredPodTemplateLabels(etcd)
	for _, podName := range podNames {
		pod := &corev1.Pod{}
		if err := r.client.Get(ctx, client.ObjectKey{Name: podName, Namespace: etcd.Namespace}, pod); err != nil {
			return false, err
		}
		if !utils.ContainsAllDesiredLabels(pod.Labels, desiredLabels) {
			return false, nil
		}
	}
	return true, nil
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
