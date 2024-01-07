package utils

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// FakeClientBuilder builds a fake client with initial set of objects and additionally
// provides an ability to set custom errors for operations supported on the client.
type FakeClientBuilder struct {
	clientBuilder *fakeclient.ClientBuilder
	getErr        *apierrors.StatusError
	listErr       *apierrors.StatusError
	createErr     *apierrors.StatusError
	patchErr      *apierrors.StatusError
	deleteErr     *apierrors.StatusError
}

// NewFakeClientBuilder creates a new FakeClientBuilder.
func NewFakeClientBuilder() *FakeClientBuilder {
	return &FakeClientBuilder{
		clientBuilder: fakeclient.NewClientBuilder(),
	}
}

// WithObjects initializes the underline controller-runtime fake client with objects.
func (b *FakeClientBuilder) WithObjects(objs ...client.Object) *FakeClientBuilder {
	b.clientBuilder.WithObjects(objs...)
	return b
}

// WithGetError sets the error that should be returned when a Get request is made on the fake client.
func (b *FakeClientBuilder) WithGetError(err *apierrors.StatusError) *FakeClientBuilder {
	b.getErr = err
	return b
}

func (b *FakeClientBuilder) WithListError(err *apierrors.StatusError) *FakeClientBuilder {
	b.listErr = err
	return b
}

// WithCreateError sets the error that should be returned when a Create request is made on the fake client.
func (b *FakeClientBuilder) WithCreateError(err *apierrors.StatusError) *FakeClientBuilder {
	b.createErr = err
	return b
}

// WithPatchError sets the error that should be returned when a Patch request is made on the fake client.
func (b *FakeClientBuilder) WithPatchError(err *apierrors.StatusError) *FakeClientBuilder {
	b.patchErr = err
	return b
}

// WithDeleteError sets the error that should be returned when a Delete request is made on the fake client.
func (b *FakeClientBuilder) WithDeleteError(err *apierrors.StatusError) *FakeClientBuilder {
	b.deleteErr = err
	return b
}

// Build returns an instance of client.WithWatch which has capability to return the configured errors for operations.
func (b *FakeClientBuilder) Build() client.WithWatch {
	return &fakeClient{
		WithWatch: b.clientBuilder.Build(),
		getErr:    b.getErr,
		listErr:   b.listErr,
		createErr: b.createErr,
		patchErr:  b.patchErr,
		deleteErr: b.deleteErr,
	}
}

type fakeClient struct {
	client.WithWatch
	getErr    *apierrors.StatusError
	listErr   *apierrors.StatusError
	createErr *apierrors.StatusError
	patchErr  *apierrors.StatusError
	deleteErr *apierrors.StatusError
}

// Get overwrites the fake client Get implementation with a capability to return any configured error.
func (f *fakeClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if f.getErr != nil {
		return f.getErr
	}
	return f.WithWatch.Get(ctx, key, obj, opts...)
}

func (f *fakeClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	if f.listErr != nil {
		return f.listErr
	}
	return f.WithWatch.List(ctx, list, opts...)
}

// Delete overwrites the fake client Get implementation with a capability to return any configured error.
func (f *fakeClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	return f.WithWatch.Delete(ctx, obj, opts...)
}

func (f *fakeClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	if f.patchErr != nil {
		return f.patchErr
	}
	return f.WithWatch.Patch(ctx, obj, patch, opts...)
}

func (f *fakeClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if f.createErr != nil {
		return f.createErr
	}
	return f.WithWatch.Create(ctx, obj, opts...)
}
