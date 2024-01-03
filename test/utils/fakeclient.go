package utils

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// FakeClientBuilder builds a fake client with initial set of objects and additionally
// provides an ability to set custom errors for operations supported on the client.
type FakeClientBuilder struct {
	clientBuilder *fakeclient.ClientBuilder
	getErr        error
	createErr     error
	patchErr      error
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
func (b *FakeClientBuilder) WithGetError(err error) *FakeClientBuilder {
	b.getErr = err
	return b
}

// Build returns an instance of client.WithWatch which has capability to return the configured errors for operations.
func (b *FakeClientBuilder) Build() client.WithWatch {
	return &fakeClient{
		WithWatch: b.clientBuilder.Build(),
		getErr:    b.getErr,
		createErr: b.createErr,
		patchErr:  b.patchErr,
	}
}

type fakeClient struct {
	client.WithWatch
	getErr    error
	createErr error
	patchErr  error
}

// Get overwrites the fake client Get implementation with a capability to return any configured error.
func (f *fakeClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if f.getErr != nil {
		return f.getErr
	}
	return f.WithWatch.Get(ctx, key, obj, opts...)
}
