// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package statefulset

import (
	"fmt"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	druiderr "github.com/gardener/etcd-druid/internal/errors"
	"github.com/gardener/etcd-druid/internal/features"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/component-base/featuregate"

	"github.com/gardener/etcd-druid/internal/operator/component"
	"github.com/gardener/etcd-druid/internal/utils"
	"github.com/gardener/gardener/pkg/controllerutils"
	"github.com/gardener/gardener/pkg/utils/imagevector"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ErrGetStatefulSet    druidv1alpha1.ErrorCode = "ERR_GET_STATEFULSET"
	ErrDeleteStatefulSet druidv1alpha1.ErrorCode = "ERR_DELETE_STATEFULSET"
	ErrSyncStatefulSet   druidv1alpha1.ErrorCode = "ERR_SYNC_STATEFULSET"
)

type _resource struct {
	client         client.Client
	imageVector    imagevector.ImageVector
	useEtcdWrapper bool
}

func New(client client.Client, imageVector imagevector.ImageVector, featureGates map[featuregate.Feature]bool) component.Operator {
	return &_resource{
		client:         client,
		imageVector:    imageVector,
		useEtcdWrapper: featureGates[features.UseEtcdWrapper],
	}
}

func (r _resource) GetExistingResourceNames(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) ([]string, error) {
	resourceNames := make([]string, 0, 1)
	objectKey := getObjectKey(etcd)
	sts, err := r.getExistingStatefulSet(ctx, etcd)
	if err != nil {
		return nil, druiderr.WrapError(err,
			ErrGetStatefulSet,
			"GetExistingResourceNames",
			fmt.Sprintf("Error getting StatefulSet: %v for etcd: %v", objectKey, etcd.GetNamespaceName()))
	}
	if sts == nil {
		return resourceNames, nil
	}
	if metav1.IsControlledBy(sts, etcd) {
		resourceNames = append(resourceNames, sts.Name)
	}
	return resourceNames, nil
}

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

func (r _resource) TriggerDelete(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	objectKey := getObjectKey(etcd)
	ctx.Logger.Info("Triggering delete of StatefulSet", "objectKey", objectKey)
	err := r.client.Delete(ctx, emptyStatefulSet(etcd))
	if err != nil {
		if errors.IsNotFound(err) {
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

func (r _resource) getExistingStatefulSet(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) (*appsv1.StatefulSet, error) {
	sts := emptyStatefulSet(etcd)
	if err := r.client.Get(ctx, getObjectKey(etcd), sts); err != nil {
		if errors.IsNotFound(err) {
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
	peerTLSEnabledForAllMembers, err := utils.IsPeerURLTLSEnabledForAllMembers(ctx, r.client, ctx.Logger, etcd.Namespace, etcd.Name)
	if err != nil {
		return fmt.Errorf("error checking if peer URL TLS is enabled: %w", err)
	}

	if isPeerTLSChangedToEnabled(peerTLSEnabledForAllMembers, etcd) {
		if !isStatefulSetPatchedWithPeerTLSVolMount(existingSts) {
			// This step ensures that only STS is updated with secret volume mounts which gets added to the etcd component due to
			// enabling of TLS for peer communication. It preserves the current STS replicas.
			if err = r.createOrPatchWithReplicas(ctx, etcd, *existingSts.Spec.Replicas); err != nil {
				return fmt.Errorf("error creating or patching StatefulSet with TLS enabled: %w", err)
			}
		} else {
			ctx.Logger.Info("Secret volume mounts to enable Peer URL TLS have already been mounted. Skipping patching StatefulSet with secret volume mounts.")
		}
		// check again if peer TLS has been enabled for all members. If not then force a requeue of the reconcile request.
		if !peerTLSEnabledForAllMembers {
			return fmt.Errorf("peer URL TLS not enabled for all members for etcd: %v, requeuing reconcile request", etcd.GetNamespaceName())
		}

		//ctx.Logger.Info("Attempting to enable TLS for etcd peers", "namespace", etcd.Namespace, "name", etcd.Name)
		//
		//tlsEnabled, err := utils.IsPeerURLTLSEnabledForAllMembers(ctx, r.client, ctx.Logger, etcd.Namespace, etcd.Name)
		//if err != nil {
		//	return fmt.Errorf("error verifying if TLS is enabled post-patch: %v", err)
		//}
		//// It usually takes time for TLS to be enabled and reflected via the lease. So first time around this will not be true.
		//// So instead of waiting we requeue the request to be re-tried again.
		//if !tlsEnabled {
		//	return fmt.Errorf("failed to enable TLS for etcd [name: %s, namespace: %s]", etcd.Name, etcd.Namespace)
		//}
		//
		//if err := deleteAllStsPods(ctx, r.client, "Deleting all StatefulSet pods due to TLS enablement", existingSts); err != nil {
		//	return fmt.Errorf("error deleting StatefulSet pods after enabling TLS: %v", err)
		//}
		//ctx.Logger.Info("TLS enabled for etcd peers", "namespace", etcd.Namespace, "name", etcd.Name)
	}
	ctx.Logger.Info("Peer URL TLS has been enabled for all members")
	return nil
}

func isStatefulSetPatchedWithPeerTLSVolMount(existingSts *appsv1.StatefulSet) bool {
	volumes := existingSts.Spec.Template.Spec.Volumes
	var peerURLCAEtcdVolPresent, peerURLEtcdServerTLSVolPresent bool
	for _, vol := range volumes {
		if vol.Name == "peer-url-ca-etcd" {
			peerURLCAEtcdVolPresent = true
		}
		if vol.Name == "peer-url-etcd-server-tls" {
			peerURLEtcdServerTLSVolPresent = true
		}
	}
	return peerURLCAEtcdVolPresent && peerURLEtcdServerTLSVolPresent
}

// isPeerTLSChangedToEnabled checks if the Peer TLS setting has changed to enabled
func isPeerTLSChangedToEnabled(peerTLSEnabledStatusFromMembers bool, etcd *druidv1alpha1.Etcd) bool {
	return !peerTLSEnabledStatusFromMembers && etcd.Spec.Etcd.PeerUrlTLS != nil
}

func deleteAllStsPods(ctx component.OperatorContext, cl client.Client, opName string, sts *appsv1.StatefulSet) error {
	// Get all Pods belonging to the StatefulSet
	podList := &corev1.PodList{}
	listOpts := []client.ListOption{
		client.InNamespace(sts.Namespace),
		client.MatchingLabels(sts.Spec.Selector.MatchLabels),
	}

	if err := cl.List(ctx, podList, listOpts...); err != nil {
		ctx.Logger.Error(err, "Failed to list pods for StatefulSet", "StatefulSet", client.ObjectKeyFromObject(sts))
		return err
	}

	for _, pod := range podList.Items {
		if err := cl.Delete(ctx, &pod); err != nil {
			ctx.Logger.Error(err, "Failed to delete pod", "Pod", pod.Name, "Namespace", pod.Namespace)
			return err
		}
	}

	return nil
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
