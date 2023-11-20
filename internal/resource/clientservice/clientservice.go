package clientservice

import (
	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	druiderr "github.com/gardener/etcd-druid/internal/errors"
	"github.com/gardener/etcd-druid/internal/registry/resource"
	"github.com/gardener/etcd-druid/internal/utils"
	"github.com/gardener/gardener/pkg/controllerutils"
	"github.com/go-logr/logr"
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
	ErrDeletingClientService druidv1alpha1.ErrorCode = "ERR_DELETING_CLIENT_SERVICE"
	ErrSyncingClientService  druidv1alpha1.ErrorCode = "ERR_SYNC_CLIENT_SERVICE"
)

type _resource struct {
	client client.Client
	logger logr.Logger
}

func (r _resource) GetExistingResourceNames(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd) ([]string, error) {
	resourceNames := make([]string, 0, 1)
	svc := &corev1.Service{}
	if err := r.client.Get(ctx, getObjectKey(etcd), svc); err != nil {
		if errors.IsNotFound(err) {
			return resourceNames, nil
		}
		return resourceNames, err
	}
	resourceNames = append(resourceNames, svc.Name)
	return resourceNames, nil
}

func (r _resource) Sync(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	svc := emptyClientService(getObjectKey(etcd))
	_, err := controllerutils.GetAndCreateOrStrategicMergePatch(ctx, r.client, svc, func() error {
		svc.Labels = getLabels(etcd)
		svc.Annotations = getAnnotations(etcd)
		svc.OwnerReferences = []metav1.OwnerReference{etcd.GetAsOwnerReference()}
		svc.Spec.Type = corev1.ServiceTypeClusterIP
		svc.Spec.SessionAffinity = corev1.ServiceAffinityNone
		svc.Spec.Selector = etcd.GetDefaultLabels()
		svc.Spec.Ports = getPorts(etcd)

		return nil
	})
	return druiderr.WrapError(err,
		ErrSyncingClientService,
		"Sync",
		"Error during create or update of client service",
	)
}

func (r _resource) TriggerDelete(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	objectKey := getObjectKey(etcd)
	r.logger.Info("Triggering delete of client service", "objectKey", objectKey)
	err := client.IgnoreNotFound(r.client.Delete(ctx, emptyClientService(objectKey)))
	return druiderr.WrapError(
		err,
		ErrDeletingClientService,
		"TriggerDelete",
		"Failed to delete client service",
	)
}

func New(client client.Client, logger logr.Logger) resource.Operator {
	return &_resource{
		client: client,
		logger: logger,
	}
}

func getObjectKey(etcd *druidv1alpha1.Etcd) client.ObjectKey {
	return client.ObjectKey{
		Name:      etcd.GetClientServiceName(),
		Namespace: etcd.Namespace,
	}
}

func getLabels(etcd *druidv1alpha1.Etcd) map[string]string {
	var labelMaps []map[string]string
	labelMaps = append(labelMaps, etcd.GetDefaultLabels())
	if etcd.Spec.Etcd.ClientService != nil {
		labelMaps = append(labelMaps, etcd.Spec.Etcd.ClientService.Labels)
	}
	return utils.MergeMaps[string, string](labelMaps...)
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
