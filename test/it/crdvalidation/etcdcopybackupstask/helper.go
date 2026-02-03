// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package etcdcopybackupstask

import (
	"context"
	"fmt"
	"testing"

	"github.com/gardener/etcd-druid/test/it/setup"
	"github.com/gardener/etcd-druid/test/utils"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/gomega"
)

const testNamespacePrefix = "etcdcopybackupstask-validation-test"

var (
	itTestEnv setup.DruidTestEnvironment
)

func validatePatchedObjectCreation[T client.Object](g *WithT, patchedObject *unstructured.Unstructured, expectErr bool) {
	cl := itTestEnv.GetClient()
	ctx := context.Background()

	if expectErr {
		g.Expect(cl.Create(ctx, patchedObject)).NotTo(Succeed())
		return
	}
	g.Expect(cl.Create(ctx, patchedObject)).To(Succeed())

	// Ensure the object is created successfully.
	// This needs to be done to ensure that 1) the object is created and 2) the created object is valid.
	// If the duration validation is not correct, the object will be created but will fail to decode correctly.
	// Reference: https://github.com/kubernetes/apimachinery/issues/131
	g.Eventually(func() error {
		fetchedObject := patchedObject.DeepCopyObject().(client.Object)
		if err := cl.Get(ctx, client.ObjectKeyFromObject(patchedObject), fetchedObject); err != nil {
			return fmt.Errorf("failed to fetch object: %w", err)
		}
		unstructuredObject, ok := fetchedObject.(*unstructured.Unstructured)
		if !ok {
			return fmt.Errorf("failed to convert fetched object to unstructured object: %T", fetchedObject)
		}
		typedObject := new(T)
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredObject.Object, typedObject); err != nil {
			return fmt.Errorf("failed to convert unstructured object to %T: %w", typedObject, err)
		}
		return nil
	}, "5s", "1s").Should(Succeed())
}

// patchObject is a helper that patches a key within the given Kubernetes object. This is needed to simulate invalid
// values passed in through YAML to the api-server, bypassing the type-checks of the Go client.
func patchObject[T client.Object](object T, fieldPath []string, value any) (*unstructured.Unstructured, error) {
	// Convert the object to a vanilla Go map
	unstructuredObject, err := runtime.DefaultUnstructuredConverter.ToUnstructured(object)
	if err != nil {
		return nil, fmt.Errorf("failed to convert EtcdCopyBackupsTask to unstructured object: %v", err)
	}

	// Navigate to the specified path and set the value
	currentObject := unstructuredObject
	for _, key := range fieldPath[:len(fieldPath)-1] {
		// Initialize the map for the sub-path if it does not exist to avoid runtime error in the next iteration.
		if _, exists := currentObject[key]; !exists {
			currentObject[key] = make(map[string]any)
		}
		currentObject = currentObject[key].(map[string]any)
	}
	currentObject[fieldPath[len(fieldPath)-1]] = value

	return &unstructured.Unstructured{
		Object: unstructuredObject,
	}, nil
}

// setupTestEnvironment performs the initial setup for namespace and test environment
func setupTestEnvironment(t *testing.T) (string, *WithT) {
	g := NewWithT(t)
	testNs := utils.GenerateTestNamespaceName(t, testNamespacePrefix, 8)

	t.Logf("Successfully created namespace: %s to run test => '%s'", testNs, t.Name())
	t.Log("Setting up Client")

	g.Expect(itTestEnv.CreateManager(utils.NewTestClientBuilder())).To(Succeed())
	g.Expect(itTestEnv.CreateTestNamespace(testNs)).To(Succeed())
	g.Expect(itTestEnv.StartManager()).To(Succeed())

	return testNs, g
}
