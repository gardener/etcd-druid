// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package peerservice

import (
	"fmt"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"
	"github.com/gardener/etcd-druid/internal/component"
	druiderr "github.com/gardener/etcd-druid/internal/errors"
	"github.com/gardener/etcd-druid/internal/utils"

	"github.com/gardener/gardener/pkg/controllerutils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// ErrGetPeerService indicates an error in getting the peer service resource.
	ErrGetPeerService druidv1alpha1.ErrorCode = "ERR_GET_PEER_SERVICE"
	// ErrSyncPeerService indicates an error in syncing the peer service resource.
	ErrSyncPeerService druidv1alpha1.ErrorCode = "ERR_SYNC_PEER_SERVICE"
	// ErrDeletePeerService indicates an error in deleting the peer service resource.
	ErrDeletePeerService druidv1alpha1.ErrorCode = "ERR_DELETE_PEER_SERVICE"
)

type _resource struct {
	client client.Client
}

// New returns a new peer service component operator.
func New(client client.Client) component.Operator {
	return &_resource{
		client: client,
	}
}

// GetExistingResourceNames returns the name of the existing peer service for the given Etcd.
func (r _resource) GetExistingResourceNames(ctx component.OperatorContext, etcdObjMeta metav1.ObjectMeta) ([]string, error) {
	resourceNames := make([]string, 0, 1)
	svcObjectKey := getObjectKey(etcdObjMeta)
	objMeta := &metav1.PartialObjectMetadata{}
	objMeta.SetGroupVersionKind(corev1.SchemeGroupVersion.WithKind("Service"))
	if err := r.client.Get(ctx, svcObjectKey, objMeta); err != nil {
		if errors.IsNotFound(err) {
			return resourceNames, nil
		}
		return resourceNames, druiderr.WrapError(err,
			ErrGetPeerService,
			component.OperationGetExistingResourceNames,
			fmt.Sprintf("Error getting peer service: %s for etcd: %v", svcObjectKey.Name, druidv1alpha1.GetNamespaceName(etcdObjMeta)))
	}
	if metav1.IsControlledBy(objMeta, &etcdObjMeta) {
		resourceNames = append(resourceNames, objMeta.Name)
	}
	return resourceNames, nil
}

// PreSync is a no-op for the peer service component.
func (r _resource) PreSync(_ component.OperatorContext, _ *druidv1alpha1.Etcd) error { return nil }

// Sync creates or updates the peer service for the given Etcd.
func (r _resource) Sync(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	objectKey := getObjectKey(etcd.ObjectMeta)
	svc := emptyPeerService(objectKey)
	result, err := controllerutils.GetAndCreateOrStrategicMergePatch(ctx, r.client, svc, func() error {
		buildResource(etcd, svc)
		return nil
	})
	if err != nil {
		return druiderr.WrapError(err,
			ErrSyncPeerService,
			component.OperationSync,
			fmt.Sprintf("Error during create or update of peer service: %v for etcd: %v", objectKey, druidv1alpha1.GetNamespaceName(etcd.ObjectMeta)),
		)
	}
	ctx.Logger.Info("synced", "component", "peer-service", "objectKey", objectKey, "result", result)
	return nil
}

// TriggerDelete triggers the deletion of the peer service for the given Etcd.
func (r _resource) TriggerDelete(ctx component.OperatorContext, etcdObjMeta metav1.ObjectMeta) error {
	objectKey := getObjectKey(etcdObjMeta)
	ctx.Logger.Info("Triggering deletion of peer service")
	if err := r.client.Delete(ctx, emptyPeerService(objectKey)); err != nil {
		if errors.IsNotFound(err) {
			ctx.Logger.Info("No Peer Service found, Deletion is a No-Op", "objectKey", objectKey)
			return nil
		}
		return druiderr.WrapError(
			err,
			ErrDeletePeerService,
			component.OperationTriggerDelete,
			fmt.Sprintf("Failed to delete peer service: %v for etcd: %v", objectKey, druidv1alpha1.GetNamespaceName(etcdObjMeta)),
		)
	}
	ctx.Logger.Info("deleted", "component", "peer-service", "objectKey", objectKey)
	return nil
}

func buildResource(etcd *druidv1alpha1.Etcd, svc *corev1.Service) {
	svc.Labels = getLabels(etcd)
	svc.OwnerReferences = []metav1.OwnerReference{druidv1alpha1.GetAsOwnerReference(etcd.ObjectMeta)}
	svc.Spec.Type = corev1.ServiceTypeClusterIP
	svc.Spec.ClusterIP = corev1.ClusterIPNone
	svc.Spec.SessionAffinity = corev1.ServiceAffinityNone
	// Peer service should only target StatefulSet pods. Default labels are going to be present on anything that is managed by etcd-druid and started for an etcd cluster.
	// Therefore, only using default labels as label selector can cause issues as we have already seen in https://github.com/gardener/etcd-druid/issues/914
	svc.Spec.Selector = utils.MergeMaps(druidv1alpha1.GetDefaultLabels(etcd.ObjectMeta), map[string]string{druidv1alpha1.LabelComponentKey: common.ComponentNameStatefulSet})
	svc.Spec.PublishNotReadyAddresses = true
	svc.Spec.Ports = getPorts(etcd)
}

func getLabels(etcd *druidv1alpha1.Etcd) map[string]string {
	svcLabels := map[string]string{
		druidv1alpha1.LabelComponentKey: common.ComponentNamePeerService,
		druidv1alpha1.LabelAppNameKey:   druidv1alpha1.GetPeerServiceName(etcd.ObjectMeta),
	}
	return utils.MergeMaps(druidv1alpha1.GetDefaultLabels(etcd.ObjectMeta), svcLabels)
}

func getObjectKey(obj metav1.ObjectMeta) client.ObjectKey {
	return client.ObjectKey{Name: druidv1alpha1.GetPeerServiceName(obj), Namespace: obj.Namespace}
}

func emptyPeerService(objectKey client.ObjectKey) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      objectKey.Name,
			Namespace: objectKey.Namespace,
		},
	}
}

func getPorts(etcd *druidv1alpha1.Etcd) []corev1.ServicePort {
	peerPort := ptr.Deref(etcd.Spec.Etcd.ServerPort, common.DefaultPortEtcdPeer)
	return []corev1.ServicePort{
		{
			Name:       "peer",
			Protocol:   corev1.ProtocolTCP,
			Port:       peerPort,
			TargetPort: intstr.FromInt(int(peerPort)),
		},
	}
}
