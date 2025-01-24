package utils

import (
	"context"
	testutils "github.com/gardener/etcd-druid/test/utils"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"
)

const (
	resourceVersion   = "42"
	testConfigMapName = "test-cm"
)

func TestAddFinalizers(t *testing.T) {
	testCases := []struct {
		name               string
		resourceExists     bool
		newFinalizers      []string
		existingFinalizers []string
		patchErr           *apierrors.StatusError
		expectedFinalizers []string
		expectedErr        *apierrors.StatusError
	}{
		{
			name:           "resource does not exist, add finalizers should return an error",
			resourceExists: false,
			newFinalizers:  []string{"f1", "f2"},
			expectedErr:    apierrors.NewNotFound(corev1.Resource("configmaps"), testConfigMapName),
		},
		{
			name:               "resource exists, add finalizers should add finalizers",
			resourceExists:     true,
			newFinalizers:      []string{"f2", "f3"},
			existingFinalizers: []string{"f1"},
			expectedFinalizers: []string{"f1", "f2", "f3"},
		},
		{
			name:               "resource exists, no finalizer passed",
			resourceExists:     true,
			existingFinalizers: []string{"f1"},
			newFinalizers:      nil,
			expectedFinalizers: []string{"f1"},
		},
		{
			name:           "resource exists, patch fails",
			resourceExists: true,
			newFinalizers:  []string{"f2", "f3"},
			patchErr:       testutils.TestAPIInternalErr,
			expectedErr:    testutils.TestAPIInternalErr,
		},
	}

	g := NewWithT(t)

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var (
				cm  *corev1.ConfigMap
				cl  client.Client
				ctx = context.Background()
			)
			cm = createConfigMap(testConfigMapName, testutils.TestNamespace, tc.existingFinalizers)
			if tc.resourceExists {
				cl = testutils.CreateTestFakeClientForObjects(nil, nil, tc.patchErr, nil, []client.Object{cm}, client.ObjectKeyFromObject(cm))
			} else {
				cm.ResourceVersion = resourceVersion
				cl = testutils.CreateDefaultFakeClient()
			}
			err := AddFinalizers(ctx, cl, cm, tc.newFinalizers...)
			if tc.expectedErr != nil {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err).To(Equal(tc.expectedErr))
			} else {
				g.Expect(err).To(BeNil())
				latestCm := &corev1.ConfigMap{}
				g.Expect(cl.Get(ctx, client.ObjectKeyFromObject(cm), latestCm)).To(Succeed())
				g.Expect(latestCm.Finalizers).To(ConsistOf(tc.expectedFinalizers))
			}
		})
	}
}

func TestRemoveFinalizers(t *testing.T) {
	testCases := []struct {
		name               string
		resourceExists     bool
		existingFinalizers []string
		finalizersToRemove []string
		patchErr           *apierrors.StatusError
		expectedFinalizers []string
		expectedErr        *apierrors.StatusError
	}{
		{
			name:               "resource does not exist, remove finalizers should not return an error",
			resourceExists:     false,
			finalizersToRemove: []string{"f1"},
			expectedFinalizers: []string{"f1"},
		},
		{
			name:               "resource exists, remove finalizers should remove finalizers",
			resourceExists:     true,
			existingFinalizers: []string{"f1", "f2", "f3"},
			finalizersToRemove: []string{"f1", "f3"},
			expectedFinalizers: []string{"f2"},
		},
		{
			name:               "resource exists, finalizers to remove consists of non-existing finalizers",
			resourceExists:     true,
			existingFinalizers: []string{"f1", "f2"},
			finalizersToRemove: []string{"f3", "f4"},
			expectedFinalizers: []string{"f1", "f2"},
		},
		{
			name:               "resource exists, no finalizer passed",
			resourceExists:     true,
			existingFinalizers: []string{"f1", "f2"},
			finalizersToRemove: nil,
			expectedFinalizers: []string{"f1", "f2"},
		},
		{
			name:               "resource exists, patch fails",
			resourceExists:     true,
			existingFinalizers: []string{"f1", "f2"},
			finalizersToRemove: []string{"f1"},
			patchErr:           testutils.TestAPIInternalErr,
			expectedErr:        testutils.TestAPIInternalErr,
		},
	}

	g := NewWithT(t)
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var (
				cm  *corev1.ConfigMap
				cl  client.Client
				ctx = context.Background()
			)
			cm = createConfigMap(testConfigMapName, testutils.TestNamespace, tc.existingFinalizers)
			if tc.resourceExists {
				cl = testutils.CreateTestFakeClientForObjects(nil, nil, tc.patchErr, nil, []client.Object{cm}, client.ObjectKeyFromObject(cm))
			} else {
				cm.ResourceVersion = resourceVersion
				cl = testutils.CreateDefaultFakeClient()
			}
			err := RemoveFinalizers(ctx, cl, cm, tc.finalizersToRemove...)
			if tc.expectedErr != nil {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err).To(Equal(tc.expectedErr))
			} else {
				g.Expect(err).To(BeNil())
				if tc.resourceExists {
					latestCm := &corev1.ConfigMap{}
					g.Expect(cl.Get(ctx, client.ObjectKeyFromObject(cm), latestCm)).To(Succeed())
					g.Expect(latestCm.Finalizers).To(ConsistOf(tc.expectedFinalizers))
				}
			}
		})
	}
}

func TestRemoveAllFinalizers(t *testing.T) {
	testCases := []struct {
		name               string
		resourceExists     bool
		existingFinalizers []string
		patchErr           *apierrors.StatusError
		expectedErr        *apierrors.StatusError
	}{
		{
			name:           "resource does not exist, remove all finalizers should not return an error",
			resourceExists: false,
		},
		{
			name:               "resource exists and has finalizers, remove all finalizers should remove finalizers",
			resourceExists:     true,
			existingFinalizers: []string{"f1", "f2", "f3"},
		},
		{
			name:           "resource exists and has no finalizers, remove all finalizers should not return an error",
			resourceExists: true,
		},
		{
			name:               "resource exists, patch fails",
			resourceExists:     true,
			existingFinalizers: []string{"f1", "f2"},
			patchErr:           testutils.TestAPIInternalErr,
			expectedErr:        testutils.TestAPIInternalErr,
		},
	}

	g := NewWithT(t)
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var (
				cm  *corev1.ConfigMap
				cl  client.Client
				ctx = context.Background()
			)
			cm = createConfigMap(testConfigMapName, testutils.TestNamespace, tc.existingFinalizers)
			if tc.resourceExists {
				cl = testutils.CreateTestFakeClientForObjects(nil, nil, tc.patchErr, nil, []client.Object{cm}, client.ObjectKeyFromObject(cm))
			} else {
				cm.ResourceVersion = resourceVersion
				cl = testutils.CreateDefaultFakeClient()
			}
			err := RemoveAllFinalizers(ctx, cl, cm)
			if tc.expectedErr != nil {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err).To(Equal(tc.expectedErr))
			} else {
				g.Expect(err).To(BeNil())
				if tc.resourceExists {
					latestCm := &corev1.ConfigMap{}
					g.Expect(cl.Get(ctx, client.ObjectKeyFromObject(cm), latestCm)).To(Succeed())
					g.Expect(latestCm.Finalizers).To(BeNil())
				}
			}
		})
	}
}

func createConfigMap(name string, namespace string, finalizers []string) *corev1.ConfigMap {
	objMeta := metav1.ObjectMeta{
		Name:      name,
		Namespace: namespace,
	}
	if len(finalizers) > 0 {
		objMeta.Finalizers = finalizers
	}
	return &corev1.ConfigMap{ObjectMeta: objMeta}
}
