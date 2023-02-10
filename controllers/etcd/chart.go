// Copyright 2023 SAP SE or an SAP affiliate company
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package etcd

import (
	"bytes"
	"fmt"
	"path/filepath"

	"github.com/gardener/gardener/pkg/chartrenderer"
	corev1 "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/rest"
)

var (
	serviceAccountChartPath = filepath.Join("etcd", "templates", "etcd-serviceaccount.yaml")
	roleChartPath           = filepath.Join("etcd", "templates", "etcd-role.yaml")
	roleBindingChartPath    = filepath.Join("etcd", "templates", "etcd-rolebinding.yaml")
)

type chart struct {
	BasePath string
	renderer chartrenderer.Interface
}

func newChart(basePath string, restConfig *rest.Config) (*chart, error) {
	renderer, err := chartrenderer.NewForConfig(restConfig)
	if err != nil {
		return nil, err
	}
	return &chart{
		BasePath: basePath,
		renderer: renderer,
	}, nil
}

func (c *chart) decodeServiceAccount(etcdName, etcdNs string, values map[string]interface{}) (*corev1.ServiceAccount, error) {
	return decodeObjectFromChart[corev1.ServiceAccount](c, etcdName, etcdNs, serviceAccountChartPath, values)
}

func (c *chart) decodeRole(etcdName, etcdNs string, values map[string]interface{}) (*rbac.Role, error) {
	return decodeObjectFromChart[rbac.Role](c, etcdName, etcdNs, roleChartPath, values)
}

func (c *chart) decodeRoleBinding(etcdName, etcdNs string, values map[string]interface{}) (*rbac.RoleBinding, error) {
	return decodeObjectFromChart[rbac.RoleBinding](c, etcdName, etcdNs, roleBindingChartPath, values)
}

func decodeObjectFromChart[T any](c *chart, etcdName, etcdNs, typeChartPath string, values map[string]interface{}) (*T, error) {
	// TODO(AleksandarSavchev): .Render is deprecated. Refactor or adapt code to use RenderEmbeddedFS https://github.com/gardener/gardener/pull/6165
	renderedChart, err := c.renderer.Render(c.BasePath, etcdName, etcdNs, values) //nolint:staticcheck
	if err != nil {
		return nil, err
	}
	obj := new(T)
	if content, ok := renderedChart.Files()[typeChartPath]; ok {
		decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader([]byte(content)), 1024)
		if err := decoder.Decode(&obj); err != nil {
			return nil, err
		}
	} else {
		return nil, fmt.Errorf("requested object does not exist in the given chart template")
	}
	return obj, nil
}
