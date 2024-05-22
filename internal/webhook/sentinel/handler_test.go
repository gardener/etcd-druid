// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package sentinel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/internal/client/kubernetes"
	testutils "github.com/gardener/etcd-druid/test/utils"

	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
	admissionv1 "k8s.io/api/admission/v1"
	appsv1 "k8s.io/api/apps/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/uuid"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

const (
	testUserName   = "test-user"
	testObjectName = "test"
	testNamespace  = "test-ns"
	testEtcdName   = "test"
)

var (
	errInternal              = errors.New("test internal error")
	apiInternalErr           = apierrors.NewInternalError(errInternal)
	apiNotFoundErr           = apierrors.NewNotFound(schema.GroupResource{}, "")
	reconcilerServiceAccount = "etcd-druid-sa"
	exemptServiceAccounts    = []string{"exempt-sa-1"}

	statefulSetGVK      = metav1.GroupVersionKind{Group: "apps", Version: "v1", Kind: "StatefulSet"}
	statefulSetGVR      = metav1.GroupVersionResource{Group: "apps", Version: "v1", Resource: "statefulsets"}
	scaleSubresourceGVK = metav1.GroupVersionKind{Group: "autoscaling", Version: "v1", Kind: "Scale"}
	leaseGVK            = metav1.GroupVersionKind{Group: "coordination.k8s.io", Version: "v1", Kind: "Lease"}
)

func TestHandleCreateAndConnect(t *testing.T) {
	g := NewWithT(t)

	testCases := []struct {
		name        string
		operation   admissionv1.Operation
		expectedMsg string
	}{
		{
			name:        "allow create operation for any resource",
			operation:   admissionv1.Create,
			expectedMsg: "operation CREATE is allowed",
		},
		{
			name:        "allow connect operation for any resource",
			operation:   admissionv1.Connect,
			expectedMsg: "operation CONNECT is allowed",
		},
	}

	cl := testutils.CreateDefaultFakeClient()
	decoder := admission.NewDecoder(cl.Scheme())

	handler := &Handler{
		Client: cl,
		config: &Config{
			Enabled: true,
		},
		decoder: decoder,
		logger:  logr.Discard(),
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resp := handler.Handle(context.Background(), admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: tc.operation,
					// Create for all resources are allowed. StatefulSet resource GVK has been taken as an example.
					Kind: statefulSetGVK,
				},
			})
			g.Expect(resp.Allowed).To(BeTrue())
			g.Expect(resp.Result.Message).To(Equal(tc.expectedMsg))
		})
	}
}

func TestHandleLeaseUpdate(t *testing.T) {
	g := NewWithT(t)

	testCases := []struct {
		name                  string
		useEtcdServiceAccount bool
		expectedAllowed       bool
		expectedMessage       string
		expectedCode          int32
	}{
		{
			name:                  "request is from Etcd service account",
			useEtcdServiceAccount: true,
			expectedAllowed:       true,
			expectedMessage:       "lease resource can be freely updated by etcd members",
			expectedCode:          http.StatusOK,
		},
		{
			name:                  "request is not from Etcd service account",
			useEtcdServiceAccount: false,
			expectedAllowed:       false,
			expectedMessage:       fmt.Sprintf("changes disallowed, since no ongoing processing of Etcd %s by etcd-druid", testEtcdName),
			expectedCode:          http.StatusForbidden,
		},
	}

	t.Parallel()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			etcd := testutils.EtcdBuilderWithDefaults(testEtcdName, testNamespace).Build()
			cl := testutils.CreateTestFakeClientWithSchemeForObjects(kubernetes.Scheme, nil, nil, nil, nil, []client.Object{etcd}, client.ObjectKey{Name: testEtcdName, Namespace: testNamespace})
			decoder := admission.NewDecoder(cl.Scheme())

			handler := &Handler{
				Client: cl,
				config: &Config{
					Enabled: true,
				},
				decoder: decoder,
				logger:  logr.Discard(),
			}

			obj := buildObjRawExtension(g, &coordinationv1.Lease{}, nil, testObjectName, testNamespace,
				map[string]string{druidv1alpha1.LabelManagedByKey: druidv1alpha1.LabelManagedByValue, druidv1alpha1.LabelPartOfKey: testEtcdName})

			username := testUserName
			if tc.useEtcdServiceAccount {
				username = fmt.Sprintf("system:serviceaccount:%s:%s", testNamespace, etcd.GetServiceAccountName())
			}

			response := handler.Handle(context.Background(), admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Update,
					UserInfo:  authenticationv1.UserInfo{Username: username},
					Kind:      leaseGVK,
					Name:      testObjectName,
					Namespace: testNamespace,
					Object:    obj,
					OldObject: obj,
				},
			})

			g.Expect(response.Allowed).To(Equal(tc.expectedAllowed))
			g.Expect(response.Result.Message).To(ContainSubstring(tc.expectedMessage))
			g.Expect(response.Result.Code).To(Equal(tc.expectedCode))
		})
	}
}

func TestHandleStatefulSetScaleSubresourceUpdate(t *testing.T) {
	g := NewWithT(t)

	// create sts without managed-by label
	sts := testutils.CreateStatefulSet(testEtcdName, testNamespace, uuid.NewUUID(), 1)
	delete(sts.Labels, druidv1alpha1.LabelManagedByKey)

	cl := testutils.CreateTestFakeClientWithSchemeForObjects(kubernetes.Scheme, nil, nil, nil, nil, []client.Object{sts}, client.ObjectKey{Name: testEtcdName, Namespace: testNamespace})
	decoder := admission.NewDecoder(cl.Scheme())

	handler := &Handler{
		Client: cl,
		config: &Config{
			Enabled: true,
		},
		decoder: decoder,
		logger:  logr.Discard(),
	}

	obj := buildObjRawExtension(g, &autoscalingv1.Scale{}, nil, testObjectName, testNamespace, nil)

	response := handler.Handle(context.Background(), admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation:   admissionv1.Update,
			Kind:        scaleSubresourceGVK,
			Resource:    statefulSetGVR,
			SubResource: "Scale",
			Name:        testObjectName,
			Namespace:   testNamespace,
			Object:      obj,
			OldObject:   obj,
		},
	})

	g.Expect(response.Allowed).To(BeTrue())
	g.Expect(response.Result.Message).To(Equal(fmt.Sprintf("resource is not managed by etcd-druid, as label %s is missing", druidv1alpha1.LabelManagedByKey)))
	g.Expect(response.Result.Code).To(Equal(int32(http.StatusOK)))
}

func TestUnexpectedResourceType(t *testing.T) {
	g := NewWithT(t)

	cl := fake.NewClientBuilder().Build()
	decoder := admission.NewDecoder(cl.Scheme())

	handler := &Handler{
		Client: cl,
		config: &Config{
			Enabled: true,
		},
		decoder: decoder,
		logger:  logr.Discard(),
	}

	resp := handler.Handle(context.Background(), admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: admissionv1.Update,
			Kind:      metav1.GroupVersionKind{Group: "coordination.k8s.io", Version: "v1", Kind: "Unknown"},
		},
	})

	g.Expect(resp.Allowed).To(BeTrue())
	g.Expect(resp.Result.Message).To(Equal("unexpected resource type: coordination.k8s.io/Unknown"))
}

func TestMissingManagedByLabel(t *testing.T) {
	g := NewWithT(t)

	cl := fake.NewClientBuilder().Build()
	decoder := admission.NewDecoder(cl.Scheme())

	handler := &Handler{
		Client: cl,
		config: &Config{
			Enabled: true,
		},
		decoder: decoder,
		logger:  logr.Discard(),
	}

	obj := buildObjRawExtension(g, &appsv1.StatefulSet{}, nil, testObjectName, testNamespace, map[string]string{druidv1alpha1.LabelPartOfKey: testEtcdName})
	response := handler.Handle(context.Background(), admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: admissionv1.Update,
			UserInfo:  authenticationv1.UserInfo{Username: testUserName},
			Kind:      statefulSetGVK,
			Name:      testObjectName,
			Namespace: testNamespace,
			Object:    obj,
			OldObject: obj,
		},
	})

	g.Expect(response.Allowed).To(Equal(true))
	g.Expect(response.Result.Message).To(Equal(fmt.Sprintf("resource is not managed by etcd-druid, as label %s is missing", druidv1alpha1.LabelManagedByKey)))
}

func TestMissingResourcePartOfLabel(t *testing.T) {
	g := NewWithT(t)

	cl := fake.NewClientBuilder().Build()
	decoder := admission.NewDecoder(cl.Scheme())

	handler := &Handler{
		Client: cl,
		config: &Config{
			Enabled: true,
		},
		decoder: decoder,
		logger:  logr.Discard(),
	}

	obj := buildObjRawExtension(g, &appsv1.StatefulSet{}, nil, testObjectName, testNamespace, map[string]string{druidv1alpha1.LabelManagedByKey: druidv1alpha1.LabelManagedByValue})
	response := handler.Handle(context.Background(), admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: admissionv1.Update,
			UserInfo:  authenticationv1.UserInfo{Username: testUserName},
			Kind:      statefulSetGVK,
			Name:      testObjectName,
			Namespace: testNamespace,
			Object:    obj,
			OldObject: obj,
		},
	})

	g.Expect(response.Allowed).To(Equal(true))
	g.Expect(response.Result.Message).To(Equal(fmt.Sprintf("label %s not found on resource", druidv1alpha1.LabelPartOfKey)))
}

func TestHandleUpdate(t *testing.T) {
	g := NewWithT(t)

	testCases := []struct {
		name string
		// ----- request -----
		userName     string
		objectLabels map[string]string
		objectRaw    []byte
		// ----- etcd configuration -----
		etcdAnnotations         map[string]string
		etcdStatusLastOperation *druidv1alpha1.LastOperation
		etcdGetErr              *apierrors.StatusError
		// ----- handler configuration -----
		reconcilerServiceAccount string
		exemptServiceAccounts    []string
		// ----- expected -----
		expectedAllowed bool
		expectedMessage string
		expectedCode    int32
	}{
		{
			name:            "disable resource protection annotation set",
			objectLabels:    map[string]string{druidv1alpha1.LabelManagedByKey: druidv1alpha1.LabelManagedByValue, druidv1alpha1.LabelPartOfKey: testEtcdName},
			etcdAnnotations: map[string]string{druidv1alpha1.DisableResourceProtectionAnnotation: ""},
			expectedAllowed: true,
			expectedMessage: fmt.Sprintf("changes allowed, since Etcd %s has annotation %s", testEtcdName, druidv1alpha1.DisableResourceProtectionAnnotation),
			expectedCode:    http.StatusOK,
		},
		{
			name:                     "operator makes a request when Etcd is being reconciled by druid",
			userName:                 testUserName,
			objectLabels:             map[string]string{druidv1alpha1.LabelManagedByKey: druidv1alpha1.LabelManagedByValue, druidv1alpha1.LabelPartOfKey: testEtcdName},
			etcdStatusLastOperation:  &druidv1alpha1.LastOperation{Type: druidv1alpha1.LastOperationTypeReconcile, State: druidv1alpha1.LastOperationStateProcessing},
			reconcilerServiceAccount: reconcilerServiceAccount,
			expectedAllowed:          false,
			expectedMessage:          fmt.Sprintf("no external intervention allowed during ongoing reconciliation of Etcd %s by etcd-druid", testEtcdName),
			expectedCode:             http.StatusForbidden,
		},
		{
			name:                     "druid makes a request during its reconciliation run",
			userName:                 reconcilerServiceAccount,
			objectLabels:             map[string]string{druidv1alpha1.LabelManagedByKey: druidv1alpha1.LabelManagedByValue, druidv1alpha1.LabelPartOfKey: testEtcdName},
			etcdStatusLastOperation:  &druidv1alpha1.LastOperation{Type: druidv1alpha1.LastOperationTypeReconcile, State: druidv1alpha1.LastOperationStateProcessing},
			reconcilerServiceAccount: reconcilerServiceAccount,
			expectedAllowed:          true,
			expectedMessage:          fmt.Sprintf("ongoing reconciliation of Etcd %s by etcd-druid requires changes to resources", testEtcdName),
			expectedCode:             http.StatusOK,
		},
		{
			name:                     "Etcd is not currently being reconciled by druid, and request is from exempt service account",
			userName:                 exemptServiceAccounts[0],
			objectLabels:             map[string]string{druidv1alpha1.LabelManagedByKey: druidv1alpha1.LabelManagedByValue, druidv1alpha1.LabelPartOfKey: testEtcdName},
			reconcilerServiceAccount: reconcilerServiceAccount,
			exemptServiceAccounts:    exemptServiceAccounts,
			expectedAllowed:          true,
			expectedMessage:          fmt.Sprintf("operations on Etcd %s by service account %s is exempt from Sentinel Webhook checks", testEtcdName, exemptServiceAccounts[0]),
			expectedCode:             http.StatusOK,
		},
		{
			name:                     "Etcd is not currently being reconciled by druid, and request is from non-exempt service account",
			userName:                 testUserName,
			objectLabels:             map[string]string{druidv1alpha1.LabelManagedByKey: druidv1alpha1.LabelManagedByValue, druidv1alpha1.LabelPartOfKey: testEtcdName},
			reconcilerServiceAccount: reconcilerServiceAccount,
			exemptServiceAccounts:    exemptServiceAccounts,
			expectedAllowed:          false,
			expectedMessage:          fmt.Sprintf("changes disallowed, since no ongoing processing of Etcd %s by etcd-druid", testEtcdName),
			expectedCode:             http.StatusForbidden,
		},
	}

	t.Parallel()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			etcd := testutils.EtcdBuilderWithDefaults(testEtcdName, testNamespace).
				WithAnnotations(tc.etcdAnnotations).
				WithLastOperation(tc.etcdStatusLastOperation).
				Build()

			cl := testutils.CreateTestFakeClientWithSchemeForObjects(kubernetes.Scheme, tc.etcdGetErr, nil, nil, nil, []client.Object{etcd}, client.ObjectKey{Name: testEtcdName, Namespace: testNamespace})
			decoder := admission.NewDecoder(cl.Scheme())

			handler := &Handler{
				Client: cl,
				config: &Config{
					Enabled:                  true,
					ReconcilerServiceAccount: reconcilerServiceAccount,
					ExemptServiceAccounts:    exemptServiceAccounts,
				},
				decoder: decoder,
				logger:  logr.Discard(),
			}

			obj := buildObjRawExtension(g, &appsv1.StatefulSet{}, tc.objectRaw, testObjectName, testNamespace, tc.objectLabels)
			response := handler.Handle(context.Background(), admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Update,
					UserInfo:  authenticationv1.UserInfo{Username: tc.userName},
					Kind:      statefulSetGVK,
					Name:      testObjectName,
					Namespace: testNamespace,
					Object:    obj,
					OldObject: obj,
				},
			})

			g.Expect(response.Allowed).To(Equal(tc.expectedAllowed))
			g.Expect(response.Result.Message).To(ContainSubstring(tc.expectedMessage))
			g.Expect(response.Result.Code).To(Equal(tc.expectedCode))
		})
	}
}

func TestHandleWithInvalidRequestObject(t *testing.T) {
	g := NewWithT(t)
	testCases := []struct {
		name              string
		operation         admissionv1.Operation
		objectRaw         []byte
		expectedAllowed   bool
		expectedMessage   string
		expectedErrorCode int32
	}{
		{
			name:              "empty request object",
			operation:         admissionv1.Update,
			objectRaw:         []byte{},
			expectedAllowed:   false,
			expectedMessage:   "there is no content to decode",
			expectedErrorCode: http.StatusInternalServerError,
		},
		{
			name:              "malformed request object",
			operation:         admissionv1.Update,
			objectRaw:         []byte("foo"),
			expectedAllowed:   false,
			expectedMessage:   "invalid character",
			expectedErrorCode: http.StatusInternalServerError,
		},
		{
			name:              "empty request object",
			operation:         admissionv1.Delete,
			objectRaw:         []byte{},
			expectedAllowed:   false,
			expectedMessage:   "there is no content to decode",
			expectedErrorCode: http.StatusInternalServerError,
		},
		{
			name:              "malformed request object",
			operation:         admissionv1.Delete,
			objectRaw:         []byte("foo"),
			expectedAllowed:   false,
			expectedMessage:   "invalid character",
			expectedErrorCode: http.StatusInternalServerError,
		},
	}

	t.Parallel()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cl := testutils.CreateDefaultFakeClient()
			decoder := admission.NewDecoder(cl.Scheme())

			handler := &Handler{
				Client: cl,
				config: &Config{
					Enabled:                  true,
					ReconcilerServiceAccount: reconcilerServiceAccount,
					ExemptServiceAccounts:    exemptServiceAccounts,
				},
				decoder: decoder,
				logger:  logr.Discard(),
			}

			obj := buildObjRawExtension(g, &appsv1.StatefulSet{}, tc.objectRaw, testObjectName, testNamespace, nil)

			response := handler.Handle(context.Background(), admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Update,
					UserInfo:  authenticationv1.UserInfo{Username: testUserName},
					Kind:      metav1.GroupVersionKind{Group: "apps", Version: "v1", Kind: "StatefulSet"},
					Name:      testObjectName,
					Namespace: testNamespace,
					Object:    obj,
					OldObject: obj,
				},
			})
			g.Expect(response.Allowed).To(Equal(tc.expectedAllowed))
			g.Expect(response.Result.Message).To(ContainSubstring(tc.expectedMessage))
			g.Expect(response.Result.Code).To(Equal(tc.expectedErrorCode))
		})
	}
}

func TestEtcdGetFailures(t *testing.T) {
	g := NewWithT(t)
	testCases := []struct {
		name            string
		etcdGetErr      *apierrors.StatusError
		expectedAllowed bool
		expectedReason  string
		expectedMessage string
		expectedCode    int32
	}{
		{
			name:            "should allow when Etcd is not found",
			etcdGetErr:      apiNotFoundErr,
			expectedAllowed: true,
			expectedReason:  fmt.Sprintf("corresponding Etcd %s not found", testEtcdName),
			expectedCode:    http.StatusOK,
		},
		{
			name:            "error in getting Etcd",
			etcdGetErr:      apiInternalErr,
			expectedAllowed: false,
			expectedMessage: errInternal.Error(),
			expectedCode:    http.StatusInternalServerError,
		},
	}

	t.Parallel()
	etcd := testutils.EtcdBuilderWithDefaults(testEtcdName, testNamespace).Build()
	for _, tc := range testCases {
		t.Run(t.Name(), func(t *testing.T) {
			cl := testutils.CreateTestFakeClientWithSchemeForObjects(kubernetes.Scheme, tc.etcdGetErr, nil, nil, nil, []client.Object{etcd}, client.ObjectKey{Name: testEtcdName, Namespace: testNamespace})
			decoder := admission.NewDecoder(cl.Scheme())

			handler := &Handler{
				Client: cl,
				config: &Config{
					Enabled:                  true,
					ReconcilerServiceAccount: reconcilerServiceAccount,
					ExemptServiceAccounts:    exemptServiceAccounts,
				},
				decoder: decoder,
				logger:  logr.Discard(),
			}

			obj := buildObjRawExtension(g, &appsv1.StatefulSet{}, nil, testObjectName, testNamespace, map[string]string{
				druidv1alpha1.LabelPartOfKey:    testEtcdName,
				druidv1alpha1.LabelManagedByKey: druidv1alpha1.LabelManagedByValue,
			})

			response := handler.Handle(context.Background(), admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Delete,
					UserInfo:  authenticationv1.UserInfo{Username: testUserName},
					Kind:      metav1.GroupVersionKind{Group: "apps", Version: "v1", Kind: "StatefulSet"},
					Name:      testObjectName,
					Namespace: testNamespace,
					OldObject: obj,
				},
			})

			g.Expect(response.Allowed).To(Equal(tc.expectedAllowed))
			g.Expect(response.Result.Message).To(ContainSubstring(tc.expectedMessage))
			g.Expect(response.Result.Code).To(Equal(tc.expectedCode))
		})
	}
}

func TestHandleDelete(t *testing.T) {
	g := NewWithT(t)

	testCases := []struct {
		name string
		// ----- request -----
		userName     string
		objectLabels map[string]string
		objectRaw    []byte
		// ----- etcd configuration -----
		etcdAnnotations         map[string]string
		etcdStatusLastOperation *druidv1alpha1.LastOperation
		etcdGetErr              *apierrors.StatusError
		// ----- handler configuration -----
		reconcilerServiceAccount string
		exemptServiceAccounts    []string
		// ----- expected -----
		expectedAllowed bool
		expectedReason  string
		expectedMessage string
		expectedCode    int32
	}{
		{
			name:            "disable resource protection annotation set",
			objectLabels:    map[string]string{druidv1alpha1.LabelManagedByKey: druidv1alpha1.LabelManagedByValue, druidv1alpha1.LabelPartOfKey: testEtcdName},
			etcdAnnotations: map[string]string{druidv1alpha1.DisableResourceProtectionAnnotation: ""},
			expectedAllowed: true,
			expectedMessage: fmt.Sprintf("changes allowed, since Etcd %s has annotation %s", testEtcdName, druidv1alpha1.DisableResourceProtectionAnnotation),
			expectedCode:    http.StatusOK,
		},
		{
			name:                     "Etcd is currently being reconciled by druid, and request is from non-exempt service account",
			userName:                 testUserName,
			objectLabels:             map[string]string{druidv1alpha1.LabelManagedByKey: druidv1alpha1.LabelManagedByValue, druidv1alpha1.LabelPartOfKey: testEtcdName},
			etcdStatusLastOperation:  &druidv1alpha1.LastOperation{Type: druidv1alpha1.LastOperationTypeReconcile, State: druidv1alpha1.LastOperationStateProcessing},
			reconcilerServiceAccount: reconcilerServiceAccount,
			expectedAllowed:          false,
			expectedReason:           "Forbidden",
			expectedMessage:          fmt.Sprintf("no external intervention allowed during ongoing reconciliation of Etcd %s by etcd-druid", testEtcdName),
			expectedCode:             http.StatusForbidden,
		},
		{
			name:                     "Etcd is currently being reconciled by druid, and request is from druid",
			userName:                 reconcilerServiceAccount,
			objectLabels:             map[string]string{druidv1alpha1.LabelManagedByKey: druidv1alpha1.LabelManagedByValue, druidv1alpha1.LabelPartOfKey: testEtcdName},
			etcdStatusLastOperation:  &druidv1alpha1.LastOperation{Type: druidv1alpha1.LastOperationTypeReconcile, State: druidv1alpha1.LastOperationStateProcessing},
			reconcilerServiceAccount: reconcilerServiceAccount,
			expectedAllowed:          true,
			expectedMessage:          fmt.Sprintf("ongoing reconciliation of Etcd %s by etcd-druid requires changes to resources", testEtcdName),
			expectedCode:             http.StatusOK,
		},
		{
			name:                     "Etcd is currently being reconciled by druid, and request is from exempt service account",
			userName:                 exemptServiceAccounts[0],
			objectLabels:             map[string]string{druidv1alpha1.LabelManagedByKey: druidv1alpha1.LabelManagedByValue, druidv1alpha1.LabelPartOfKey: testEtcdName},
			etcdStatusLastOperation:  &druidv1alpha1.LastOperation{Type: druidv1alpha1.LastOperationTypeReconcile, State: druidv1alpha1.LastOperationStateProcessing},
			reconcilerServiceAccount: reconcilerServiceAccount,
			exemptServiceAccounts:    exemptServiceAccounts,
			expectedAllowed:          false,
			expectedMessage:          fmt.Sprintf("no external intervention allowed during ongoing reconciliation of Etcd %s by etcd-druid", testEtcdName),
			expectedReason:           "Forbidden",
			expectedCode:             http.StatusForbidden,
		},
		{
			name:                     "Etcd is not currently being reconciled by druid, and request is from exempt service account",
			userName:                 exemptServiceAccounts[0],
			objectLabels:             map[string]string{druidv1alpha1.LabelManagedByKey: druidv1alpha1.LabelManagedByValue, druidv1alpha1.LabelPartOfKey: testEtcdName},
			reconcilerServiceAccount: reconcilerServiceAccount,
			exemptServiceAccounts:    exemptServiceAccounts,
			expectedAllowed:          true,
			expectedMessage:          fmt.Sprintf("operations on Etcd %s by service account %s is exempt from Sentinel Webhook checks", testEtcdName, exemptServiceAccounts[0]),
			expectedCode:             http.StatusOK,
		},
		{
			name:                     "Etcd is not currently being reconciled or deleted by druid, and request is from non-exempt service account",
			userName:                 testUserName,
			objectLabels:             map[string]string{druidv1alpha1.LabelManagedByKey: druidv1alpha1.LabelManagedByValue, druidv1alpha1.LabelPartOfKey: testEtcdName},
			reconcilerServiceAccount: reconcilerServiceAccount,
			exemptServiceAccounts:    exemptServiceAccounts,
			expectedAllowed:          false,
			expectedMessage:          fmt.Sprintf("changes disallowed, since no ongoing processing of Etcd %s by etcd-druid", testEtcdName),
			expectedReason:           "Forbidden",
			expectedCode:             http.StatusForbidden,
		},
		{
			name:                     "Etcd is currently being deleted by druid, and request is from non-exempt service account",
			userName:                 testUserName,
			objectLabels:             map[string]string{druidv1alpha1.LabelManagedByKey: druidv1alpha1.LabelManagedByValue, druidv1alpha1.LabelPartOfKey: testEtcdName},
			etcdStatusLastOperation:  &druidv1alpha1.LastOperation{Type: druidv1alpha1.LastOperationTypeDelete, State: druidv1alpha1.LastOperationStateProcessing},
			reconcilerServiceAccount: reconcilerServiceAccount,
			expectedAllowed:          false,
			expectedReason:           "Forbidden",
			expectedMessage:          fmt.Sprintf("no external intervention allowed during ongoing deletion of Etcd %s by etcd-druid", testEtcdName),
			expectedCode:             http.StatusForbidden,
		},
		{
			name:                     "Etcd is currently being deleted by druid, and request is from druid",
			userName:                 reconcilerServiceAccount,
			objectLabels:             map[string]string{druidv1alpha1.LabelManagedByKey: druidv1alpha1.LabelManagedByValue, druidv1alpha1.LabelPartOfKey: testEtcdName},
			etcdStatusLastOperation:  &druidv1alpha1.LastOperation{Type: druidv1alpha1.LastOperationTypeDelete, State: druidv1alpha1.LastOperationStateProcessing},
			reconcilerServiceAccount: reconcilerServiceAccount,
			expectedAllowed:          true,
			expectedMessage:          fmt.Sprintf("deletion of resource by etcd-druid is allowed during deletion of Etcd %s", testEtcdName),
			expectedCode:             http.StatusOK,
		},
		{
			name:                     "Etcd is not currently being deleted by druid, and request is from exempt service account",
			userName:                 exemptServiceAccounts[0],
			objectLabels:             map[string]string{druidv1alpha1.LabelManagedByKey: druidv1alpha1.LabelManagedByValue, druidv1alpha1.LabelPartOfKey: testEtcdName},
			etcdStatusLastOperation:  &druidv1alpha1.LastOperation{Type: druidv1alpha1.LastOperationTypeDelete, State: druidv1alpha1.LastOperationStateProcessing},
			reconcilerServiceAccount: reconcilerServiceAccount,
			exemptServiceAccounts:    exemptServiceAccounts,
			expectedAllowed:          true,
			expectedMessage:          fmt.Sprintf("deletion of resource by exempt SA %s is allowed during deletion of Etcd %s", exemptServiceAccounts[0], testEtcdName),
			expectedCode:             http.StatusOK,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			etcd := testutils.EtcdBuilderWithDefaults(testEtcdName, testNamespace).
				WithAnnotations(tc.etcdAnnotations).
				WithLastOperation(tc.etcdStatusLastOperation).
				Build()

			cl := testutils.CreateTestFakeClientWithSchemeForObjects(kubernetes.Scheme, tc.etcdGetErr, nil, nil, nil, []client.Object{etcd}, client.ObjectKey{Name: testEtcdName, Namespace: testNamespace})
			decoder := admission.NewDecoder(cl.Scheme())

			handler := &Handler{
				Client: cl,
				config: &Config{
					Enabled:                  true,
					ReconcilerServiceAccount: reconcilerServiceAccount,
					ExemptServiceAccounts:    exemptServiceAccounts,
				},
				decoder: decoder,
				logger:  logr.Discard(),
			}

			obj := buildObjRawExtension(g, &appsv1.StatefulSet{}, tc.objectRaw, testObjectName, testNamespace, tc.objectLabels)

			response := handler.Handle(context.Background(), admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Delete,
					UserInfo:  authenticationv1.UserInfo{Username: tc.userName},
					Kind:      metav1.GroupVersionKind{Group: "apps", Version: "v1", Kind: "StatefulSet"},
					Name:      testObjectName,
					Namespace: testNamespace,
					OldObject: obj,
				},
			})

			g.Expect(response.Allowed).To(Equal(tc.expectedAllowed))
			g.Expect(string(response.Result.Reason)).To(Equal(tc.expectedReason))
			g.Expect(response.Result.Message).To(ContainSubstring(tc.expectedMessage))
			g.Expect(response.Result.Code).To(Equal(tc.expectedCode))
		})
	}
}

// ---------------- Helper functions -------------------

func buildObjRawExtension(g *WithT, emptyObj runtime.Object, objRaw []byte, testObjectName, testNs string, labels map[string]string) runtime.RawExtension {
	var (
		rawBytes []byte
		err      error
	)
	rawBytes = objRaw
	obj := buildObject(getObjectGVK(g, emptyObj), testObjectName, testNs, labels)
	if objRaw == nil {
		rawBytes, err = json.Marshal(obj)
		g.Expect(err).ToNot(HaveOccurred())
	}
	return runtime.RawExtension{
		Object: obj,
		Raw:    rawBytes,
	}
}

func getObjectGVK(g *WithT, obj runtime.Object) schema.GroupVersionKind {
	gvk, err := apiutil.GVKForObject(obj, kubernetes.Scheme)
	g.Expect(err).ToNot(HaveOccurred())
	return gvk
}

func buildObject(gvk schema.GroupVersionKind, name, namespace string, labels map[string]string) runtime.Object {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)
	obj.SetName(name)
	obj.SetNamespace(namespace)
	obj.SetLabels(labels)
	return obj
}
