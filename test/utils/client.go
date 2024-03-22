// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// ClientMethod is a name of the method on client.Client for which an error is recorded.
type ClientMethod string

const (
	// ClientMethodGet is the name of the Get method on client.Client.
	ClientMethodGet ClientMethod = "Get"
	// ClientMethodList is the name of the List method on client.Client.
	ClientMethodList ClientMethod = "List"
	// ClientMethodCreate is the name of the Create method on client.Client.
	ClientMethodCreate ClientMethod = "Create"
	// ClientMethodDelete is the name of the Delete method on client.Client.
	ClientMethodDelete ClientMethod = "Delete"
	// ClientMethodDeleteAll is the name of the DeleteAllOf method on client.Client.
	ClientMethodDeleteAll ClientMethod = "DeleteAll"
	// ClientMethodPatch is the name of the Patch method on client.Client.
	ClientMethodPatch ClientMethod = "Patch"
	// ClientMethodUpdate is the name of the Update method on client.Client.
	ClientMethodUpdate ClientMethod = "Update"
)

// errorRecord contains the recorded error for a specific client.Client method and identifiers such as name, namespace and matching labels.
type errorRecord struct {
	method            ClientMethod
	resourceName      string
	resourceNamespace string
	labels            labels.Set
	resourceGVK       schema.GroupVersionKind
	err               error
}

type ErrorsForGVK struct {
	GVK       schema.GroupVersionKind
	DeleteErr *apierrors.StatusError
	ListErr   *apierrors.StatusError
}

// TestClientBuilder builds a client.Client which will also react to the configured errors.
type TestClientBuilder struct {
	delegatingClient client.Client
	errorRecords     []errorRecord
}

// CreateTestFakeClientForObjects is a convenience function which creates a test client which uses a fake client as a delegate and reacts to the configured errors for the given object key.
func CreateTestFakeClientForObjects(getErr, createErr, patchErr, deleteErr *apierrors.StatusError, existingObjects []client.Object, objKeys ...client.ObjectKey) client.Client {
	fakeDelegateClientBuilder := fake.NewClientBuilder()
	if existingObjects != nil && len(existingObjects) > 0 {
		fakeDelegateClientBuilder.WithObjects(existingObjects...)
	}
	fakeDelegateClient := fakeDelegateClientBuilder.Build()
	testClientBuilder := NewTestClientBuilder().WithClient(fakeDelegateClient)
	for _, objKey := range objKeys {
		testClientBuilder.RecordErrorForObjects(ClientMethodGet, getErr, objKey).
			RecordErrorForObjects(ClientMethodCreate, createErr, objKey).
			RecordErrorForObjects(ClientMethodDelete, deleteErr, objKey).
			RecordErrorForObjects(ClientMethodPatch, patchErr, objKey)
	}
	return testClientBuilder.Build()
}

// CreateTestFakeClientForAllObjectsInNamespace is a convenience function which creates a test client which uses a fake client as a delegate and reacts to the configured errors for all objects in the given namespace matching the given labels.
func CreateTestFakeClientForAllObjectsInNamespace(deleteAllErr, listErr *apierrors.StatusError, namespace string, matchingLabels map[string]string, existingObjects ...client.Object) client.Client {
	fakeDelegateClientBuilder := fake.NewClientBuilder()
	if existingObjects != nil && len(existingObjects) > 0 {
		fakeDelegateClientBuilder.WithObjects(existingObjects...)
	}
	fakeDelegateClient := fakeDelegateClientBuilder.Build()
	cl := NewTestClientBuilder().
		WithClient(fakeDelegateClient).
		RecordErrorForObjectsMatchingLabels(ClientMethodDeleteAll, namespace, matchingLabels, deleteAllErr).
		RecordErrorForObjectsMatchingLabels(ClientMethodList, namespace, matchingLabels, listErr).
		Build()
	return cl
}

// CreateTestFakeClientForObjectsInNamespaceWithGVK is a convenience function which creates a test client which uses a fake client as a delegate and reacts to the configured errors for all objects in the given namespace for the given GroupVersionKinds.
func CreateTestFakeClientForObjectsInNamespaceWithGVK(errors []ErrorsForGVK, namespace string, existingObjects ...client.Object) client.Client {
	fakeDelegateClientBuilder := fake.NewClientBuilder()
	if existingObjects != nil && len(existingObjects) > 0 {
		fakeDelegateClientBuilder.WithObjects(existingObjects...)
	}
	fakeDelegateClient := fakeDelegateClientBuilder.Build()

	cl := NewTestClientBuilder().WithClient(fakeDelegateClient)

	for _, e := range errors {
		cl.RecordErrorForObjectsWithGVK(ClientMethodDeleteAll, namespace, e.GVK, e.DeleteErr).
			RecordErrorForObjectsWithGVK(ClientMethodList, namespace, e.GVK, e.ListErr)
	}
	return cl.Build()
}

// NewTestClientBuilder creates a new instance of TestClientBuilder.
func NewTestClientBuilder() *TestClientBuilder {
	return &TestClientBuilder{}
}

// WithClient sets the client.Client to be used as a delegate for the test client.
func (b *TestClientBuilder) WithClient(client client.Client) *TestClientBuilder {
	b.delegatingClient = client
	return b
}

// RecordErrorForObjects records an error for a specific client.Client method and object keys.
func (b *TestClientBuilder) RecordErrorForObjects(method ClientMethod, err *apierrors.StatusError, objectKeys ...client.ObjectKey) *TestClientBuilder {
	// this method records error, so if nil error is passed then there is no need to create any error record.
	if err == nil {
		return b
	}
	for _, objectKey := range objectKeys {
		b.errorRecords = append(b.errorRecords, errorRecord{
			method:            method,
			resourceName:      objectKey.Name,
			resourceNamespace: objectKey.Namespace,
			err:               err,
		})
	}
	return b
}

// RecordErrorForObjectsMatchingLabels records an error for a specific client.Client method and objects in a given namespace matching the given labels.
func (b *TestClientBuilder) RecordErrorForObjectsMatchingLabels(method ClientMethod, namespace string, matchingLabels map[string]string, err *apierrors.StatusError) *TestClientBuilder {
	// this method records error, so if nil error is passed then there is no need to create any error record.
	if err == nil {
		return b
	}
	b.errorRecords = append(b.errorRecords, errorRecord{
		method:            method,
		resourceNamespace: namespace,
		labels:            createLabelSet(matchingLabels),
		err:               err,
	})
	return b
}

// RecordErrorForObjectsWithGVK records an error for a specific client.Client method and objects in a given namespace of a given GroupVersionKind.
func (b *TestClientBuilder) RecordErrorForObjectsWithGVK(method ClientMethod, namespace string, gvk schema.GroupVersionKind, err *apierrors.StatusError) *TestClientBuilder {
	// this method records error, so if nil error is passed then there is no need to create any error record.
	if err == nil {
		return b
	}
	b.errorRecords = append(b.errorRecords, errorRecord{
		method:            method,
		resourceGVK:       gvk,
		resourceNamespace: namespace,
		err:               err,
	})
	return b
}

// Build creates a new instance of client.Client which will react to the configured errors.
func (b *TestClientBuilder) Build() client.Client {
	return &testClient{
		delegate:     b.delegatingClient,
		errorRecords: b.errorRecords,
	}
}

// testClient is a client.Client implementation which reacts to the configured errors.
type testClient struct {
	delegate     client.Client
	errorRecords []errorRecord
}

// ---------------------------------- Implementation of client.Client ----------------------------------

func (c *testClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if err := c.getRecordedObjectError(ClientMethodGet, key); err != nil {
		return err
	}
	return c.delegate.Get(ctx, key, obj, opts...)
}

func (c *testClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	listOpts := client.ListOptions{}
	listOpts.ApplyOptions(opts)

	gvk, err := apiutil.GVKForObject(list, c.delegate.Scheme())
	if err != nil {
		return err
	}

	if err := c.getRecordedObjectCollectionError(ClientMethodList, listOpts.Namespace, listOpts.LabelSelector, gvk); err != nil {
		return err
	}
	return c.delegate.List(ctx, list, opts...)
}

func (c *testClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if err := c.getRecordedObjectError(ClientMethodCreate, client.ObjectKeyFromObject(obj)); err != nil {
		return err
	}
	return c.delegate.Create(ctx, obj, opts...)
}

func (c *testClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	if err := c.getRecordedObjectError(ClientMethodDelete, client.ObjectKeyFromObject(obj)); err != nil {
		return err
	}
	return c.delegate.Delete(ctx, obj, opts...)
}

func (c *testClient) DeleteAllOf(ctx context.Context, obj client.Object, opts ...client.DeleteAllOfOption) error {
	deleteOpts := client.DeleteAllOfOptions{}
	deleteOpts.ApplyOptions(opts)
	if err := c.getRecordedObjectCollectionError(ClientMethodDeleteAll, deleteOpts.Namespace, deleteOpts.LabelSelector, obj.GetObjectKind().GroupVersionKind()); err != nil {
		return err
	}
	return c.delegate.DeleteAllOf(ctx, obj, opts...)
}

func (c *testClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	if err := c.getRecordedObjectError(ClientMethodPatch, client.ObjectKeyFromObject(obj)); err != nil {
		return err
	}
	return c.delegate.Patch(ctx, obj, patch, opts...)
}

func (c *testClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	if err := c.getRecordedObjectError(ClientMethodUpdate, client.ObjectKeyFromObject(obj)); err != nil {
		return err
	}
	return c.delegate.Update(ctx, obj, opts...)
}

func (c *testClient) Status() client.SubResourceWriter {
	return c.delegate.Status()
}

func (c *testClient) SubResource(subResource string) client.SubResourceClient {
	return c.delegate.SubResource(subResource)
}

func (c *testClient) Scheme() *runtime.Scheme {
	return c.delegate.Scheme()
}

func (c *testClient) RESTMapper() meta.RESTMapper {
	return c.delegate.RESTMapper()
}

// ---------------------------------- Helper methods ----------------------------------
func (c *testClient) getRecordedObjectError(method ClientMethod, objKey client.ObjectKey) error {
	for _, errRecord := range c.errorRecords {
		recordedObjKey := client.ObjectKey{Name: errRecord.resourceName, Namespace: errRecord.resourceNamespace}
		if errRecord.method == method && recordedObjKey == objKey {
			return errRecord.err
		}
	}
	return nil
}

func (c *testClient) getRecordedObjectCollectionError(method ClientMethod, namespace string, labelSelector labels.Selector, objGVK schema.GroupVersionKind) error {
	for _, errRecord := range c.errorRecords {
		if errRecord.method == method && errRecord.resourceNamespace == namespace {
			if errRecord.resourceGVK == objGVK || (labelSelector == nil && errRecord.labels == nil) || labelSelector.Matches(errRecord.labels) {
				return errRecord.err
			}
		}
	}
	return nil
}

func createLabelSet(l map[string]string) labels.Set {
	s := labels.Set{}
	for k, v := range l {
		s[k] = v
	}
	return s
}
