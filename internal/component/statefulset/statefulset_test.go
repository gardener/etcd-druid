// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package statefulset

import (
	"context"
	"testing"

	druidconfigv1alpha1 "github.com/gardener/etcd-druid/api/config/v1alpha1"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/common"
	"github.com/gardener/etcd-druid/internal/component"
	druiderr "github.com/gardener/etcd-druid/internal/errors"
	druidstore "github.com/gardener/etcd-druid/internal/store"
	"github.com/gardener/etcd-druid/internal/utils"
	testutils "github.com/gardener/etcd-druid/test/utils"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/gomega"
)

// ------------------------ GetExistingResourceNames ------------------------
func TestGetExistingResourceNames(t *testing.T) {
	etcd := testutils.EtcdBuilderWithDefaults(testutils.TestEtcdName, testutils.TestNamespace).Build()
	testCases := []struct {
		name             string
		stsExists        bool
		getErr           *apierrors.StatusError
		expectedStsNames []string
		expectedErr      *druiderr.DruidError
	}{
		{
			name:             "should return an empty slice if no sts is found",
			stsExists:        false,
			expectedStsNames: []string{},
		},
		{
			name:             "should return existing sts",
			stsExists:        true,
			expectedStsNames: []string{etcd.Name},
		},
		{
			name:      "should return err when client get fails",
			stsExists: true,
			getErr:    testutils.TestAPIInternalErr,
			expectedErr: &druiderr.DruidError{
				Code:      ErrGetStatefulSet,
				Cause:     testutils.TestAPIInternalErr,
				Operation: "GetExistingResourceNames",
			},
		},
	}

	g := NewWithT(t)
	t.Parallel()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var existingObjects []client.Object
			if tc.stsExists {
				existingObjects = append(existingObjects, emptyStatefulSet(etcd.ObjectMeta))
			}
			cl := testutils.CreateTestFakeClientForObjects(tc.getErr, nil, nil, nil, existingObjects, getObjectKey(etcd.ObjectMeta))
			operator := New(cl, nil)
			opCtx := component.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
			actualStsNames, err := operator.GetExistingResourceNames(opCtx, etcd.ObjectMeta)
			if tc.expectedErr != nil {
				testutils.CheckDruidError(g, tc.expectedErr, err)
			} else {
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(actualStsNames).To(Equal(tc.expectedStsNames))
			}
		})
	}
}

// ----------------------------------- Sync -----------------------------------
func TestSyncWhenNoSTSExists(t *testing.T) {
	testCases := []struct {
		name                   string
		replicas               int32
		emptyDir               bool
		annotations            map[string]string
		createErr              *apierrors.StatusError
		expectedErr            *druiderr.DruidError
		expectedReplicas       *int32
		expectNoServiceAccount bool
		expectNoService        bool
	}{
		{
			name:             "creates a single replica sts for a single node etcd cluster",
			replicas:         1,
			expectedReplicas: ptr.To[int32](1),
		},
		{
			name:             "creates a single replica sts for a single node etcd cluster with emptyDir volumes",
			replicas:         1,
			expectedReplicas: ptr.To[int32](1),
			emptyDir:         true,
		},
		{
			name:             "creates multiple replica sts for a multi-node etcd cluster",
			replicas:         3,
			expectedReplicas: ptr.To[int32](3),
			emptyDir:         false,
		},
		{
			name:             "creates multiple replica sts for a multi-node etcd cluster with emptyDir volumes",
			replicas:         3,
			expectedReplicas: ptr.To[int32](3),
			emptyDir:         true,
		},
		{
			name:             "returns error when client create fails",
			replicas:         3,
			expectedReplicas: ptr.To[int32](3),
			createErr:        testutils.TestAPIInternalErr,
			expectedErr: &druiderr.DruidError{
				Code:      ErrSyncStatefulSet,
				Cause:     testutils.TestAPIInternalErr,
				Operation: "Sync",
			},
		},
		{
			name:     "creates sts with 0 replicas, with no serviceAccount and service defined, with lease renewal and client-service-endpoint CLI flags disabled on backup-restore container when runtime component creation is disabled",
			replicas: 3,
			annotations: map[string]string{
				druidv1alpha1.DisableEtcdRuntimeComponentCreationAnnotation: "",
			},
			expectedReplicas:       ptr.To[int32](0),
			expectNoServiceAccount: true,
			expectNoService:        true,
		},
	}

	g := NewWithT(t)
	t.Parallel()
	iv := testutils.CreateImageVector(true, true)
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// *************** Build test environment ***************
			etcdBuilder := testutils.EtcdBuilderWithDefaults(testutils.TestEtcdName, testutils.TestNamespace).
				WithReplicas(tc.replicas).
				WithAnnotations(tc.annotations)
			if tc.emptyDir {
				err := druidconfigv1alpha1.DefaultFeatureGates.SetEnabledFeaturesFromMap(map[string]bool{
					druidconfigv1alpha1.AllowEmptyDir: true,
				})
				g.Expect(err).ShouldNot(HaveOccurred())
				etcdBuilder.WithEmptyDir()
			}
			etcd := etcdBuilder.Build()

			cl := testutils.CreateTestFakeClientForObjects(nil, tc.createErr, nil, nil, []client.Object{buildBackupSecret()}, getObjectKey(etcd.ObjectMeta))
			etcdImage, etcdBRImage, initContainerImage, err := utils.GetEtcdImages(etcd, iv)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(tc.expectedReplicas).ToNot(BeNil())
			stsMatcher := NewStatefulSetMatcher(g, cl, etcd, *tc.expectedReplicas, initContainerImage, etcdImage, etcdBRImage, ptr.To(druidstore.Local), tc.expectNoServiceAccount, tc.expectNoService)
			operator := New(cl, iv)
			// *************** Test and assert ***************
			opCtx := component.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
			opCtx.Data[common.CheckSumKeyConfigMap] = testutils.TestConfigMapCheckSum
			syncErr := operator.Sync(opCtx, etcd)
			latestSTS, getErr := getLatestStatefulSet(cl, etcd)
			if tc.expectedErr != nil {
				testutils.CheckDruidError(g, tc.expectedErr, syncErr)
				g.Expect(apierrors.IsNotFound(getErr)).To(BeTrue())
			} else {
				g.Expect(syncErr).ToNot(HaveOccurred())
				g.Expect(getErr).ToNot(HaveOccurred())
				g.Expect(latestSTS).ToNot(BeNil())
				g.Expect(*latestSTS).Should(stsMatcher.MatchStatefulSet())
			}
		})
	}
}

// ----------------------------- TriggerDelete -------------------------------
// ---------------------------- Helper Functions -----------------------------

func getLatestStatefulSet(cl client.Client, etcd *druidv1alpha1.Etcd) (*appsv1.StatefulSet, error) {
	sts := &appsv1.StatefulSet{}
	err := cl.Get(context.Background(), client.ObjectKeyFromObject(etcd), sts)
	return sts, err
}

func buildBackupSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "etcd-backup",
			Namespace: testutils.TestNamespace,
		},
		Data: map[string][]byte{
			"bucketName": []byte("NDQ5YjEwZj"),
			"hostPath":   []byte("/var/data/etcd-backup"),
		},
	}
}
