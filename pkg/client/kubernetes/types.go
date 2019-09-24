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
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// DefaultApplierOptions contains options for common k8s objects, e.g. Service, ServiceAccount.
var DefaultApplierOptions = ApplierOptions{
	MergeFuncs: map[schema.GroupKind]MergeFunc{
		corev1.SchemeGroupVersion.WithKind("Service").GroupKind(): func(newObj, oldObj *unstructured.Unstructured) {
			// We do not want to overwrite a Service's `.spec.clusterIP' or '.spec.ports[*].nodePort' values.
			oldPorts := oldObj.Object["spec"].(map[string]interface{})["ports"].([]interface{})
			newPorts := newObj.Object["spec"].(map[string]interface{})["ports"].([]interface{})
			ports := []map[string]interface{}{}

			// Check whether ports of the newObj have also been present previously. If yes, take the nodePort
			// of the existing object.
			for _, newPort := range newPorts {
				np := newPort.(map[string]interface{})

				for _, oldPort := range oldPorts {
					op := oldPort.(map[string]interface{})
					// np["port"] is of type float64 (due to Helm Tiller rendering) while op["port"] is of type int64.
					// Equality can only be checked via their string representations.
					if fmt.Sprintf("%v", np["port"]) == fmt.Sprintf("%v", op["port"]) {
						if nodePort, ok := op["nodePort"]; ok {
							np["nodePort"] = nodePort
						}
					}
				}
				ports = append(ports, np)
			}

			newObj.Object["spec"].(map[string]interface{})["clusterIP"] = oldObj.Object["spec"].(map[string]interface{})["clusterIP"]
			newObj.Object["spec"].(map[string]interface{})["ports"] = ports
		},
		corev1.SchemeGroupVersion.WithKind("ServiceAccount").GroupKind(): func(newObj, oldObj *unstructured.Unstructured) {
			// We do not want to overwrite a ServiceAccount's `.secrets[]` list or `.imagePullSecrets[]`.
			newObj.Object["secrets"] = oldObj.Object["secrets"]
			newObj.Object["imagePullSecrets"] = oldObj.Object["imagePullSecrets"]
		},
	},
}

// UnstructuredReader an interface that all manifest readers should implement
type UnstructuredReader interface {
	Read() (*unstructured.Unstructured, error)
}

// Applier is a default implementation of the ApplyInterface. It applies objects with
// by first checking whether they exist and then either creating / updating them (update happens
// with a predefined merge logic).
type Applier struct {
	client    client.Client
	discovery discovery.CachedDiscoveryInterface
}

// MergeFunc determines how oldOj is merged into new oldObj.
type MergeFunc func(newObj, oldObj *unstructured.Unstructured)

// ApplierOptions contains options used by the Applier.
type ApplierOptions struct {
	MergeFuncs map[schema.GroupKind]MergeFunc
}

// ApplierInterface is an interface which describes declarative operations to apply multiple
// Kubernetes objects.
type ApplierInterface interface {
	ApplyManifest(ctx context.Context, unstructured UnstructuredReader, options ApplierOptions) error
	DeleteManifest(ctx context.Context, unstructured UnstructuredReader) error
}

// NewNamespaceSettingReader initializes a reader for yaml manifests with support for setting the namespace
func NewNamespaceSettingReader(mReader UnstructuredReader, namespace string) UnstructuredReader {
	return &namespaceSettingReader{
		reader:    mReader,
		namespace: namespace,
	}
}

// namespaceSettingReader is an unstructured reader that contains a JSONDecoder and a manifest reader (or other reader types)
type namespaceSettingReader struct {
	reader    UnstructuredReader
	namespace string
}

// Read decodes yaml data into an unstructured object
func (n *namespaceSettingReader) Read() (*unstructured.Unstructured, error) {
	readObj, err := n.reader.Read()
	if err != nil {
		return nil, err
	}

	readObj.SetNamespace(n.namespace)

	return readObj, nil
}
