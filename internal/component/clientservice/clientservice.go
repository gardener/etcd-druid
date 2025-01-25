// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package clientservice

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
	// ErrGetClientService indicates an error in getting the client service resource.
	ErrGetClientService druidv1alpha1.ErrorCode = "ERR_GET_CLIENT_SERVICE"
	// ErrSyncClientService indicates an error in syncing the client service resource.
	ErrSyncClientService druidv1alpha1.ErrorCode = "ERR_SYNC_CLIENT_SERVICE"
	// ErrDeleteClientService indicates an error in deleting the client service resource.
	ErrDeleteClientService druidv1alpha1.ErrorCode = "ERR_DELETE_CLIENT_SERVICE"
)

type _resource struct {
	client client.Client
}

// New returns a new client service component operator.
func New(client client.Client) component.Operator {
	return &_resource{
		client: client,
	}
}

// GetExistingResourceNames returns the name of the existing client service for the given Etcd.
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
			ErrGetClientService,
			component.OperationGetExistingResourceNames,
			fmt.Sprintf("Error getting client service: %v for etcd: %v", svcObjectKey, druidv1alpha1.GetNamespaceName(etcdObjMeta)))
	}
	if metav1.IsControlledBy(objMeta, &etcdObjMeta) {
		resourceNames = append(resourceNames, objMeta.Name)
	}
	return resourceNames, nil
}

// PreSync is a no-op for the client service component.
func (r _resource) PreSync(_ component.OperatorContext, _ *druidv1alpha1.Etcd) error { return nil }

// Sync creates or updates the client service for the given Etcd.
func (r _resource) Sync(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	objectKey := getObjectKey(etcd.ObjectMeta)
	svc := emptyClientService(objectKey)
	result, err := controllerutils.GetAndCreateOrStrategicMergePatch(ctx, r.client, svc, func() error {
		buildResource(etcd, svc)
		return nil
	})
	if err != nil {
		return druiderr.WrapError(err,
			ErrSyncClientService,
			component.OperationSync,
			fmt.Sprintf("Error during create or update of client service: %v for etcd: %v", objectKey, druidv1alpha1.GetNamespaceName(etcd.ObjectMeta)),
		)
	}
	ctx.Logger.Info("synced", "component", "client-service", "objectKey", objectKey, "result", result)
	return nil
}

// TriggerDelete triggers the deletion of the client service for the given Etcd.
func (r _resource) TriggerDelete(ctx component.OperatorContext, etcdObjMeta metav1.ObjectMeta) error {
	objectKey := getObjectKey(etcdObjMeta)
	ctx.Logger.Info("Triggering deletion of client service", "objectKey", objectKey)
	if err := r.client.Delete(ctx, emptyClientService(objectKey)); err != nil {
		if errors.IsNotFound(err) {
			ctx.Logger.Info("No Client Service found, Deletion is a No-Op", "objectKey", objectKey)
			return nil
		}
		return druiderr.WrapError(
			err,
			ErrDeleteClientService,
			component.OperationTriggerDelete,
			"Failed to delete client service",
		)
	}
	ctx.Logger.Info("deleted", "component", "client-service", "objectKey", objectKey)
	return nil
}

func buildResource(etcd *druidv1alpha1.Etcd, svc *corev1.Service) {
	svc.Labels = getLabels(etcd)
	svc.Annotations = getAnnotations(etcd)
	svc.OwnerReferences = []metav1.OwnerReference{druidv1alpha1.GetAsOwnerReference(etcd.ObjectMeta)}
	svc.Spec.Type = corev1.ServiceTypeClusterIP
	svc.Spec.SessionAffinity = corev1.ServiceAffinityNone
	// Client service should only target StatefulSet pods. Default labels are going to be present on anything that is managed by etcd-druid and started for an etcd cluster.
	// Therefore, only using default labels as label selector can cause issues as we have already seen in https://github.com/gardener/etcd-druid/issues/914
	svc.Spec.Selector = utils.MergeMaps(druidv1alpha1.GetDefaultLabels(etcd.ObjectMeta), map[string]string{druidv1alpha1.LabelComponentKey: common.ComponentNameStatefulSet})
	svc.Spec.Ports = getPorts(etcd)
	svc.Spec.TrafficDistribution = getTrafficDistribution(etcd)
}

func getObjectKey(obj metav1.ObjectMeta) client.ObjectKey {
	return client.ObjectKey{
		Name:      druidv1alpha1.GetClientServiceName(obj),
		Namespace: obj.Namespace,
	}
}

func getLabels(etcd *druidv1alpha1.Etcd) map[string]string {
	clientSvcLabels := map[string]string{
		druidv1alpha1.LabelAppNameKey:   druidv1alpha1.GetClientServiceName(etcd.ObjectMeta),
		druidv1alpha1.LabelComponentKey: common.ComponentNameClientService,
	}
	// Add any client service labels as defined in the etcd resource
	specClientSvcLabels := map[string]string{}
	if etcd.Spec.Etcd.ClientService != nil && etcd.Spec.Etcd.ClientService.Labels != nil {
		specClientSvcLabels = etcd.Spec.Etcd.ClientService.Labels
	}
	return utils.MergeMaps(druidv1alpha1.GetDefaultLabels(etcd.ObjectMeta), clientSvcLabels, specClientSvcLabels)
}

func getAnnotations(etcd *druidv1alpha1.Etcd) map[string]string {
	if etcd.Spec.Etcd.ClientService != nil {
		return etcd.Spec.Etcd.ClientService.Annotations
	}
	return nil
}

func getTrafficDistribution(etcd *druidv1alpha1.Etcd) *string {
	if etcd.Spec.Etcd.ClientService != nil {
		return etcd.Spec.Etcd.ClientService.TrafficDistribution
	}
	return nil
}

func emptyClientService(objectKey client.ObjectKey) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      objectKey.Name,
			Namespace: objectKey.Namespace,
		},
	}
}

func getPorts(etcd *druidv1alpha1.Etcd) []corev1.ServicePort {
	backupPort := ptr.Deref(etcd.Spec.Backup.Port, common.DefaultPortEtcdBackupRestore)
	clientPort := ptr.Deref(etcd.Spec.Etcd.ClientPort, common.DefaultPortEtcdClient)
	peerPort := ptr.Deref(etcd.Spec.Etcd.ServerPort, common.DefaultPortEtcdPeer)

	return []corev1.ServicePort{
		{
			Name:       "client",
			Protocol:   corev1.ProtocolTCP,
			Port:       clientPort,
			TargetPort: intstr.FromInt(int(clientPort)),
		},
		// TODO: Remove the "server" port in a future release
		{
			Name:       "server",
			Protocol:   corev1.ProtocolTCP,
			Port:       peerPort,
			TargetPort: intstr.FromInt(int(peerPort)),
		},
		{
			Name:       "backuprestore",
			Protocol:   corev1.ProtocolTCP,
			Port:       backupPort,
			TargetPort: intstr.FromInt(int(backupPort)),
		},
	}
}
