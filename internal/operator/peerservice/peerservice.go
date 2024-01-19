package peerservice

import (
	"fmt"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"
	druiderr "github.com/gardener/etcd-druid/internal/errors"
	"github.com/gardener/etcd-druid/internal/operator/resource"
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
	ErrGetPeerService    druidv1alpha1.ErrorCode = "ERR_GET_PEER_SERVICE"
	ErrDeletePeerService druidv1alpha1.ErrorCode = "ERR_DELETE_PEER_SERVICE"
	ErrSyncPeerService   druidv1alpha1.ErrorCode = "ERR_SYNC_PEER_SERVICE"
)

type _resource struct {
	client client.Client
}

func (r _resource) GetExistingResourceNames(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd) ([]string, error) {
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
	resourceNames = append(resourceNames, svc.Name)
	return resourceNames, nil
}

func (r _resource) Sync(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	svc := emptyPeerService(getObjectKey(etcd))
	result, err := controllerutils.GetAndCreateOrStrategicMergePatch(ctx, r.client, svc, func() error {
		buildResource(etcd, svc)
		return nil
	})
	if err == nil {
		ctx.Logger.Info("synced", "resource", "peer-service", "name", svc.Name, "result", result)
		return nil
	}
	return druiderr.WrapError(err,
		ErrSyncPeerService,
		"Sync",
		fmt.Sprintf("Error during create or update of peer service: %s for etcd: %v", svc.Name, etcd.GetNamespaceName()),
	)
}

func (r _resource) TriggerDelete(ctx resource.OperatorContext, etcd *druidv1alpha1.Etcd) error {
	objectKey := getObjectKey(etcd)
	ctx.Logger.Info("Triggering delete of peer service")
	err := client.IgnoreNotFound(r.client.Delete(ctx, emptyPeerService(objectKey)))
	if err == nil {
		ctx.Logger.Info("deleted", "resource", "peer-service", "name", objectKey.Name)
		return nil
	}
	return druiderr.WrapError(
		err,
		ErrDeletePeerService,
		"TriggerDelete",
		fmt.Sprintf("Failed to delete peer service: %s for etcd: %v", objectKey.Name, etcd.GetNamespaceName()),
	)
}

func New(client client.Client) resource.Operator {
	return &_resource{
		client: client,
	}
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
	return utils.MergeMaps[string, string](etcd.GetDefaultLabels(), svcLabels)
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
