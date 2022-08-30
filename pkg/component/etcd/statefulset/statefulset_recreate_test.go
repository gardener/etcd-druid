package statefulset

import (
	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/pkg/client/kubernetes"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	clientPort         = 2379
	serverPort         = 2380
	backupPort         = 8080
	etcdImage          = "eu.gcr.io/gardener-project/gardener/etcd:test"
	backupRestoreImage = "eu.gcr.io/gardener-project/gardener/etcdbrctl:test"
	etcdUID            = "d306f0b3-bf17-4ebd-9853-269866abbe12"
)

var _ = Describe("etcd sync", func() {

	var (
		cl     client.Client
		etcd   *druidv1alpha1.Etcd
		values Values
	)

	const (
		name      = "etcd-test-0"
		namespace = "test-ns"
	)

	BeforeEach(func() {
		cl = fakeclient.NewClientBuilder().WithScheme(kubernetes.Scheme).Build()
	})

	Describe("peer url enabled checks", func() {
		var (
			stsDeployer Interface
		)

		JustBeforeEach(func() {
			values = GenerateValues(etcd,
				pointer.Int32(clientPort),
				pointer.Int32Ptr(serverPort),
				pointer.Int32Ptr(backupPort),
				etcdImage,
				backupRestoreImage,
				map[string]string{"checksum/etcd-configmap": "test-hash"},
			)
			stsDeployer = New(cl, logr.Discard(), values)
		})

		When("peerUrlEnabled in etcd status nil and in peerTLSConfig is nil", func() {
			BeforeEach(func() {
				etcd = createEtcd(name, namespace, 1)
			})
			It("test IsPeerUrlTLSChangedToEnabled", func() {
				Expect(stsDeployer.IsPeerUrlTLSChangedToEnabled()).To(BeFalse())
			})
		})

		When("peerUrlEnabled in etcd status is false and peerTLSConfig is set", func() {
			BeforeEach(func() {
				etcd = createEtcd(name, namespace, 1)
				etcd.Status.PeerUrlTLSEnabled = pointer.BoolPtr(false)
				etcd.Spec.Etcd.PeerUrlTLS = createTLSConfig("peer-url-ca-etcd", "peer-url-etcd-server-tls", nil)
			})
			It("test IsPeerUrlTLSChangedToEnabled", func() {
				Expect(stsDeployer.IsPeerUrlTLSChangedToEnabled()).To(BeTrue())
			})
		})

		When("peerUrlEnabled in etcd status is true and peerTLSConfig is set", func() {
			BeforeEach(func() {
				etcd = createEtcd(name, namespace, 1)
				etcd.Status.PeerUrlTLSEnabled = pointer.BoolPtr(true)
				etcd.Spec.Etcd.PeerUrlTLS = createTLSConfig("peer-url-ca-etcd", "peer-url-etcd-server-tls", nil)
			})
			It("test IsPeerUrlTLSChangedToEnabled", func() {
				Expect(stsDeployer.IsPeerUrlTLSChangedToEnabled()).To(BeFalse())
			})
		})
	})

	Describe("compute number of times to recreate StatefulSet", func() {
		var (
			sts *appsv1.StatefulSet
			c   component
		)
		BeforeEach(func() {
			sts = &appsv1.StatefulSet{}
		})

		JustBeforeEach(func() {
			values = GenerateValues(etcd,
				pointer.Int32(clientPort),
				pointer.Int32Ptr(serverPort),
				pointer.Int32Ptr(backupPort),
				etcdImage,
				backupRestoreImage,
				map[string]string{"checksum/etcd-configmap": "test-hash"},
			)
			c = component{
				client: cl,
				logger: logr.Discard(),
				values: values,
			}
		})

		When("sts generation is greater than one, etcd scaled up to multi-node, no immutable field changed", func() {
			BeforeEach(func() {
				sts.Generation = 2
				sts.Spec.ServiceName = name + "-peer"
				sts.Spec.PodManagementPolicy = appsv1.ParallelPodManagement
				etcd = createEtcd(name, namespace, 2)
				etcd.Status.Replicas = 0
			})
			It("test getNumTimesToRecreateSts", func() {
				Expect(c.getNumTimesToRecreateSts(sts)).To(Equal(uint8(0)))
			})
		})

		When("sts generation is greater than one, etcd scaled up to multi-node, immutable field changed", func() {
			BeforeEach(func() {
				sts.Generation = 2
				sts.Spec.ServiceName = name + "-peer-changed"
				sts.Spec.PodManagementPolicy = appsv1.ParallelPodManagement
				etcd = createEtcd(name, namespace, 2)
				etcd.Status.Replicas = 0
			})
			It("test getNumTimesToRecreateSts", func() {
				Expect(c.getNumTimesToRecreateSts(sts)).To(Equal(uint8(1)))
			})
		})

		When("sts generation is equal to one, etcd scaled up to multi-node, immutable field changed", func() {
			BeforeEach(func() {
				sts.Generation = 1
				sts.Spec.ServiceName = name + "-peer-changed"
				sts.Spec.PodManagementPolicy = appsv1.ParallelPodManagement
				etcd = createEtcd(name, namespace, 2)
				etcd.Status.Replicas = 0
			})
			It("test getNumTimesToRecreateSts", func() {
				Expect(c.getNumTimesToRecreateSts(sts)).To(Equal(uint8(0)))
			})
		})

		When("peer url TLS has been changed and set", func() {
			BeforeEach(func() {
				sts.Generation = 1
				etcd = createEtcd(name, namespace, 1)
				etcd.Status.Replicas = 0
				etcd.Status.PeerUrlTLSEnabled = pointer.BoolPtr(false)
				etcd.Spec.Etcd.PeerUrlTLS = createTLSConfig("peer-url-ca-etcd", "peer-url-etcd-server-tls", nil)
			})
			It("test getNumTimesToRecreateSts", func() {
				Expect(c.getNumTimesToRecreateSts(sts)).To(Equal(uint8(2)))
			})
		})
	})

})

func createEtcd(name, namespace string, replicas int32) *druidv1alpha1.Etcd {
	return &druidv1alpha1.Etcd{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       etcdUID,
		},
		Spec: druidv1alpha1.EtcdSpec{
			Labels: map[string]string{"gardener.cloud/role": "controlplane", "role": "main"},
			Etcd: druidv1alpha1.EtcdConfig{
				ServerPort:   pointer.Int32Ptr(serverPort),
				ClientPort:   pointer.Int32Ptr(clientPort),
				Image:        pointer.String(etcdImage),
				ClientUrlTLS: createTLSConfig("client-url-ca-etcd", "client-url-etcd-client-tls", pointer.String("client-url-etcd-server-tls")),
			},
			Backup: druidv1alpha1.BackupSpec{
				Port:  pointer.Int32Ptr(backupPort),
				Image: pointer.String(backupRestoreImage),
			},
			Replicas: replicas,
		},
		Status: druidv1alpha1.EtcdStatus{},
	}
}

func createTLSConfig(tlsCASecretRefName, serverTLSSecretRefName string, clientTLSSecretRefName *string) *druidv1alpha1.TLSConfig {
	tlsConfig := druidv1alpha1.TLSConfig{
		TLSCASecretRef: druidv1alpha1.SecretReference{
			SecretReference: corev1.SecretReference{
				Name: tlsCASecretRefName,
			},
			DataKey: pointer.String("ca.crt"),
		},
		ServerTLSSecretRef: corev1.SecretReference{
			Name: serverTLSSecretRefName,
		},
	}
	if clientTLSSecretRefName != nil {
		tlsConfig.ClientTLSSecretRef = corev1.SecretReference{Name: *clientTLSSecretRefName}
	}
	return &tlsConfig
}
