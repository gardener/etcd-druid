// SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package fake

import (
	cmdutils "github.com/gardener/etcd-druid/druidctl/cmd/utils"
	"github.com/gardener/etcd-druid/druidctl/internal/log"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/genericiooptions"
)

// TestHelper provides utilities for creating test environments
type TestHelper struct {
	etcdObjects []runtime.Object
	k8sObjects  []runtime.Object
	streams     genericiooptions.IOStreams
}

// NewTestHelper creates a new test helper
func NewTestHelper() *TestHelper {
	streams, _, _, _ := genericiooptions.NewTestIOStreams()
	return &TestHelper{
		etcdObjects: make([]runtime.Object, 0),
		k8sObjects:  make([]runtime.Object, 0),
		streams:     streams,
	}
}

// WithEtcdObjects adds etcd objects to the test environment
func (h *TestHelper) WithEtcdObjects(objects []runtime.Object) *TestHelper {
	h.etcdObjects = append(h.etcdObjects, objects...)
	return h
}

// EtcdObjectCount returns the number of Etcd objects currently tracked by the helper.
func (h *TestHelper) EtcdObjectCount() int { return len(h.etcdObjects) }

// WithK8sObjects adds k8s objects to the test environment
func (h *TestHelper) WithK8sObjects(objects []runtime.Object) *TestHelper {
	h.k8sObjects = append(h.k8sObjects, objects...)
	return h
}

// WithTestScenario adds objects from a test scenario builder
func (h *TestHelper) WithTestScenario(builder *TestDataBuilder) *TestHelper {
	etcdObjs, k8sObjs := builder.Build()
	h.etcdObjects = append(h.etcdObjects, etcdObjs...)
	h.k8sObjects = append(h.k8sObjects, k8sObjs...)
	return h
}

// CreateTestOptions creates Options configured for testing
func (h *TestHelper) CreateTestOptions() *cmdutils.GlobalOptions {
	testFactory := NewTestFactoryWithData(h.etcdObjects, h.k8sObjects)
	namespace := "default"

	// Create fake config flags for testing
	configFlags := &genericclioptions.ConfigFlags{
		Namespace: &namespace,
	}

	return &cmdutils.GlobalOptions{
		LoggerKind:  log.LoggerKindCharm,
		ConfigFlags: configFlags,
		IOStreams:   h.streams,
		Clients:     cmdutils.NewClientBundle(testFactory),
	}
}
