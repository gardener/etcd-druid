// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package statefulset

import (
	"context"
	"fmt"
	"testing"

	druidapicommon "github.com/gardener/etcd-druid/api/common"
	druidconfigv1alpha1 "github.com/gardener/etcd-druid/api/config/v1alpha1"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/client/kubernetes"
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

// ----------------------------------- PreSync -----------------------------------
func TestPreSync(t *testing.T) {
	const (
		oldImage     = "europe-docker.pkg.dev/gardener-project/public/gardener/etcd-wrapper:v0.6.2"
		currentImage = ""
	)

	testCases := []struct {
		name               string
		backupEnabled      bool
		stsExists          bool
		featureGateEnabled bool
		stsReplicas        int32
		etcdReplicas       int32
		etcdWrapperImage   string
		existingTasks      []*druidv1alpha1.EtcdOpsTask
		expectedErrCode    *druidapicommon.ErrorCode
	}{
		{
			name:          "returns nil when backup is disabled",
			backupEnabled: false,
			stsExists:     true,
			stsReplicas:   3,
			etcdReplicas:  3,
		},
		{
			name:          "returns nil when no STS exists",
			backupEnabled: true,
			stsExists:     false,
			etcdReplicas:  3,
		},
		{
			name:               "returns nil when no hibernation and no upgrade",
			backupEnabled:      true,
			stsExists:          true,
			featureGateEnabled: true,
			stsReplicas:        3,
			etcdReplicas:       3,
			etcdWrapperImage:   currentImage,
		},
		{
			name:               "hibernation succeeds when task completed",
			backupEnabled:      true,
			stsExists:          true,
			featureGateEnabled: false,
			stsReplicas:        3,
			etcdReplicas:       0,
			etcdWrapperImage:   currentImage,
			existingTasks:      []*druidv1alpha1.EtcdOpsTask{buildPreSyncTask(preSyncTaskPrefixHibernation, 0, ptr.To(druidv1alpha1.TaskStateSucceeded))},
		},
		{
			name:               "hibernation proceeds after max retries exceeded",
			backupEnabled:      true,
			stsExists:          true,
			featureGateEnabled: false,
			stsReplicas:        3,
			etcdReplicas:       0,
			etcdWrapperImage:   currentImage,
			existingTasks:      []*druidv1alpha1.EtcdOpsTask{buildPreSyncTask(preSyncTaskPrefixHibernation, maxPreSyncRetries-1, ptr.To(druidv1alpha1.TaskStateFailed))},
		},
		{
			name:               "hibernation with upgrade succeeds when task completed",
			backupEnabled:      true,
			stsExists:          true,
			featureGateEnabled: true,
			stsReplicas:        3,
			etcdReplicas:       0,
			etcdWrapperImage:   oldImage,
			existingTasks:      []*druidv1alpha1.EtcdOpsTask{buildPreSyncTask(preSyncTaskPrefixHibernation, 0, ptr.To(druidv1alpha1.TaskStateSucceeded))},
		},
		{
			name:               "hibernation with upgrade proceeds after max retries exceeded",
			backupEnabled:      true,
			stsExists:          true,
			featureGateEnabled: true,
			stsReplicas:        3,
			etcdReplicas:       0,
			etcdWrapperImage:   oldImage,
			existingTasks:      []*druidv1alpha1.EtcdOpsTask{buildPreSyncTask(preSyncTaskPrefixHibernation, maxPreSyncRetries-1, ptr.To(druidv1alpha1.TaskStateFailed))},
		},
		{
			name:               "upgrade succeeds when task completed",
			backupEnabled:      true,
			stsExists:          true,
			featureGateEnabled: true,
			stsReplicas:        3,
			etcdReplicas:       3,
			etcdWrapperImage:   oldImage,
			existingTasks:      []*druidv1alpha1.EtcdOpsTask{buildPreSyncTask(preSyncTaskPrefixUpgrade, 0, ptr.To(druidv1alpha1.TaskStateSucceeded))},
		},
		{
			name:               "upgrade proceeds after max retries exceeded",
			backupEnabled:      true,
			stsExists:          true,
			featureGateEnabled: true,
			stsReplicas:        3,
			etcdReplicas:       3,
			etcdWrapperImage:   oldImage,
			existingTasks:      []*druidv1alpha1.EtcdOpsTask{buildPreSyncTask(preSyncTaskPrefixUpgrade, maxPreSyncRetries-1, ptr.To(druidv1alpha1.TaskStateFailed))},
		},
		{
			name:               "hibernation requeues when no task exists",
			backupEnabled:      true,
			stsExists:          true,
			featureGateEnabled: false,
			stsReplicas:        3,
			etcdReplicas:       0,
			etcdWrapperImage:   currentImage,
			expectedErrCode:    ptr.To(druidapicommon.ErrorCode(druiderr.ErrRequeueAfter)),
		},
		{
			name:               "hibernation requeues when task is in progress",
			backupEnabled:      true,
			stsExists:          true,
			featureGateEnabled: false,
			stsReplicas:        3,
			etcdReplicas:       0,
			etcdWrapperImage:   currentImage,
			existingTasks:      []*druidv1alpha1.EtcdOpsTask{buildPreSyncTask(preSyncTaskPrefixHibernation, 0, ptr.To(druidv1alpha1.TaskStateInProgress))},
			expectedErrCode:    ptr.To(druidapicommon.ErrorCode(druiderr.ErrRequeueAfter)),
		},
		{
			name:               "hibernation requeues when task failed and retries remain",
			backupEnabled:      true,
			stsExists:          true,
			featureGateEnabled: false,
			stsReplicas:        3,
			etcdReplicas:       0,
			etcdWrapperImage:   currentImage,
			existingTasks:      []*druidv1alpha1.EtcdOpsTask{buildPreSyncTask(preSyncTaskPrefixHibernation, 1, ptr.To(druidv1alpha1.TaskStateFailed))},
			expectedErrCode:    ptr.To(druidapicommon.ErrorCode(druiderr.ErrRequeueAfter)),
		},
		{
			name:               "upgrade requeues when no task exists",
			backupEnabled:      true,
			stsExists:          true,
			featureGateEnabled: true,
			stsReplicas:        3,
			etcdReplicas:       3,
			etcdWrapperImage:   oldImage,
			expectedErrCode:    ptr.To(druidapicommon.ErrorCode(druiderr.ErrRequeueAfter)),
		},
		{
			name:               "upgrade requeues when task is in progress",
			backupEnabled:      true,
			stsExists:          true,
			featureGateEnabled: true,
			stsReplicas:        3,
			etcdReplicas:       3,
			etcdWrapperImage:   oldImage,
			existingTasks:      []*druidv1alpha1.EtcdOpsTask{buildPreSyncTask(preSyncTaskPrefixUpgrade, 0, ptr.To(druidv1alpha1.TaskStateInProgress))},
			expectedErrCode:    ptr.To(druidapicommon.ErrorCode(druiderr.ErrRequeueAfter)),
		},
		{
			name:               "upgrade requeues when task failed and retries remain",
			backupEnabled:      true,
			stsExists:          true,
			featureGateEnabled: true,
			stsReplicas:        3,
			etcdReplicas:       3,
			etcdWrapperImage:   oldImage,
			existingTasks:      []*druidv1alpha1.EtcdOpsTask{buildPreSyncTask(preSyncTaskPrefixUpgrade, 1, ptr.To(druidv1alpha1.TaskStateFailed))},
			expectedErrCode:    ptr.To(druidapicommon.ErrorCode(druiderr.ErrRequeueAfter)),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			err := druidconfigv1alpha1.DefaultFeatureGates.SetEnabledFeaturesFromMap(
				map[string]bool{druidconfigv1alpha1.UpgradeEtcdVersion: tc.featureGateEnabled},
			)
			g.Expect(err).ToNot(HaveOccurred())

			etcdBuilder := testutils.EtcdBuilderWithDefaults(testutils.TestEtcdName, testutils.TestNamespace).
				WithReplicas(tc.etcdReplicas)
			if !tc.backupEnabled {
				etcdBuilder = etcdBuilder.WithoutProvider()
			}
			etcd := etcdBuilder.Build()

			iv := testutils.CreateImageVector(true, true)

			etcdWrapperImage := tc.etcdWrapperImage
			if etcdWrapperImage == currentImage {
				etcdWrapperImage, _, _, err = utils.GetEtcdImages(etcd, iv)
				g.Expect(err).ToNot(HaveOccurred())
			}

			var existingObjects []client.Object
			if tc.stsExists {
				existingObjects = append(existingObjects, buildStatefulSetWithImage(etcd.ObjectMeta, tc.stsReplicas, etcdWrapperImage))
			}
			for _, task := range tc.existingTasks {
				existingObjects = append(existingObjects, task)
			}

			cl := testutils.NewTestClientBuilder().
				WithScheme(kubernetes.Scheme).
				WithObjects(existingObjects...).
				Build()
			operator := New(cl, iv)
			opCtx := component.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())

			syncErr := operator.PreSync(opCtx, etcd)

			if tc.expectedErrCode == nil {
				g.Expect(syncErr).ToNot(HaveOccurred())
			} else {
				g.Expect(syncErr).To(HaveOccurred())
				druidErr := druiderr.AsDruidError(syncErr)
				g.Expect(druidErr).ToNot(BeNil())
				g.Expect(druidErr.Code).To(Equal(*tc.expectedErrCode))
			}
		})
	}
}

// ----------------------------------- Sync -----------------------------------
func TestSyncWhenNoSTSExists(t *testing.T) {
	testCases := []struct {
		name                        string
		replicas                    int32
		hasExternallyManagedMembers bool
		createErr                   *apierrors.StatusError
		expectedErr                 *druiderr.DruidError
		expectedReplicas            *int32
		expectNoServiceAccount      bool
		expectNoService             bool
		etcdEnv                     []corev1.EnvVar
		etcdVolumeMounts            []corev1.VolumeMount
		volumes                     []corev1.Volume
		backupEnv                   []corev1.EnvVar
		backupVolumeMounts          []corev1.VolumeMount
	}{
		{
			name:             "creates a single replica sts for a single node etcd cluster",
			replicas:         1,
			expectedReplicas: ptr.To[int32](1),
		},
		{
			name:             "creates multiple replica sts for a multi-node etcd cluster",
			replicas:         3,
			expectedReplicas: ptr.To[int32](3),
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
			name:                        "creates sts with 0 replicas, with no service defined and client-service-endpoint CLI flags disabled on backup-restore container when members are managed externally",
			replicas:                    3,
			hasExternallyManagedMembers: true,
			expectedReplicas:            ptr.To[int32](0),
			expectNoService:             true,
		},
		{
			name:             "creates sts with additional env, volumes and volume mounts",
			replicas:         1,
			expectedReplicas: ptr.To[int32](1),
			etcdEnv: []corev1.EnvVar{
				{Name: "CUSTOM_ETCD_VAR", Value: "etcd-value"},
			},
			etcdVolumeMounts: []corev1.VolumeMount{
				{Name: "custom-etcd-vol", MountPath: "/custom/etcd"},
			},
			volumes: []corev1.Volume{
				{Name: "custom-etcd-vol", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
				{Name: "custom-br-vol", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
			},
			backupEnv: []corev1.EnvVar{
				{Name: "CUSTOM_BR_VAR", Value: "br-value"},
			},
			backupVolumeMounts: []corev1.VolumeMount{
				{Name: "custom-br-vol", MountPath: "/custom/br"},
			},
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
				WithReplicas(tc.replicas)
			if tc.hasExternallyManagedMembers {
				etcdBuilder = etcdBuilder.WithExternallyManagedMembers([]string{"1.1.1.1", "1.1.1.2", "1.1.1.3"})
			}
			if tc.etcdEnv != nil {
				etcdBuilder = etcdBuilder.WithEtcdEnv(tc.etcdEnv)
			}
			if tc.etcdVolumeMounts != nil {
				etcdBuilder = etcdBuilder.WithEtcdVolumeMounts(tc.etcdVolumeMounts)
			}
			if tc.volumes != nil {
				etcdBuilder = etcdBuilder.WithVolumes(tc.volumes)
			}
			if tc.backupEnv != nil {
				etcdBuilder = etcdBuilder.WithBackupEnv(tc.backupEnv)
			}
			if tc.backupVolumeMounts != nil {
				etcdBuilder = etcdBuilder.WithBackupVolumeMounts(tc.backupVolumeMounts)
			}
			etcd := etcdBuilder.Build()

			cl := testutils.CreateTestFakeClientForObjects(nil, tc.createErr, nil, nil, []client.Object{buildBackupSecret()}, getObjectKey(etcd.ObjectMeta))
			etcdImage, etcdBRImage, initContainerImage, err := utils.GetEtcdImages(etcd, iv)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(tc.expectedReplicas).ToNot(BeNil())
			stsMatcher := NewStatefulSetMatcher(g, cl, etcd, *tc.expectedReplicas, initContainerImage, etcdImage, etcdBRImage, ptr.To(druidstore.Local), tc.expectNoService)
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

func buildPreSyncTask(prefix string, index int, state *druidv1alpha1.TaskState) *druidv1alpha1.EtcdOpsTask {
	taskName := fmt.Sprintf("%s%d", prefix, index)
	builder := testutils.EtcdOpsTaskBuilderWithDefaults(taskName, testutils.TestNamespace).
		WithEtcdName(testutils.TestEtcdName).
		WithOnDemandSnapshotConfig(&druidv1alpha1.OnDemandSnapshotConfig{})
	if state != nil {
		builder = builder.WithState(*state)
	}
	task := builder.Build()
	return task
}

func buildStatefulSetWithImage(objMeta metav1.ObjectMeta, replicas int32, image string) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      objMeta.Name,
			Namespace: objMeta.Namespace,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: ptr.To(replicas),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"name":     "etcd",
					"instance": objMeta.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  common.ContainerNameEtcd,
							Image: image,
						},
					},
				},
			},
		},
		Status: appsv1.StatefulSetStatus{
			Replicas: replicas,
		},
	}
}
