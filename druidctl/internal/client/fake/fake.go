// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package fake

import (
	"context"
	"fmt"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/druidctl/internal/client"
	corev1 "k8s.io/api/core/v1"
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

type TestFactory struct {
	etcdObjects []runtime.Object
	k8sObjects  []runtime.Object
}

// NewTestFactory creates a TestFactory without test data
func NewTestFactory() *TestFactory {
	return &TestFactory{
		etcdObjects: make([]runtime.Object, 0),
		k8sObjects:  make([]runtime.Object, 0),
	}
}

// NewTestFactoryWithData creates a TestFactory with pre-seeded test data
func NewTestFactoryWithData(etcdObjects, k8sObjects []runtime.Object) *TestFactory {
	return &TestFactory{
		etcdObjects: etcdObjects,
		k8sObjects:  k8sObjects,
	}
}

func (f *TestFactory) CreateEtcdClient() (client.EtcdClientInterface, error) {
	return NewFakeEtcdClient(f.etcdObjects), nil
}

func (f *TestFactory) CreateGenericClient() (client.GenericClientInterface, error) {
	return NewFakeGenericClient(f.k8sObjects), nil
}

type FakeEtcdClient struct {
	etcds map[string]*druidv1alpha1.Etcd
}

// NewFakeEtcdClient creates a FakeEtcdClient with pre-seeded data
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

func (c *FakeEtcdClient) GetEtcd(ctx context.Context, namespace, name string) (*druidv1alpha1.Etcd, error) {
	etcd, exists := c.etcds[fmt.Sprintf("%s/%s", namespace, name)]
	if !exists {
		return nil, errors.NewNotFound(schema.GroupResource{Group: "druid.gardener.cloud", Resource: "etcds"}, name)
	}
	return etcd.DeepCopy(), nil
}

func (c *FakeEtcdClient) UpdateEtcd(ctx context.Context, etcd *druidv1alpha1.Etcd, etcdModifier func(*druidv1alpha1.Etcd)) error {
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

func (c *FakeEtcdClient) ListEtcds(ctx context.Context, namespace string) (*druidv1alpha1.EtcdList, error) {
	etcdList := &druidv1alpha1.EtcdList{}
	for _, etcd := range c.etcds {
		if namespace == "" || etcd.Namespace == namespace {
			etcdList.Items = append(etcdList.Items, *etcd.DeepCopy())
		}
	}
	return etcdList, nil
}

type FakeGenericClient struct {
	scheme          *runtime.Scheme
	k8sClient       kubernetes.Interface
	dynClient       dynamic.Interface
	discoveryClient discovery.DiscoveryInterface
}

// NewFakeGenericClient creates a FakeGenericClient with pre-seeded data
func NewFakeGenericClient(k8sObjects []runtime.Object) *FakeGenericClient {
	scheme := runtime.NewScheme()

	// Add core types to scheme
	corev1.AddToScheme(scheme)

	// Create fake clients with pre-seeded data
	k8sClient := kubefake.NewSimpleClientset(k8sObjects...)
	dynClient := dynamicfake.NewSimpleDynamicClient(scheme, k8sObjects...)
	discoveryClient := discoveryfake.FakeDiscovery{Fake: &k8sClient.Fake}

	return &FakeGenericClient{
		scheme:          scheme,
		k8sClient:       k8sClient,
		dynClient:       dynClient,
		discoveryClient: &discoveryClient,
	}
}

func (c *FakeGenericClient) Kube() kubernetes.Interface {
	return c.k8sClient
}

func (c *FakeGenericClient) Dynamic() dynamic.Interface {
	return c.dynClient
}

func (c *FakeGenericClient) Discovery() discovery.DiscoveryInterface {
	return c.discoveryClient
}

func (c *FakeGenericClient) RESTMapper() meta.RESTMapper {
	return nil
}
