// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package peerservice

import (
	"fmt"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"
	druiderr "github.com/gardener/etcd-druid/internal/errors"
	"github.com/gardener/etcd-druid/internal/operator/component"
	"github.com/gardener/etcd-druid/internal/utils"
	"github.com/gardener/gardener/pkg/controllerutils"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const defaultServerPort = 2380

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

// New returns a new peer service operator.
func New(client client.Client) component.Operator {
	return &_resource{
		client: client,
	}
}

// GetExistingResourceNames returns the name of the existing peer service for the given Etcd.
func (r _resource) GetExistingResourceNames(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) ([]string, error) {
	resourceNames := make([]string, 0, 1)
	svcObjectKey := getObjectKey(etcd)
	svc := &corev1.Service{}
	if err := r.client.Get(ctx, svcObjectKey, svc); err != nil {
		if errors.IsNotFound(err) {
			return resourceNames, nil
		}
		return resourceNames, druiderr.WrapError(err,
			ErrGetPeerService,
			"GetExistingResourceNames",
			fmt.Sprintf("Error getting peer service: %s for etcd: %v", svcObjectKey.Name, etcd.GetNamespaceName()))
	}
	if metav1.IsControlledBy(svc, etcd) {
		resourceNames = append(resourceNames, svc.Name)
	}
	return resourceNames, nil
}

// Sync creates or updates the peer service for the given Etcd.
func (r _resource) Sync(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	objectKey := getObjectKey(etcd)
	svc := emptyPeerService(objectKey)
	result, err := controllerutils.GetAndCreateOrStrategicMergePatch(ctx, r.client, svc, func() error {
		buildResource(etcd, svc)
		return nil
	})
	if err != nil {
		return druiderr.WrapError(err,
			ErrSyncPeerService,
			"Sync",
			fmt.Sprintf("Error during create or update of peer service: %v for etcd: %v", objectKey, etcd.GetNamespaceName()),
		)
	}
	ctx.Logger.Info("synced", "component", "peer-service", "objectKey", objectKey, "result", result)
	return nil
}

// TriggerDelete triggers the deletion of the peer service for the given Etcd.
func (r _resource) TriggerDelete(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	objectKey := getObjectKey(etcd)
	ctx.Logger.Info("Triggering delete of peer service")
	err := r.client.Delete(ctx, emptyPeerService(objectKey))
	if err != nil {
		if errors.IsNotFound(err) {
			ctx.Logger.Info("No Peer Service found, Deletion is a No-Op", "objectKey", objectKey)
			return nil
		}
		return druiderr.WrapError(
			err,
			ErrDeletePeerService,
			"TriggerDelete",
			fmt.Sprintf("Failed to delete peer service: %v for etcd: %v", objectKey, etcd.GetNamespaceName()),
		)
	}
	ctx.Logger.Info("deleted", "component", "peer-service", "objectKey", objectKey)
	return nil
}

func buildResource(etcd *druidv1alpha1.Etcd, svc *corev1.Service) {
	svc.Labels = getLabels(etcd)
	svc.OwnerReferences = []metav1.OwnerReference{etcd.GetAsOwnerReference()}
	svc.Spec.Type = corev1.ServiceTypeClusterIP
	svc.Spec.ClusterIP = corev1.ClusterIPNone
	svc.Spec.SessionAffinity = corev1.ServiceAffinityNone
	svc.Spec.Selector = etcd.GetDefaultLabels()
	svc.Spec.PublishNotReadyAddresses = true
	svc.Spec.Ports = getPorts(etcd)
}

func getLabels(etcd *druidv1alpha1.Etcd) map[string]string {
	svcLabels := map[string]string{
		druidv1alpha1.LabelComponentKey: common.PeerServiceComponentName,
		druidv1alpha1.LabelAppNameKey:   etcd.GetPeerServiceName(),
	}
	return utils.MergeMaps(etcd.GetDefaultLabels(), svcLabels)
}

func getObjectKey(etcd *druidv1alpha1.Etcd) client.ObjectKey {
	return client.ObjectKey{Name: etcd.GetPeerServiceName(), Namespace: etcd.Namespace}
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
	peerPort := utils.TypeDeref[int32](etcd.Spec.Etcd.ServerPort, defaultServerPort)
	return []corev1.ServicePort{
		{
			Name:       "peer",
			Protocol:   corev1.ProtocolTCP,
			Port:       peerPort,
			TargetPort: intstr.FromInt(int(peerPort)),
		},
	}
}
