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
	batchv1 "k8s.io/api/batch/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var (
	objectsForUpdate = []runtime.Object{
		&corev1.ServiceAccount{},
		&corev1.Service{},
		&corev1.ConfigMap{},
		&rbacv1.Role{},
		&rbacv1.RoleBinding{},
		&appsv1.StatefulSet{},
		&policyv1.PodDisruptionBudget{},
		&batchv1.Job{},
	}
	objectsForDelete = append(
		objectsForUpdate,
		&coordinationv1.Lease{},
	)
)

type testCase struct {
	name string
	// ----- handler configuration -----
	reconcilerServiceAccount string
	exemptServiceAccounts    []string
	// ----- request -----
	userName        string
	operation       admissionv1.Operation
	objectKind      *schema.GroupVersionKind
	objectName      string
	objectNamespace string
	objectLabels    map[string]string
	object          *runtime.RawExtension
	oldObject       *runtime.RawExtension
	// ----- etcd configuration -----
	etcdName                string
	etcdNamespace           string
	etcdAnnotations         map[string]string
	etcdStatusLastOperation *druidv1alpha1.LastOperation
	etcdGetErr              *apierrors.StatusError
	// ----- expected -----
	expectedAllowed bool
	expectedReason  string
	expectedMessage string
	expectedCode    int32
}

// ------------------------ Handle ------------------------
func TestHandle(t *testing.T) {
	g := NewWithT(t)
	t.Parallel()

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

	deploymentGVK, err := apiutil.GVKForObject(&appsv1.Deployment{}, kubernetes.Scheme)
	g.Expect(err).ToNot(HaveOccurred())

	commonTestCases := []testCase{
		{
			name:            "create operation",
			operation:       admissionv1.Create,
			expectedAllowed: true,
			expectedReason:  fmt.Sprintf("operation is not %s or %s", admissionv1.Update, admissionv1.Delete),
			expectedCode:    http.StatusOK,
		},
		{
			name:            "empty request object",
			object:          &runtime.RawExtension{Raw: []byte{}},
			oldObject:       &runtime.RawExtension{Raw: []byte{}},
			expectedAllowed: false,
			expectedMessage: "there is no content to decode",
			expectedCode:    http.StatusInternalServerError,
		},
		{
			name:            "malformed request object",
			object:          &runtime.RawExtension{Raw: []byte("foo")},
			oldObject:       &runtime.RawExtension{Raw: []byte("foo")},
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
			etcdName:        testEtcdName,
			etcdNamespace:   testNamespace,
			etcdGetErr:      apiNotFoundErr,
			expectedAllowed: true,
			expectedReason:  fmt.Sprintf("corresponding etcd %s not found", testEtcdName),
			expectedCode:    http.StatusOK,
		},
		{
			name:            "error in getting etcd",
			objectLabels:    map[string]string{druidv1alpha1.LabelPartOfKey: testEtcdName},
			etcdName:        testEtcdName,
			etcdNamespace:   testNamespace,
			etcdGetErr:      apiInternalErr,
			expectedAllowed: false,
			expectedMessage: internalErr.Error(),
			expectedCode:    http.StatusInternalServerError,
		},
		{
			name:            "etcd reconciliation suspended",
			objectLabels:    map[string]string{druidv1alpha1.LabelPartOfKey: testEtcdName},
			etcdName:        testEtcdName,
			etcdNamespace:   testNamespace,
			etcdAnnotations: map[string]string{druidv1alpha1.SuspendEtcdSpecReconcileAnnotation: "true"},
			expectedAllowed: true,
			expectedReason:  fmt.Sprintf("spec reconciliation of etcd %s is currently suspended", testEtcdName),
			expectedCode:    http.StatusOK,
		},
		{
			name:            "resource protection annotation set to false",
			objectLabels:    map[string]string{druidv1alpha1.LabelPartOfKey: testEtcdName},
			etcdName:        testEtcdName,
			etcdNamespace:   testNamespace,
			etcdAnnotations: map[string]string{druidv1alpha1.ResourceProtectionAnnotation: "false"},
			expectedAllowed: true,
			expectedReason:  fmt.Sprintf("changes allowed, since etcd %s has annotation %s: false", testEtcdName, druidv1alpha1.ResourceProtectionAnnotation),
			expectedCode:    http.StatusOK,
		},
		{
			name:                     "etcd is currently being reconciled by druid",
			objectLabels:             map[string]string{druidv1alpha1.LabelPartOfKey: testEtcdName},
			etcdName:                 testEtcdName,
			etcdNamespace:            testNamespace,
			etcdStatusLastOperation:  &druidv1alpha1.LastOperation{State: druidv1alpha1.LastOperationStateProcessing},
			reconcilerServiceAccount: reconcilerServiceAccount,
			userName:                 testUserName,
			expectedAllowed:          false,
			expectedReason:           fmt.Sprintf("no external intervention allowed during ongoing reconciliation of etcd %s by etcd-druid", testEtcdName),
			expectedCode:             http.StatusForbidden,
		},
		{
			name:                     "etcd is currently being reconciled by druid, but request is from druid",
			objectLabels:             map[string]string{druidv1alpha1.LabelPartOfKey: testEtcdName},
			etcdName:                 testEtcdName,
			etcdNamespace:            testNamespace,
			etcdStatusLastOperation:  &druidv1alpha1.LastOperation{State: druidv1alpha1.LastOperationStateProcessing},
			reconcilerServiceAccount: reconcilerServiceAccount,
			userName:                 reconcilerServiceAccount,
			expectedAllowed:          true,
			expectedReason:           fmt.Sprintf("ongoing reconciliation of etcd %s by etcd-druid requires changes to resources", testEtcdName),
			expectedCode:             http.StatusOK,
		},
		{
			name:                     "etcd is not currently being reconciled by druid, and request is from exempt service account",
			objectLabels:             map[string]string{druidv1alpha1.LabelPartOfKey: testEtcdName},
			etcdName:                 testEtcdName,
			etcdNamespace:            testNamespace,
			reconcilerServiceAccount: reconcilerServiceAccount,
			exemptServiceAccounts:    exemptServiceAccounts,
			userName:                 exemptServiceAccounts[0],
			expectedAllowed:          true,
			expectedReason:           fmt.Sprintf("operations on etcd %s by service account %s is exempt from Sentinel Webhook checks", testEtcdName, exemptServiceAccounts[0]),
			expectedCode:             http.StatusOK,
		},
		{
			name:                     "etcd is not currently being reconciled by druid, and request is from non-exempt service account",
			objectLabels:             map[string]string{druidv1alpha1.LabelPartOfKey: testEtcdName},
			etcdName:                 testEtcdName,
			etcdNamespace:            testNamespace,
			reconcilerServiceAccount: reconcilerServiceAccount,
			exemptServiceAccounts:    exemptServiceAccounts,
			userName:                 testUserName,
			expectedAllowed:          false,
			expectedReason:           fmt.Sprintf("changes disallowed, since no ongoing processing of etcd %s by etcd-druid", testEtcdName),
			expectedCode:             http.StatusForbidden,
		},
	}

	for _, operation := range []admissionv1.Operation{admissionv1.Update, admissionv1.Delete} {
		objects := objectsForUpdate
		if operation == admissionv1.Delete {
			objects = objectsForDelete
		}

		for _, object := range objects {
			for _, tc := range commonTestCases {
				var (
					obj, oldObj *unstructured.Unstructured
				)

				if tc.objectName == "" {
					tc.objectName = testObjectName
				}
				if tc.objectNamespace == "" {
					tc.objectNamespace = testNamespace
				}

				oldObj = &unstructured.Unstructured{}
				apiVersion, kind := object.GetObjectKind().GroupVersionKind().ToAPIVersionAndKind()
				oldObj.SetAPIVersion(apiVersion)
				oldObj.SetKind(kind)
				oldObj.SetLabels(tc.objectLabels)
				oldObj.SetName(tc.objectName)
				oldObj.SetNamespace(tc.objectNamespace)

				if operation == admissionv1.Update {
					obj = oldObj.DeepCopy()
				}

				if tc.object == nil {
					rawExt, err := getRawExtensionFromUnstructured(obj)
					g.Expect(err).ToNot(HaveOccurred())
					tc.object = &rawExt
				}

				if tc.oldObject == nil {
					rawExt, err := getRawExtensionFromUnstructured(oldObj)
					g.Expect(err).ToNot(HaveOccurred())
					tc.oldObject = &rawExt
				}

				if tc.objectKind == nil {
					gvk, err := apiutil.GVKForObject(object, kubernetes.Scheme)
					g.Expect(err).ToNot(HaveOccurred())
					tc.objectKind = &gvk
				}

				if tc.operation == "" {
					tc.operation = operation
				}

				gvk, err := apiutil.GVKForObject(object, kubernetes.Scheme)
				g.Expect(err).ToNot(HaveOccurred())

				t.Run(fmt.Sprintf("%s for %s operation on object GVK %s/%s/%s", tc.name, tc.operation, gvk.Group, gvk.Version, gvk.Kind), func(t *testing.T) {
					resp, err := runTestCase(tc)
					g.Expect(err).ToNot(HaveOccurred())
					assertTestResult(g, tc, resp)
				})
			}
		}
	}

	specialTestCases := []testCase{
		{
			name:            "lease update",
			operation:       admissionv1.Update,
			objectKind:      &schema.GroupVersionKind{Group: "coordination.k8s.io", Version: "v1", Kind: "Lease"},
			expectedAllowed: true,
			expectedReason:  "lease resource can be freely updated",
			expectedCode:    http.StatusOK,
		},
		{
			name:            "unknown resource type",
			operation:       admissionv1.Update,
			objectKind:      &schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
			expectedAllowed: true,
			expectedReason:  fmt.Sprintf("unexpected resource type: %s/%s", deploymentGVK.Group, deploymentGVK.Kind),
			expectedCode:    http.StatusOK,
		},
	}

	for _, tc := range specialTestCases {
		if tc.object == nil {
			rawExt, err := getRawExtensionFromUnstructured(&unstructured.Unstructured{})
			g.Expect(err).ToNot(HaveOccurred())
			tc.object = &rawExt
		}
		if tc.oldObject == nil {
			tc.oldObject = tc.object
		}
		t.Run(tc.name, func(t *testing.T) {
			resp, err := runTestCase(tc)
			g.Expect(err).ToNot(HaveOccurred())
			assertTestResult(g, tc, resp)
		})
	}
}

func runTestCase(tc testCase) (*admission.Response, error) {
	etcd := testutils.EtcdBuilderWithDefaults(tc.etcdName, tc.etcdNamespace).
		WithAnnotations(tc.etcdAnnotations).
		WithLastOperation(tc.etcdStatusLastOperation).
		Build()
	existingObjects := []client.Object{etcd}

	cl := testutils.CreateTestFakeClientWithSchemeForObjects(kubernetes.Scheme, tc.etcdGetErr, nil, nil, nil, existingObjects, client.ObjectKey{Name: tc.etcdName, Namespace: tc.etcdNamespace})

	config := &Config{
		Enabled:                  true,
		ReconcilerServiceAccount: tc.reconcilerServiceAccount,
		ExemptServiceAccounts:    tc.exemptServiceAccounts,
	}

	decoder, err := admission.NewDecoder(cl.Scheme())
	if err != nil {
		return nil, err
	}

	handler := &Handler{
		Client:  cl,
		config:  config,
		decoder: decoder,
		logger:  logr.Discard(),
	}

	resp := handler.Handle(context.Background(), admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Operation: tc.operation,
			Kind:      metav1.GroupVersionKind{Group: tc.objectKind.Group, Version: tc.objectKind.Version, Kind: tc.objectKind.Kind},
			Name:      tc.objectName,
			Namespace: tc.objectNamespace,
			UserInfo:  authenticationv1.UserInfo{Username: tc.userName},
			Object:    *tc.object,
			OldObject: *tc.oldObject,
		},
	})

	return &resp, nil
}

func getRawExtensionFromUnstructured(obj *unstructured.Unstructured) (runtime.RawExtension, error) {
	if obj == nil {
		return runtime.RawExtension{}, nil
	}

	ro := runtime.Object(obj)
	objJSON, err := json.Marshal(ro)
	if err != nil {
		return runtime.RawExtension{}, err
	}
	return runtime.RawExtension{
		Object: ro,
		Raw:    objJSON,
	}, nil
}

func assertTestResult(g *WithT, tc testCase, resp *admission.Response) {
	g.Expect(resp.Allowed).To(Equal(tc.expectedAllowed))
	g.Expect(string(resp.Result.Reason)).To(Equal(tc.expectedReason))
	g.Expect(resp.Result.Message).To(ContainSubstring(tc.expectedMessage))
	g.Expect(resp.Result.Code).To(Equal(tc.expectedCode))
}
