package client

import (
	"context"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/client/clientset/versioned/typed/core/v1alpha1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

// Factory is the main interface for creating all types of clients needed by commands
type Factory interface {
	// CreateEtcdClient creates a client for Etcd custom resources
	CreateEtcdClient() (EtcdClientInterface, error)
	// CreateGenericClient creates a composite client for generic k8s operations
	CreateGenericClient() (GenericClientInterface, error)
}

type ClientFactory struct {
	configFlags *genericclioptions.ConfigFlags
}

func NewClientFactory(configFlags *genericclioptions.ConfigFlags) *ClientFactory {
	return &ClientFactory{configFlags: configFlags}
}

type EtcdClientInterface interface {
	GetEtcd(ctx context.Context, namespace, name string) (*druidv1alpha1.Etcd, error)
	UpdateEtcd(ctx context.Context, etcd *druidv1alpha1.Etcd, etcdModifier func(*druidv1alpha1.Etcd)) error
	ListEtcds(ctx context.Context, namespace string) (*druidv1alpha1.EtcdList, error)
}

type EtcdClient struct {
	client v1alpha1.DruidV1alpha1Interface
}

func NewEtcdClient(client v1alpha1.DruidV1alpha1Interface) EtcdClientInterface {
	return &EtcdClient{client: client}
}

// GenericClientInterface exposes commonly used Kubernetes clients in one place.
type GenericClientInterface interface {
	// Kube returns the typed Kubernetes clientset (core/built-in APIs).
	Kube() kubernetes.Interface
	// Dynamic returns the dynamic client for arbitrary resources (including CRDs).
	Dynamic() dynamic.Interface
	// Discovery returns the discovery client used for API discovery and server resources.
	Discovery() discovery.DiscoveryInterface
	// RESTMapper returns a RESTMapper capable of resolving kinds/short names to resources.
	RESTMapper() meta.RESTMapper
}

type genericClient struct {
	kube       kubernetes.Interface
	dynamic    dynamic.Interface
	discovery  discovery.DiscoveryInterface
	restMapper meta.RESTMapper
}

func (g *genericClient) Kube() kubernetes.Interface              { return g.kube }
func (g *genericClient) Dynamic() dynamic.Interface              { return g.dynamic }
func (g *genericClient) Discovery() discovery.DiscoveryInterface { return g.discovery }
func (g *genericClient) RESTMapper() meta.RESTMapper             { return g.restMapper }
