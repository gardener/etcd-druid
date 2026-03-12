// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package fake

import (
	"context"
	"fmt"

	"github.com/gardener/etcd-druid/druidctl/internal/client"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	discoveryfake "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

// resourceMapping defines a k8s resource type for RESTMapper registration.
type resourceMapping struct {
	group, version, kind, plural, singular string
}

// commonResourceMappings lists resource types used in tests.
var commonResourceMappings = []resourceMapping{
	{"", "v1", "Pod", "pods", "pod"},
	{"", "v1", "Service", "services", "service"},
	{"", "v1", "ConfigMap", "configmaps", "configmap"},
	{"", "v1", "Secret", "secrets", "secret"},
	{"", "v1", "PersistentVolumeClaim", "persistentvolumeclaims", "persistentvolumeclaim"},
	{"", "v1", "ServiceAccount", "serviceaccounts", "serviceaccount"},
	{"apps", "v1", "StatefulSet", "statefulsets", "statefulset"},
	{"coordination.k8s.io", "v1", "Lease", "leases", "lease"},
	{"policy", "v1", "PodDisruptionBudget", "poddisruptionbudgets", "poddisruptionbudget"},
	{"rbac.authorization.k8s.io", "v1", "Role", "roles", "role"},
	{"rbac.authorization.k8s.io", "v1", "RoleBinding", "rolebindings", "rolebinding"},
}

// TestFactory provides helpers for constructing fake etcd and Kubernetes clients for unit tests.
type TestFactory struct {
	etcdObjects []runtime.Object
	k8sObjects  []runtime.Object
}

// NewTestFactory creates an empty TestFactory without any test data
func NewTestFactory() *TestFactory {
	return &TestFactory{
		etcdObjects: make([]runtime.Object, 0),
		k8sObjects:  make([]runtime.Object, 0),
	}
}

// NewTestFactoryWithData creates a TestFactory seeded with the provided etcd and k8s objects.
func NewTestFactoryWithData(etcdObjects, k8sObjects []runtime.Object) *TestFactory {
	return &TestFactory{
		etcdObjects: etcdObjects,
		k8sObjects:  k8sObjects,
	}
}

// CreateEtcdClient returns a fake Etcd client populated with the factory's objects.
func (f *TestFactory) CreateEtcdClient() (client.EtcdClientInterface, error) {
	return NewFakeEtcdClient(f.etcdObjects), nil
}

// CreateGenericClient returns a fake composite Kubernetes client populated with the factory's objects.
func (f *TestFactory) CreateGenericClient() (client.GenericClientInterface, error) {
	return NewFakeGenericClient(f.k8sObjects), nil
}

// FakeEtcdClient implements EtcdClientInterface backed by an in-memory map.
type FakeEtcdClient struct {
	etcds map[string]*druidv1alpha1.Etcd
}

// NewFakeEtcdClient constructs a FakeEtcdClient optionally seeded with Etcd objects.
func NewFakeEtcdClient(etcdObjects []runtime.Object) *FakeEtcdClient {
	etcds := make(map[string]*druidv1alpha1.Etcd)

	for _, obj := range etcdObjects {
		if etcd, ok := obj.(*druidv1alpha1.Etcd); ok {
			key := fmt.Sprintf("%s/%s", etcd.Namespace, etcd.Name)
			etcds[key] = etcd.DeepCopy()
		}
	}

	return &FakeEtcdClient{etcds: etcds}
}

// GetEtcd retrieves a single Etcd object by namespace/name.
func (c *FakeEtcdClient) GetEtcd(_ context.Context, namespace, name string) (*druidv1alpha1.Etcd, error) {
	etcd, exists := c.etcds[fmt.Sprintf("%s/%s", namespace, name)]
	if !exists {
		return nil, errors.NewNotFound(schema.GroupResource{Group: "druid.gardener.cloud", Resource: "etcds"}, name)
	}
	return etcd.DeepCopy(), nil
}

// UpdateEtcd applies a modifier function to an existing Etcd object, simulating an update.
func (c *FakeEtcdClient) UpdateEtcd(_ context.Context, etcd *druidv1alpha1.Etcd, etcdModifier func(*druidv1alpha1.Etcd)) error {
	key := fmt.Sprintf("%s/%s", etcd.Namespace, etcd.Name)
	existingEtcd, exists := c.etcds[key]
	if !exists {
		return errors.NewNotFound(schema.GroupResource{Group: "druid.gardener.cloud", Resource: "etcds"}, etcd.Name)
	}
	// Work on a copy to simulate real behavior
	updatedEtcd := existingEtcd.DeepCopy()
	etcdModifier(updatedEtcd)
	c.etcds[key] = updatedEtcd
	return nil
}

// ListEtcds lists Etcd objects optionally filtered by namespace and label selector.
// Note: The fake implementation ignores labelSelector for simplicity in tests.
func (c *FakeEtcdClient) ListEtcds(_ context.Context, namespace string, _ string) (*druidv1alpha1.EtcdList, error) {
	etcdList := &druidv1alpha1.EtcdList{}
	for _, etcd := range c.etcds {
		if namespace == "" || etcd.Namespace == namespace {
			etcdList.Items = append(etcdList.Items, *etcd.DeepCopy())
		}
	}
	return etcdList, nil
}

// FakeGenericClient is a composite fake Kubernetes client bundle used in tests.
type FakeGenericClient struct {
	scheme          *runtime.Scheme
	k8sClient       kubernetes.Interface
	dynClient       dynamic.Interface
	discoveryClient discovery.DiscoveryInterface
	restMapper      meta.RESTMapper
}

// NewFakeGenericClient constructs a FakeGenericClient seeded with the provided Kubernetes objects.
func NewFakeGenericClient(k8sObjects []runtime.Object) *FakeGenericClient {
	scheme := runtime.NewScheme()

	// Add API types to scheme for dynamic client listing support
	if err := corev1.AddToScheme(scheme); err != nil {
		panic(fmt.Sprintf("failed to add corev1 to scheme: %v", err))
	}
	if err := appsv1.AddToScheme(scheme); err != nil {
		panic(fmt.Sprintf("failed to add appsv1 to scheme: %v", err))
	}
	if err := coordinationv1.AddToScheme(scheme); err != nil {
		panic(fmt.Sprintf("failed to add coordinationv1 to scheme: %v", err))
	}
	if err := policyv1.AddToScheme(scheme); err != nil {
		panic(fmt.Sprintf("failed to add policyv1 to scheme: %v", err))
	}
	if err := rbacv1.AddToScheme(scheme); err != nil {
		panic(fmt.Sprintf("failed to add rbacv1 to scheme: %v", err))
	}

	// Create fake clients with pre-seeded data
	k8sClient := kubefake.NewSimpleClientset(k8sObjects...)
	dynClient := dynamicfake.NewSimpleDynamicClient(scheme, k8sObjects...)
	discoveryClient := discoveryfake.FakeDiscovery{Fake: &k8sClient.Fake}

	// Create a simple RESTMapper with common resource types
	restMapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{
		{Group: "", Version: "v1"},
		{Group: "apps", Version: "v1"},
		{Group: "coordination.k8s.io", Version: "v1"},
		{Group: "policy", Version: "v1"},
		{Group: "rbac.authorization.k8s.io", Version: "v1"},
	})

	// Register common resource types
	for _, r := range commonResourceMappings {
		restMapper.AddSpecific(
			schema.GroupVersionKind{Group: r.group, Version: r.version, Kind: r.kind},
			schema.GroupVersionResource{Group: r.group, Version: r.version, Resource: r.plural},
			schema.GroupVersionResource{Group: r.group, Version: r.version, Resource: r.singular},
			meta.RESTScopeNamespace)
	}

	return &FakeGenericClient{
		scheme:          scheme,
		k8sClient:       k8sClient,
		dynClient:       dynClient,
		discoveryClient: &discoveryClient,
		restMapper:      restMapper,
	}
}

// Kube returns the typed Kubernetes clientset.
func (c *FakeGenericClient) Kube() kubernetes.Interface { return c.k8sClient }

// Dynamic returns the dynamic client implementation.
func (c *FakeGenericClient) Dynamic() dynamic.Interface { return c.dynClient }

// Discovery returns the discovery client implementation.
func (c *FakeGenericClient) Discovery() discovery.DiscoveryInterface { return c.discoveryClient }

// RESTMapper returns a RESTMapper with common resource types registered for testing.
func (c *FakeGenericClient) RESTMapper() meta.RESTMapper { return c.restMapper }
