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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

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

func TestHandleCreate(t *testing.T) {
	g := NewWithT(t)

	testCases := []struct {
		name            string
		expectedAllowed bool
		expectedReason  string
	}{
		{
			name:            "create any resource",
			expectedAllowed: true,
			expectedReason:  "operation CREATE is allowed",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cl := fake.NewClientBuilder().Build()
			decoder, err := admission.NewDecoder(cl.Scheme())
			if err != nil {
				g.Expect(err).ToNot(HaveOccurred())
			}

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
					Operation: admissionv1.Create,
				},
			})

			g.Expect(resp.Allowed).To(Equal(tc.expectedAllowed))
			g.Expect(string(resp.Result.Reason)).To(Equal(tc.expectedReason))
		})
	}
}

func TestHandleLeaseUpdate(t *testing.T) {
	g := NewWithT(t)

	testCases := []struct {
		name            string
		gvk             metav1.GroupVersionKind
		expectedAllowed bool
		expectedReason  string
	}{
		{
			name:            "update Lease",
			expectedAllowed: true,
			expectedReason:  "lease resource can be freely updated",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cl := fake.NewClientBuilder().Build()
			decoder, err := admission.NewDecoder(cl.Scheme())
			g.Expect(err).ToNot(HaveOccurred())

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
					Kind:      metav1.GroupVersionKind{Group: "coordination.k8s.io", Version: "v1", Kind: "Lease"},
				},
			})

			g.Expect(resp.Allowed).To(Equal(tc.expectedAllowed))
			g.Expect(string(resp.Result.Reason)).To(Equal(tc.expectedReason))
		})
	}
}

func TestHandleUpdate(t *testing.T) {
	g := NewWithT(t)

	var (
		reconcilerServiceAccount = "etcd-druid-sa"
		exemptServiceAccounts    = []string{"exempt-sa-1"}
		testUserName             = "test-user"
		testObjectName           = "test"
		testNamespace            = "test-ns"
		testEtcdName             = "test"

		internalErr    = errors.New("test internal error")
		apiInternalErr = apierrors.NewInternalError(internalErr)
		apiNotFoundErr = apierrors.NewNotFound(schema.GroupResource{}, "")
	)

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
			name:            "empty request object",
			objectRaw:       []byte{},
			expectedAllowed: false,
			expectedMessage: "there is no content to decode",
			expectedCode:    http.StatusInternalServerError,
		},
		{
			name:            "malformed request object",
			objectRaw:       []byte("foo"),
			expectedAllowed: false,
			expectedMessage: "invalid character",
			expectedCode:    http.StatusInternalServerError,
		},
		{
			name:            "resource has no part-of label",
			objectLabels:    map[string]string{},
			expectedAllowed: true,
			expectedReason:  fmt.Sprintf("label %s not found on resource", druidv1alpha1.LabelPartOfKey),
			expectedCode:    http.StatusOK,
		},
		{
			name:            "etcd not found",
			objectLabels:    map[string]string{druidv1alpha1.LabelPartOfKey: testEtcdName},
			etcdGetErr:      apiNotFoundErr,
			expectedAllowed: true,
			expectedReason:  fmt.Sprintf("corresponding etcd %s not found", testEtcdName),
			expectedCode:    http.StatusOK,
		},
		{
			name:            "error in getting etcd",
			objectLabels:    map[string]string{druidv1alpha1.LabelPartOfKey: testEtcdName},
			etcdGetErr:      apiInternalErr,
			expectedAllowed: false,
			expectedMessage: internalErr.Error(),
			expectedCode:    http.StatusInternalServerError,
		},
		{
			name:            "etcd reconciliation suspended",
			objectLabels:    map[string]string{druidv1alpha1.LabelPartOfKey: testEtcdName},
			etcdAnnotations: map[string]string{druidv1alpha1.SuspendEtcdSpecReconcileAnnotation: "true"},
			expectedAllowed: true,
			expectedReason:  fmt.Sprintf("spec reconciliation of etcd %s is currently suspended", testEtcdName),
			expectedCode:    http.StatusOK,
		},
		{
			name:            "resource protection annotation set to false",
			objectLabels:    map[string]string{druidv1alpha1.LabelPartOfKey: testEtcdName},
			etcdAnnotations: map[string]string{druidv1alpha1.ResourceProtectionAnnotation: "false"},
			expectedAllowed: true,
			expectedReason:  fmt.Sprintf("changes allowed, since etcd %s has annotation %s: false", testEtcdName, druidv1alpha1.ResourceProtectionAnnotation),
			expectedCode:    http.StatusOK,
		},
		{
			name:                     "etcd is currently being reconciled by druid",
			userName:                 testUserName,
			objectLabels:             map[string]string{druidv1alpha1.LabelPartOfKey: testEtcdName},
			etcdStatusLastOperation:  &druidv1alpha1.LastOperation{State: druidv1alpha1.LastOperationStateProcessing},
			reconcilerServiceAccount: reconcilerServiceAccount,
			expectedAllowed:          false,
			expectedReason:           fmt.Sprintf("no external intervention allowed during ongoing reconciliation of etcd %s by etcd-druid", testEtcdName),
			expectedCode:             http.StatusForbidden,
		},
		{
			name:                     "etcd is currently being reconciled by druid, but request is from druid",
			userName:                 reconcilerServiceAccount,
			objectLabels:             map[string]string{druidv1alpha1.LabelPartOfKey: testEtcdName},
			etcdStatusLastOperation:  &druidv1alpha1.LastOperation{State: druidv1alpha1.LastOperationStateProcessing},
			reconcilerServiceAccount: reconcilerServiceAccount,
			expectedAllowed:          true,
			expectedReason:           fmt.Sprintf("ongoing reconciliation of etcd %s by etcd-druid requires changes to resources", testEtcdName),
			expectedCode:             http.StatusOK,
		},
		{
			name:                     "etcd is not currently being reconciled by druid, and request is from exempt service account",
			userName:                 exemptServiceAccounts[0],
			objectLabels:             map[string]string{druidv1alpha1.LabelPartOfKey: testEtcdName},
			reconcilerServiceAccount: reconcilerServiceAccount,
			exemptServiceAccounts:    exemptServiceAccounts,
			expectedAllowed:          true,
			expectedReason:           fmt.Sprintf("operations on etcd %s by service account %s is exempt from Sentinel Webhook checks", testEtcdName, exemptServiceAccounts[0]),
			expectedCode:             http.StatusOK,
		},
		{
			name:                     "etcd is not currently being reconciled by druid, and request is from non-exempt service account",
			userName:                 testUserName,
			objectLabels:             map[string]string{druidv1alpha1.LabelPartOfKey: testEtcdName},
			reconcilerServiceAccount: reconcilerServiceAccount,
			exemptServiceAccounts:    exemptServiceAccounts,
			expectedAllowed:          false,
			expectedReason:           fmt.Sprintf("changes disallowed, since no ongoing processing of etcd %s by etcd-druid", testEtcdName),
			expectedCode:             http.StatusForbidden,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			etcd := testutils.EtcdBuilderWithDefaults(testEtcdName, testNamespace).
				WithAnnotations(tc.etcdAnnotations).
				WithLastOperation(tc.etcdStatusLastOperation).
				Build()

			cl := testutils.CreateTestFakeClientWithSchemeForObjects(kubernetes.Scheme, tc.etcdGetErr, nil, nil, nil, []client.Object{etcd}, client.ObjectKey{Name: testEtcdName, Namespace: testNamespace})
			decoder, err := admission.NewDecoder(cl.Scheme())
			g.Expect(err).ToNot(HaveOccurred())

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

			sts := &appsv1.StatefulSet{}
			obj := runtime.RawExtension{
				Object: buildObject(getObjectGVK(g, sts), testObjectName, testNamespace, tc.objectLabels),
				Raw:    tc.objectRaw,
			}
			if tc.objectRaw == nil {
				objRaw, err := json.Marshal(obj.Object)
				g.Expect(err).ToNot(HaveOccurred())
				obj.Raw = objRaw
			}

			response := handler.Handle(context.Background(), admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Operation: admissionv1.Update,
					UserInfo:  authenticationv1.UserInfo{Username: tc.userName},
					Kind:      metav1.GroupVersionKind{Group: "apps", Version: "v1", Kind: "StatefulSet"},
					Name:      testObjectName,
					Namespace: testNamespace,
					Object:    obj,
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

func TestHandleDelete(t *testing.T) {
	g := NewWithT(t)

	var (
		reconcilerServiceAccount = "etcd-druid-sa"
		exemptServiceAccounts    = []string{"exempt-sa-1"}
		testUserName             = "test-user"
		testObjectName           = "test"
		testNamespace            = "test-ns"
		testEtcdName             = "test"

		internalErr    = errors.New("test internal error")
		apiInternalErr = apierrors.NewInternalError(internalErr)
		apiNotFoundErr = apierrors.NewNotFound(schema.GroupResource{}, "")
	)

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
			name:            "empty request object",
			objectRaw:       []byte{},
			expectedAllowed: false,
			expectedMessage: "there is no content to decode",
			expectedCode:    http.StatusInternalServerError,
		},
		{
			name:            "malformed request object",
			objectRaw:       []byte("foo"),
			expectedAllowed: false,
			expectedMessage: "invalid character",
			expectedCode:    http.StatusInternalServerError,
		},
		{
			name:            "resource has no part-of label",
			objectLabels:    map[string]string{},
			expectedAllowed: true,
			expectedReason:  fmt.Sprintf("label %s not found on resource", druidv1alpha1.LabelPartOfKey),
			expectedCode:    http.StatusOK,
		},
		{
			name:            "etcd not found",
			objectLabels:    map[string]string{druidv1alpha1.LabelPartOfKey: testEtcdName},
			etcdGetErr:      apiNotFoundErr,
			expectedAllowed: true,
			expectedReason:  fmt.Sprintf("corresponding etcd %s not found", testEtcdName),
			expectedCode:    http.StatusOK,
		},
		{
			name:            "error in getting etcd",
			objectLabels:    map[string]string{druidv1alpha1.LabelPartOfKey: testEtcdName},
			etcdGetErr:      apiInternalErr,
			expectedAllowed: false,
			expectedMessage: internalErr.Error(),
			expectedCode:    http.StatusInternalServerError,
		},
		{
			name:            "etcd reconciliation suspended",
			objectLabels:    map[string]string{druidv1alpha1.LabelPartOfKey: testEtcdName},
			etcdAnnotations: map[string]string{druidv1alpha1.SuspendEtcdSpecReconcileAnnotation: "true"},
			expectedAllowed: true,
			expectedReason:  fmt.Sprintf("spec reconciliation of etcd %s is currently suspended", testEtcdName),
			expectedCode:    http.StatusOK,
		},
		{
			name:            "resource protection annotation set to false",
			objectLabels:    map[string]string{druidv1alpha1.LabelPartOfKey: testEtcdName},
			etcdAnnotations: map[string]string{druidv1alpha1.ResourceProtectionAnnotation: "false"},
			expectedAllowed: true,
			expectedReason:  fmt.Sprintf("changes allowed, since etcd %s has annotation %s: false", testEtcdName, druidv1alpha1.ResourceProtectionAnnotation),
			expectedCode:    http.StatusOK,
		},
		{
			name:                     "etcd is currently being reconciled by druid",
			userName:                 testUserName,
			objectLabels:             map[string]string{druidv1alpha1.LabelPartOfKey: testEtcdName},
			etcdStatusLastOperation:  &druidv1alpha1.LastOperation{State: druidv1alpha1.LastOperationStateProcessing},
			reconcilerServiceAccount: reconcilerServiceAccount,
			expectedAllowed:          false,
			expectedReason:           fmt.Sprintf("no external intervention allowed during ongoing reconciliation of etcd %s by etcd-druid", testEtcdName),
			expectedCode:             http.StatusForbidden,
		},
		{
			name:                     "etcd is currently being reconciled by druid, but request is from druid",
			userName:                 reconcilerServiceAccount,
			objectLabels:             map[string]string{druidv1alpha1.LabelPartOfKey: testEtcdName},
			etcdStatusLastOperation:  &druidv1alpha1.LastOperation{State: druidv1alpha1.LastOperationStateProcessing},
			reconcilerServiceAccount: reconcilerServiceAccount,
			expectedAllowed:          true,
			expectedReason:           fmt.Sprintf("ongoing reconciliation of etcd %s by etcd-druid requires changes to resources", testEtcdName),
			expectedCode:             http.StatusOK,
		},
		{
			name:                     "etcd is not currently being reconciled by druid, and request is from exempt service account",
			userName:                 exemptServiceAccounts[0],
			objectLabels:             map[string]string{druidv1alpha1.LabelPartOfKey: testEtcdName},
			reconcilerServiceAccount: reconcilerServiceAccount,
			exemptServiceAccounts:    exemptServiceAccounts,
			expectedAllowed:          true,
			expectedReason:           fmt.Sprintf("operations on etcd %s by service account %s is exempt from Sentinel Webhook checks", testEtcdName, exemptServiceAccounts[0]),
			expectedCode:             http.StatusOK,
		},
		{
			name:                     "etcd is not currently being reconciled by druid, and request is from non-exempt service account",
			userName:                 testUserName,
			objectLabels:             map[string]string{druidv1alpha1.LabelPartOfKey: testEtcdName},
			reconcilerServiceAccount: reconcilerServiceAccount,
			exemptServiceAccounts:    exemptServiceAccounts,
			expectedAllowed:          false,
			expectedReason:           fmt.Sprintf("changes disallowed, since no ongoing processing of etcd %s by etcd-druid", testEtcdName),
			expectedCode:             http.StatusForbidden,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			etcd := testutils.EtcdBuilderWithDefaults(testEtcdName, testNamespace).
				WithAnnotations(tc.etcdAnnotations).
				WithLastOperation(tc.etcdStatusLastOperation).
				Build()

			cl := testutils.CreateTestFakeClientWithSchemeForObjects(kubernetes.Scheme, tc.etcdGetErr, nil, nil, nil, []client.Object{etcd}, client.ObjectKey{Name: testEtcdName, Namespace: testNamespace})
			decoder, err := admission.NewDecoder(cl.Scheme())
			g.Expect(err).ToNot(HaveOccurred())

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

			sts := &appsv1.StatefulSet{}
			obj := runtime.RawExtension{
				Object: buildObject(getObjectGVK(g, sts), testObjectName, testNamespace, tc.objectLabels),
				Raw:    tc.objectRaw,
			}
			if tc.objectRaw == nil {
				objRaw, err := json.Marshal(obj.Object)
				g.Expect(err).ToNot(HaveOccurred())
				obj.Raw = objRaw
			}

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
