package fake

import (
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// TestDataBuilder helps create test scenarios with predefined data
type TestDataBuilder struct {
	etcds      []runtime.Object
	k8sObjects []runtime.Object
}

// NewTestDataBuilder creates a new test data builder
func NewTestDataBuilder() *TestDataBuilder {
	return &TestDataBuilder{
		etcds:      make([]runtime.Object, 0),
		k8sObjects: make([]runtime.Object, 0),
	}
}

// WithEtcd adds an Etcd resource to the test data
func (b *TestDataBuilder) WithEtcd(name, namespace string) *TestDataBuilder {
	etcd := &druidv1alpha1.Etcd{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name": name,
			},
		},
		Spec: druidv1alpha1.EtcdSpec{
			Replicas: 3,
		},
		Status: druidv1alpha1.EtcdStatus{
			Ready: &[]bool{true}[0], // Convert to *bool
		},
	}
	b.etcds = append(b.etcds, etcd)
	return b
}

// WithManagedPod adds a Pod managed by the Etcd resource
func (b *TestDataBuilder) WithManagedPod(etcdName, namespace, podName string) *TestDataBuilder {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/part-of": etcdName,
				"app.kubernetes.io/name":    podName,
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name: "etcd",
				},
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}
	b.k8sObjects = append(b.k8sObjects, pod)
	return b
}

// WithManagedService adds a Service managed by the Etcd resource
func (b *TestDataBuilder) WithManagedService(etcdName, namespace, serviceName string) *TestDataBuilder {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/part-of": etcdName,
				"app.kubernetes.io/name":    serviceName,
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name: "client",
					Port: 2379,
				},
				{
					Name: "peer",
					Port: 2380,
				},
			},
		},
	}
	b.k8sObjects = append(b.k8sObjects, service)
	return b
}

// Build returns the test data
func (b *TestDataBuilder) Build() ([]runtime.Object, []runtime.Object) {
	return b.etcds, b.k8sObjects
}

// Common test scenarios

// SingleEtcdWithResources creates a realistic test scenario with one Etcd and its managed resources
func SingleEtcdWithResources() *TestDataBuilder {
	return NewTestDataBuilder().
		WithEtcd("test-etcd", "default").
		WithManagedPod("test-etcd", "default", "test-etcd-0").
		WithManagedPod("test-etcd", "default", "test-etcd-1").
		WithManagedPod("test-etcd", "default", "test-etcd-2").
		WithManagedService("test-etcd", "default", "test-etcd-client").
		WithManagedService("test-etcd", "default", "test-etcd-peer")
}

// MultipleEtcdsScenario creates a test scenario with multiple Etcd resources across namespaces
func MultipleEtcdsScenario() *TestDataBuilder {
	return NewTestDataBuilder().
		WithEtcd("etcd-main", "shoot-ns1").
		WithManagedPod("etcd-main", "shoot-ns1", "etcd-main-0").
		WithManagedService("etcd-main", "shoot-ns1", "etcd-main-client").
		WithEtcd("etcd-events", "shoot-ns1").
		WithManagedPod("etcd-events", "shoot-ns1", "etcd-events-0").
		WithManagedService("etcd-events", "shoot-ns1", "etcd-events-client").
		WithEtcd("etcd-main", "shoot-ns2").
		WithManagedPod("etcd-main", "shoot-ns2", "etcd-main-0").
		WithManagedService("etcd-main", "shoot-ns2", "etcd-main-client")
}

// EmptyScenario creates a test scenario with no resources
func EmptyScenario() *TestDataBuilder {
	return NewTestDataBuilder()
}
