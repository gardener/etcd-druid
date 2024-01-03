package sample

import (
	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/utils"
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

func getClientServicePorts(etcd *druidv1alpha1.Etcd) []corev1.ServicePort {
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
