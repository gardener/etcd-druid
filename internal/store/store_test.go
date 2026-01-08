// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package store_test

import (
	"context"
	"errors"
	"testing"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/store"
	"github.com/gardener/etcd-druid/internal/utils"
	testutils "github.com/gardener/etcd-druid/test/utils"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/gomega"
)

func TestGetHostMountPathFromSecretRef(t *testing.T) {
	const (
		hostPath              = "/var/data/etcd-backup"
		existingSecretName    = "test-backup-secret"
		nonExistingSecretName = "non-existing-backup-secret"
	)
	internalErr := errors.New("test internal error")
	apiInternalErr := apierrors.NewInternalError(internalErr)
	logger := logr.Discard()

	testCases := []struct {
		name                  string
		secretRefDefined      bool
		secretExists          bool
		hostPathSetInSecret   bool
		hostPathInSecret      *string
		expectedHostMountPath string
		getErr                *apierrors.StatusError
		expectedErr           *apierrors.StatusError
	}{
		{
			name:                  "no secret ref configured, should return default mount path",
			secretRefDefined:      false,
			expectedHostMountPath: store.LocalProviderDefaultMountPath,
		},
		{
			name:             "secret ref points to an unknown secret, should return an error",
			secretRefDefined: true,
			secretExists:     false,
			expectedErr:      apierrors.NewNotFound(corev1.Resource("secrets"), nonExistingSecretName),
		},
		{
			name:                  "secret ref points to a secret whose data does not have path set, should return default mount path",
			secretRefDefined:      true,
			secretExists:          true,
			hostPathSetInSecret:   false,
			expectedHostMountPath: store.LocalProviderDefaultMountPath,
		},
		{
			name:                  "secret ref points to a secret whose data has a path, should return the path defined in secret.Data",
			secretRefDefined:      true,
			secretExists:          true,
			hostPathSetInSecret:   true,
			hostPathInSecret:      ptr.To(hostPath),
			expectedHostMountPath: hostPath,
		},
		{
			name:             "secret exists but get fails, should return an error",
			secretRefDefined: true,
			secretExists:     true,
			getErr:           apiInternalErr,
			expectedErr:      apiInternalErr,
		},
	}

	g := NewWithT(t)
	t.Parallel()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var existingObjects []client.Object
			if tc.secretExists {
				sec := createSecret(existingSecretName, testutils.TestNamespace, tc.hostPathInSecret)
				existingObjects = append(existingObjects, sec)
			}
			cl := testutils.CreateTestFakeClientForObjects(tc.getErr, nil, nil, nil, existingObjects, client.ObjectKey{Name: existingSecretName, Namespace: testutils.TestNamespace})
			secretName := utils.IfConditionOr(tc.secretExists, existingSecretName, nonExistingSecretName)
			storeSpec := createStoreSpec(tc.secretRefDefined, secretName, testutils.TestNamespace)
			actualHostPath, err := store.GetHostMountPathFromSecretRef(context.Background(), cl, logger, storeSpec, testutils.TestNamespace)
			if tc.expectedErr != nil {
				g.Expect(err).ToNot(BeNil())
				var actualErr *apierrors.StatusError
				g.Expect(errors.As(err, &actualErr)).To(BeTrue())
				g.Expect(actualErr.Status().Code).To(Equal(tc.expectedErr.Status().Code))
			} else {
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(actualHostPath).To(Equal(tc.expectedHostMountPath))
			}
		})
	}
}

func createStoreSpec(secretRefDefined bool, secretName, secretNamespace string) *druidv1alpha1.StoreSpec {
	var secretRef *corev1.SecretReference
	if secretRefDefined {
		secretRef = &corev1.SecretReference{
			Name:      secretName,
			Namespace: secretNamespace,
		}
	}
	return &druidv1alpha1.StoreSpec{
		SecretRef: secretRef,
	}
}

func createSecret(name, namespace string, hostPath *string) *corev1.Secret {
	data := map[string][]byte{
		"bucketName": []byte("NDQ5YjEwZj"),
	}
	if hostPath != nil {
		data["hostPath"] = []byte(*hostPath)
	}
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: data,
	}
}
