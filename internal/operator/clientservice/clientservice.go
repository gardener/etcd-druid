// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package clientservice

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

// default values
const (
	defaultBackupPort = 8080
	defaultClientPort = 2379
	defaultServerPort = 2380
)

const (
	ErrGetClientService    druidv1alpha1.ErrorCode = "ERR_GET_CLIENT_SERVICE"
	ErrDeleteClientService druidv1alpha1.ErrorCode = "ERR_DELETE_CLIENT_SERVICE"
	ErrSyncClientService   druidv1alpha1.ErrorCode = "ERR_SYNC_CLIENT_SERVICE"
)

type _resource struct {
	client client.Client
}

func (r _resource) GetExistingResourceNames(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) ([]string, error) {
	resourceNames := make([]string, 0, 1)
	svc := &corev1.Service{}
	svcObjectKey := getObjectKey(etcd)
	if err := r.client.Get(ctx, svcObjectKey, svc); err != nil {
		if errors.IsNotFound(err) {
			return resourceNames, nil
		}
		return resourceNames, druiderr.WrapError(err,
			ErrGetClientService,
			"GetExistingResourceNames",
			fmt.Sprintf("Error getting client service: %v for etcd: %v", svcObjectKey, etcd.GetNamespaceName()))
	}
	if metav1.IsControlledBy(svc, etcd) {
		resourceNames = append(resourceNames, svc.Name)
	}
	return resourceNames, nil
}

func (r _resource) Sync(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	objectKey := getObjectKey(etcd)
	svc := emptyClientService(objectKey)
	result, err := controllerutils.GetAndCreateOrStrategicMergePatch(ctx, r.client, svc, func() error {
		buildResource(etcd, svc)
		return nil
	})
	if err != nil {
		return druiderr.WrapError(err,
			ErrSyncClientService,
			"Sync",
			fmt.Sprintf("Error during create or update of client service: %v for etcd: %v", objectKey, etcd.GetNamespaceName()),
		)
	}
	ctx.Logger.Info("synced", "component", "client-service", "objectKey", objectKey, "result", result)
	return nil
}

func (r _resource) TriggerDelete(ctx component.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	objectKey := getObjectKey(etcd)
	ctx.Logger.Info("Triggering delete of client service", "objectKey", objectKey)
	if err := r.client.Delete(ctx, emptyClientService(objectKey)); err != nil {
		if errors.IsNotFound(err) {
			ctx.Logger.Info("No Client Service found, Deletion is a No-Op", "objectKey", objectKey)
			return nil
		}
		return druiderr.WrapError(
			err,
			ErrDeleteClientService,
			"TriggerDelete",
			"Failed to delete client service",
		)
	}
	ctx.Logger.Info("deleted", "component", "client-service", "objectKey", objectKey)
	return nil
}

func New(client client.Client) component.Operator {
	return &_resource{
		client: client,
	}
}

func buildResource(etcd *druidv1alpha1.Etcd, svc *corev1.Service) {
	svc.Labels = getLabels(etcd)
	svc.Annotations = getAnnotations(etcd)
	svc.OwnerReferences = []metav1.OwnerReference{etcd.GetAsOwnerReference()}
	svc.Spec.Type = corev1.ServiceTypeClusterIP
	svc.Spec.SessionAffinity = corev1.ServiceAffinityNone
	svc.Spec.Selector = etcd.GetDefaultLabels()
	svc.Spec.Ports = getPorts(etcd)
}

func getObjectKey(etcd *druidv1alpha1.Etcd) client.ObjectKey {
	return client.ObjectKey{
		Name:      etcd.GetClientServiceName(),
		Namespace: etcd.Namespace,
	}
}

func getLabels(etcd *druidv1alpha1.Etcd) map[string]string {
	clientSvcLabels := map[string]string{
		druidv1alpha1.LabelAppNameKey:   etcd.GetClientServiceName(),
		druidv1alpha1.LabelComponentKey: common.ClientServiceComponentName,
	}
	// Add any client service labels as defined in the etcd resource
	specClientSvcLabels := map[string]string{}
	if etcd.Spec.Etcd.ClientService != nil && etcd.Spec.Etcd.ClientService.Labels != nil {
		specClientSvcLabels = etcd.Spec.Etcd.ClientService.Labels
	}
	return utils.MergeMaps[string, string](etcd.GetDefaultLabels(), clientSvcLabels, specClientSvcLabels)
}

func getAnnotations(etcd *druidv1alpha1.Etcd) map[string]string {
	if etcd.Spec.Etcd.ClientService != nil {
		return etcd.Spec.Etcd.ClientService.Annotations
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
	backupPort := utils.TypeDeref[int32](etcd.Spec.Backup.Port, defaultBackupPort)
	clientPort := utils.TypeDeref[int32](etcd.Spec.Etcd.ClientPort, defaultClientPort)
	peerPort := utils.TypeDeref[int32](etcd.Spec.Etcd.ServerPort, defaultServerPort)

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
