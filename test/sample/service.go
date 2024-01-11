package sample

import (
	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	testutils "github.com/gardener/etcd-druid/test/utils"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	defaultBackupPort = 8080
	defaultClientPort = 2379
	defaultServerPort = 2380
)

// NewClientService creates a new sample client service initializing it from the passed in etcd object.
func NewClientService(etcd *druidv1alpha1.Etcd) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            etcd.GetClientServiceName(),
			Namespace:       etcd.Namespace,
			OwnerReferences: []metav1.OwnerReference{etcd.GetAsOwnerReference()},
		},
		Spec: corev1.ServiceSpec{
			Ports:           getClientServicePorts(etcd),
			Selector:        etcd.GetDefaultLabels(),
			Type:            corev1.ServiceTypeClusterIP,
			SessionAffinity: corev1.ServiceAffinityNone,
		},
	}
}

// NewPeerService creates a new sample peer service initializing it from the passed in etcd object.
func NewPeerService(etcd *druidv1alpha1.Etcd) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      etcd.GetPeerServiceName(),
			Namespace: etcd.Namespace,
			Labels:    etcd.GetDefaultLabels(),
			OwnerReferences: []metav1.OwnerReference{
				etcd.GetAsOwnerReference(),
			},
		},
		Spec: corev1.ServiceSpec{
			Type:                     corev1.ServiceTypeClusterIP,
			ClusterIP:                corev1.ClusterIPNone,
			SessionAffinity:          corev1.ServiceAffinityNone,
			Selector:                 etcd.GetDefaultLabels(),
			PublishNotReadyAddresses: true,
			Ports: []corev1.ServicePort{
				{
					Name:       "peer",
					Protocol:   corev1.ProtocolTCP,
					Port:       *etcd.Spec.Etcd.ServerPort,
					TargetPort: intstr.FromInt(int(*etcd.Spec.Etcd.ServerPort)),
				},
			},
		},
	}
}

func getClientServicePorts(etcd *druidv1alpha1.Etcd) []corev1.ServicePort {
	svcBackupPort := testutils.TypeDeref[int32](etcd.Spec.Backup.Port, defaultBackupPort)
	svcClientPort := testutils.TypeDeref[int32](etcd.Spec.Etcd.ClientPort, defaultClientPort)
	svcPeerPort := testutils.TypeDeref[int32](etcd.Spec.Etcd.ServerPort, defaultServerPort)
	return []corev1.ServicePort{
		{
			Name:       "client",
			Protocol:   corev1.ProtocolTCP,
			Port:       svcClientPort,
			TargetPort: intstr.FromInt(int(svcClientPort)),
		},
		{
			Name:       "server",
			Protocol:   corev1.ProtocolTCP,
			Port:       svcPeerPort,
			TargetPort: intstr.FromInt(int(svcPeerPort)),
		},
		{
			Name:       "backuprestore",
			Protocol:   corev1.ProtocolTCP,
			Port:       svcBackupPort,
			TargetPort: intstr.FromInt(int(svcBackupPort)),
		},
	}
}
