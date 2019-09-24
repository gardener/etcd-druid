// Copyright (c) 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package kubernetes

import (
	"context"
	"errors"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/gardener/etcd-druid/pkg/utils"
	corev1 "k8s.io/api/core/v1"

	kubernetesclientset "k8s.io/client-go/kubernetes"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"k8s.io/apimachinery/pkg/api/meta"
)

// KubeConfig is the key to the kubeconfig
const KubeConfig = "kubeconfig"

// NewRuntimeClientFromSecret creates a new controller runtime Client struct for a given secret.
func NewRuntimeClientFromSecret(secret *corev1.Secret, opts client.Options) (client.Client, error) {
	if kubeconfig, ok := secret.Data[KubeConfig]; ok {
		return NewRuntimeClientFromBytes(kubeconfig, opts)
	}
	return nil, errors.New("no valid kubeconfig found")

}

// NewRuntimeClientFromBytes creates a new controller runtime Client struct for a given kubeconfig byte slice.
func NewRuntimeClientFromBytes(kubeconfig []byte, opts client.Options) (client.Client, error) {
	config, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		return nil, err
	}
	return NewRuntimeClientForConfig(config, opts)
}

// NewRuntimeClientForConfig returns a new controller runtime client from a config.
func NewRuntimeClientForConfig(config *rest.Config, opts client.Options) (client.Client, error) {
	return client.New(config, opts)
}

// NewClientFromFile creates a new Client struct for a given kubeconfig. The kubeconfig will be
// read from the filesystem at location <kubeconfigPath>. If given, <masterURL> overrides the
// master URL in the kubeconfig.
// If no filepath is given, the in-cluster configuration will be taken into account.
func NewClientFromFile(masterURL, kubeconfigPath string, opts client.Options) (Interface, error) {
	config, err := clientcmd.BuildConfigFromFlags(masterURL, kubeconfigPath)
	if err != nil {
		return nil, err
	}

	return NewForConfig(config, opts)
}

// NewClientFromBytes creates a new Client struct for a given kubeconfig byte slice.
func NewClientFromBytes(kubeconfig []byte, opts client.Options) (Interface, error) {
	config, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	if err != nil {
		return nil, err
	}

	return NewForConfig(config, opts)
}

// NewClientFromSecret creates a new Client struct for a given kubeconfig stored as a
// Secret in an existing Kubernetes cluster. This cluster will be accessed by the <k8sClient>. It will
// read the Secret <secretName> in <namespace>. The Secret must contain a field "kubeconfig" which will
// be used.
func NewClientFromSecret(k8sClient Interface, namespace, secretName string, opts client.Options) (Interface, error) {
	secret := &corev1.Secret{}
	if err := k8sClient.Client().Get(context.TODO(), utils.Key(namespace, secretName), secret); err != nil {
		return nil, err
	}
	return NewClientFromSecretObject(secret, opts)
}

// NewClientFromSecretObject creates a new Client struct for a given Kubernetes Secret object. The Secret must
// contain a field "kubeconfig" which will be used.
func NewClientFromSecretObject(secret *corev1.Secret, opts client.Options) (Interface, error) {
	if kubeconfig, ok := secret.Data[KubeConfig]; ok {
		return NewClientFromBytes(kubeconfig, opts)
	}
	return nil, errors.New("the secret does not contain a field with name 'kubeconfig'")
}

// NewForConfig returns a new Kubernetes base client.
func NewForConfig(config *rest.Config, options client.Options) (Interface, error) {
	c, err := client.New(config, options)
	if err != nil {
		return nil, err
	}

	applier, err := NewApplierForConfig(config)
	if err != nil {
		return nil, err
	}

	kubernetes, err := kubernetesclientset.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	clientSet := &Clientset{
		config:     config,
		restMapper: options.Mapper,
		restClient: kubernetes.Discovery().RESTClient(),

		applier: applier,

		client: c,

		kubernetes: kubernetes,
	}

	serverVersion, err := clientSet.kubernetes.Discovery().ServerVersion()
	if err != nil {
		return nil, err
	}

	clientSet.version = serverVersion.GitVersion

	return clientSet, nil
}

// Clientset is a struct containing the configuration for the respective Kubernetes
// cluster, the collection of Kubernetes clients <Clientset> containing all REST clients
// for the built-in Kubernetes API groups.
// The RESTClient itself is a normal HTTP client for the respective Kubernetes cluster,
// allowing requests to arbitrary URLs.
// The version string contains only the major/minor part in the form <major>.<minor>.
type Clientset struct {
	config     *rest.Config
	restMapper meta.RESTMapper
	restClient rest.Interface

	applier ApplierInterface

	client client.Client

	kubernetes kubernetesclientset.Interface
	version    string
}

// Interface is used to wrap the interactions with a Kubernetes cluster
// (which are performed with the help of kubernetes/client-go) in order to allow the implementation
// of several Kubernetes versions.
type Interface interface {
	RESTConfig() *rest.Config
	RESTMapper() meta.RESTMapper
	RESTClient() rest.Interface

	Client() client.Client
	Applier() ApplierInterface

	Kubernetes() kubernetesclientset.Interface
	Version() string
}
