package etcd

import (
	"bytes"

	"github.com/gardener/gardener/pkg/chartrenderer"
	corev1 "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/rest"
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

func (c *chart) decodeServiceAccount(etcdName, etcdNs, typeChartPath string, values map[string]interface{}) (*corev1.ServiceAccount, error) {
	return decodeObjectFromChart[corev1.ServiceAccount](c, etcdName, etcdNs, typeChartPath, values)
}

func (c *chart) decodeRole(etcdName, etcdNs, typeChartPath string, values map[string]interface{}) (*rbac.Role, error) {
	return decodeObjectFromChart[rbac.Role](c, etcdName, etcdNs, typeChartPath, values)
}

func (c *chart) decodeRoleBinding(etcdName, etcdNs, typeChartPath string, values map[string]interface{}) (*rbac.RoleBinding, error) {
	return decodeObjectFromChart[rbac.RoleBinding](c, etcdName, etcdNs, typeChartPath, values)
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
	}
	return obj, nil
}
