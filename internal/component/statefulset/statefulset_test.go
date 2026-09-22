// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package statefulset

import (
	"context"
	"fmt"
	"maps"
	"testing"

	druidapicommon "github.com/gardener/etcd-druid/api/common"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	etcdfake "github.com/gardener/etcd-druid/internal/client/etcd/fake"
	clientkubernetes "github.com/gardener/etcd-druid/internal/client/kubernetes"
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
	t.Parallel()
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
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var existingObjects []client.Object
			if tc.stsExists {
				existingObjects = append(existingObjects, emptyStatefulSet(etcd.ObjectMeta))
			}
			cl := testutils.CreateTestFakeClientForObjects(tc.getErr, nil, nil, nil, existingObjects, getObjectKey(etcd.ObjectMeta))
			operator := New(cl, nil, &etcdfake.Factory{Client: etcdfake.NewClient("etcd-test", 1)})
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
	t.Parallel()
	const (
		oldWrapperImage       = "europe-docker.pkg.dev/gardener-project/public/gardener/etcd-wrapper:v0.6.2"
		oldBackupRestoreImage = "europe-docker.pkg.dev/gardener-project/public/gardener/etcdbrctl:v0.30.0"
		oldInitImage          = "europe-docker.pkg.dev/gardener-project/public/3rd/alpine:3.18.4"
	)

	testCases := []struct {
		name                string
		backupEnabled       bool
		stsExists           bool
		stsReplicas         int32
		etcdReplicas        int32
		etcdGeneration      int64             // when non-zero, overrides the fake etcd's generation
		stsImages           map[string]string // keyed by container name; nil = use current image-vector defaults for all containers
		existingTasks       []*druidv1alpha1.EtcdOpsTask
		skipAnnotation      bool // when true, sets the skip-spec-update-snapshot annotation on the Etcd
		expectedErrCode     *druidapicommon.ErrorCode
		expectNoTasks       bool   // when true, asserts that no EtcdOpsTask exists after PreSync
		expectedFailure     bool   // when true, asserts the failure flag is set in OperatorContext.Data
		expectedNewTaskName string // when non-empty, asserts the specific new task name created by PreSync
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
			name:          "returns nil when STS replicas are 0",
			backupEnabled: true,
			stsExists:     true,
			stsReplicas:   0,
			etcdReplicas:  3,
		},
		{
			name:          "returns nil when no image change and no replica change",
			backupEnabled: true,
			stsExists:     true,
			stsReplicas:   3,
			etcdReplicas:  3,
		},
		{
			name:            "hibernation requeues when no task exists",
			backupEnabled:   true,
			stsExists:       true,
			stsReplicas:     3,
			etcdReplicas:    0,
			expectedErrCode: ptr.To(druidapicommon.ErrorCode(druiderr.ErrRequeueAfter)),
		},
		{
			name:          "hibernation succeeds when task completed",
			backupEnabled: true,
			stsExists:     true,
			stsReplicas:   3,
			etcdReplicas:  0,
			existingTasks: []*druidv1alpha1.EtcdOpsTask{buildPreSyncTask(fmt.Sprintf("%s%d-", preSyncTaskHibernationPrefix, 0), 0, ptr.To(druidv1alpha1.TaskStateSucceeded))},
		},
		{
			name:            "hibernation proceeds after max retries exceeded",
			backupEnabled:   true,
			stsExists:       true,
			stsReplicas:     3,
			etcdReplicas:    0,
			existingTasks:   []*druidv1alpha1.EtcdOpsTask{buildPreSyncTask(fmt.Sprintf("%s%d-", preSyncTaskHibernationPrefix, 0), maxPreSyncRetries-1, ptr.To(druidv1alpha1.TaskStateFailed))},
			expectedFailure: true,
		},
		{
			name:            "update requeues when wrapper image changed and no task exists",
			backupEnabled:   true,
			stsExists:       true,
			stsReplicas:     3,
			etcdReplicas:    3,
			stsImages:       map[string]string{common.ContainerNameEtcd: oldWrapperImage},
			expectedErrCode: ptr.To(druidapicommon.ErrorCode(druiderr.ErrRequeueAfter)),
		},
		{
			name:            "update requeues when backup-restore image changed and no task exists",
			backupEnabled:   true,
			stsExists:       true,
			stsReplicas:     3,
			etcdReplicas:    3,
			stsImages:       map[string]string{common.ContainerNameEtcdBackupRestore: oldBackupRestoreImage},
			expectedErrCode: ptr.To(druidapicommon.ErrorCode(druiderr.ErrRequeueAfter)),
		},
		{
			name:            "update requeues when init-container image changed and no task exists",
			backupEnabled:   true,
			stsExists:       true,
			stsReplicas:     3,
			etcdReplicas:    3,
			stsImages:       map[string]string{common.InitContainerNameChangeBackupBucketPermissions: oldInitImage},
			expectedErrCode: ptr.To(druidapicommon.ErrorCode(druiderr.ErrRequeueAfter)),
		},
		{
			name:          "update succeeds when task completed",
			backupEnabled: true,
			stsExists:     true,
			stsReplicas:   3,
			etcdReplicas:  3,
			stsImages:     map[string]string{common.ContainerNameEtcd: oldWrapperImage},
			existingTasks: []*druidv1alpha1.EtcdOpsTask{buildPreSyncTask(fmt.Sprintf("%s%d-", preSyncTaskUpdatePrefix, 0), 0, ptr.To(druidv1alpha1.TaskStateSucceeded))},
		},
		{
			name:            "update proceeds after max retries exceeded",
			backupEnabled:   true,
			stsExists:       true,
			stsReplicas:     3,
			etcdReplicas:    3,
			stsImages:       map[string]string{common.ContainerNameEtcd: oldWrapperImage},
			existingTasks:   []*druidv1alpha1.EtcdOpsTask{buildPreSyncTask(fmt.Sprintf("%s%d-", preSyncTaskUpdatePrefix, 0), maxPreSyncRetries-1, ptr.To(druidv1alpha1.TaskStateFailed))},
			expectedFailure: true,
		},
		{
			name:            "update requeues when task is in progress",
			backupEnabled:   true,
			stsExists:       true,
			stsReplicas:     3,
			etcdReplicas:    3,
			stsImages:       map[string]string{common.ContainerNameEtcd: oldWrapperImage},
			existingTasks:   []*druidv1alpha1.EtcdOpsTask{buildPreSyncTask(fmt.Sprintf("%s%d-", preSyncTaskUpdatePrefix, 0), 0, ptr.To(druidv1alpha1.TaskStateInProgress))},
			expectedErrCode: ptr.To(druidapicommon.ErrorCode(druiderr.ErrRequeueAfter)),
		},
		{
			name:            "update requeues when task failed and retries remain",
			backupEnabled:   true,
			stsExists:       true,
			stsReplicas:     3,
			etcdReplicas:    3,
			stsImages:       map[string]string{common.ContainerNameEtcd: oldWrapperImage},
			existingTasks:   []*druidv1alpha1.EtcdOpsTask{buildPreSyncTask(fmt.Sprintf("%s%d-", preSyncTaskUpdatePrefix, 0), 1, ptr.To(druidv1alpha1.TaskStateFailed))},
			expectedErrCode: ptr.To(druidapicommon.ErrorCode(druiderr.ErrRequeueAfter)),
		},
		{
			name:            "update requeues when replicas scale up (3->5)",
			backupEnabled:   true,
			stsExists:       true,
			stsReplicas:     3,
			etcdReplicas:    5,
			expectedErrCode: ptr.To(druidapicommon.ErrorCode(druiderr.ErrRequeueAfter)),
		},
		{
			name:            "update requeues when replicas scale down to non-zero (5->3)",
			backupEnabled:   true,
			stsExists:       true,
			stsReplicas:     5,
			etcdReplicas:    3,
			expectedErrCode: ptr.To(druidapicommon.ErrorCode(druiderr.ErrRequeueAfter)),
		},
		{
			name:          "hibernation prefix wins when replicas go to 0 even if images also changed",
			backupEnabled: true,
			stsExists:     true,
			stsReplicas:   3,
			etcdReplicas:  0,
			stsImages:     map[string]string{common.ContainerNameEtcd: oldWrapperImage},
			existingTasks: []*druidv1alpha1.EtcdOpsTask{buildPreSyncTask(fmt.Sprintf("%s%d-", preSyncTaskHibernationPrefix, 0), 0, ptr.To(druidv1alpha1.TaskStateSucceeded))},
		},
		{
			name:           "update skipped when skip annotation present (image change)",
			backupEnabled:  true,
			stsExists:      true,
			stsReplicas:    3,
			etcdReplicas:   3,
			stsImages:      map[string]string{common.ContainerNameEtcd: oldWrapperImage},
			skipAnnotation: true,
			expectNoTasks:  true,
		},
		{
			name:           "update skipped when skip annotation present (replica change)",
			backupEnabled:  true,
			stsExists:      true,
			stsReplicas:    3,
			etcdReplicas:   5,
			skipAnnotation: true,
			expectNoTasks:  true,
		},
		{
			name:           "skip annotation short-circuits even when no image or replica change",
			backupEnabled:  true,
			stsExists:      true,
			stsReplicas:    3,
			etcdReplicas:   3,
			skipAnnotation: true,
			expectNoTasks:  true,
		},
		{
			name:            "waits for in-progress task from a previous generation",
			backupEnabled:   true,
			stsExists:       true,
			stsReplicas:     3,
			etcdReplicas:    3,
			etcdGeneration:  1,
			stsImages:       map[string]string{common.ContainerNameEtcd: oldWrapperImage},
			existingTasks:   []*druidv1alpha1.EtcdOpsTask{buildPreSyncTask(fmt.Sprintf("%s%d-", preSyncTaskUpdatePrefix, 0), 0, ptr.To(druidv1alpha1.TaskStateInProgress))},
			expectedErrCode: ptr.To(druidapicommon.ErrorCode(druiderr.ErrRequeueAfter)),
		},
		{
			name:           "adopts succeeded snapshot from a previous generation",
			backupEnabled:  true,
			stsExists:      true,
			stsReplicas:    3,
			etcdReplicas:   3,
			etcdGeneration: 1,
			stsImages:      map[string]string{common.ContainerNameEtcd: oldWrapperImage},
			existingTasks:  []*druidv1alpha1.EtcdOpsTask{buildPreSyncTask(fmt.Sprintf("%s%d-", preSyncTaskUpdatePrefix, 0), 0, ptr.To(druidv1alpha1.TaskStateSucceeded))},
		},
		{
			name:                "continues retry count from previous generation after failure (creates current-gen task at inherited index)",
			backupEnabled:       true,
			stsExists:           true,
			stsReplicas:         3,
			etcdReplicas:        3,
			etcdGeneration:      1,
			stsImages:           map[string]string{common.ContainerNameEtcd: oldWrapperImage},
			existingTasks:       []*druidv1alpha1.EtcdOpsTask{buildPreSyncTask(fmt.Sprintf("%s%d-", preSyncTaskUpdatePrefix, 0), 1, ptr.To(druidv1alpha1.TaskStateFailed))},
			expectedErrCode:     ptr.To(druidapicommon.ErrorCode(druiderr.ErrRequeueAfter)),
			expectedNewTaskName: fmt.Sprintf("%s%d-%d", preSyncTaskUpdatePrefix, 1, 2),
		},
		{
			name:            "exhausts retries inherited across generations and proceeds with sync",
			backupEnabled:   true,
			stsExists:       true,
			stsReplicas:     3,
			etcdReplicas:    3,
			etcdGeneration:  1,
			stsImages:       map[string]string{common.ContainerNameEtcd: oldWrapperImage},
			existingTasks:   []*druidv1alpha1.EtcdOpsTask{buildPreSyncTask(fmt.Sprintf("%s%d-", preSyncTaskUpdatePrefix, 0), maxPreSyncRetries-1, ptr.To(druidv1alpha1.TaskStateFailed))},
			expectedFailure: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)

			etcdBuilder := testutils.EtcdBuilderWithDefaults(testutils.TestEtcdName, testutils.TestNamespace).
				WithReplicas(tc.etcdReplicas)
			if !tc.backupEnabled {
				etcdBuilder = etcdBuilder.WithoutProvider()
			}
			if tc.skipAnnotation {
				etcdBuilder = etcdBuilder.WithAnnotations(map[string]string{druidv1alpha1.SkipSpecUpdateSnapshotAnnotation: ""})
			}
			etcd := etcdBuilder.Build()
			if tc.etcdGeneration != 0 {
				etcd.Generation = tc.etcdGeneration
			}

			iv := testutils.CreateImageVector(true, true)

			defaultWrapperImage, defaultBRImage, defaultInitImage, err := utils.GetEtcdImages(etcd, iv)
			g.Expect(err).ToNot(HaveOccurred())

			stsImages := map[string]string{
				common.ContainerNameEtcd:                              defaultWrapperImage,
				common.ContainerNameEtcdBackupRestore:                 defaultBRImage,
				common.InitContainerNameChangeBackupBucketPermissions: defaultInitImage,
			}
			maps.Copy(stsImages, tc.stsImages)

			var existingObjects []client.Object
			if tc.stsExists {
				existingObjects = append(existingObjects, buildStatefulSetWithImages(etcd.ObjectMeta, tc.stsReplicas, stsImages))
			}
			for _, task := range tc.existingTasks {
				existingObjects = append(existingObjects, task)
			}

			cl := testutils.NewTestClientBuilder().
				WithScheme(clientkubernetes.Scheme).
				WithObjects(existingObjects...).
				Build()
			operator := New(cl, iv, &etcdfake.Factory{Client: etcdfake.NewClient("etcd-test", 1)})
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

			if tc.expectNoTasks {
				taskList := &druidv1alpha1.EtcdOpsTaskList{}
				g.Expect(cl.List(opCtx, taskList, client.InNamespace(etcd.Namespace))).To(Succeed())
				g.Expect(taskList.Items).To(BeEmpty(), "expected no EtcdOpsTask to be created when the update snapshot is skipped")
			}

			if tc.skipAnnotation {
				// etcd-druid never removes the skip annotation; it must remain on the resource after PreSync.
				g.Expect(etcd.Annotations).To(HaveKey(druidv1alpha1.SkipSpecUpdateSnapshotAnnotation))
			}

			_, isSnapshotFailed := opCtx.Data[common.KeyPreSyncSnapshotFailed]
			g.Expect(isSnapshotFailed).To(Equal(tc.expectedFailure))

			if tc.expectedNewTaskName != "" {
				// verify PreSync created the task with the expected name
				newTask := &druidv1alpha1.EtcdOpsTask{}
				g.Expect(cl.Get(opCtx, client.ObjectKey{Name: tc.expectedNewTaskName, Namespace: etcd.Namespace}, newTask)).To(Succeed(),
					"expected PreSync to create task %q but it was not found", tc.expectedNewTaskName)
			}
		})
	}
}

// ----------------------------------- Sync -----------------------------------
func TestSyncWhenNoSTSExists(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name                        string
		replicas                    int32
		tolerations                 []corev1.Toleration
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
			name:     "creates sts with tolerations propagated from schedulingConstraints",
			replicas: 3,
			tolerations: []corev1.Toleration{
				{
					Key:      "dedicated",
					Operator: corev1.TolerationOpEqual,
					Value:    "etcd",
					Effect:   corev1.TaintEffectNoSchedule,
				},
			},
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
	iv := testutils.CreateImageVector(true, true)
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// *************** Build test environment ***************
			etcdBuilder := testutils.EtcdBuilderWithDefaults(testutils.TestEtcdName, testutils.TestNamespace).
				WithReplicas(tc.replicas).
				WithTolerations(tc.tolerations)
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
			operator := New(cl, iv, &etcdfake.Factory{Client: etcdfake.NewClient("etcd-test", 1)})
			// *************** Test and assert ***************
			opCtx := component.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
			opCtx.Data[common.CheckSumKeyConfigMap] = testutils.TestConfigMapCheckSum
			syncErr := operator.Sync(opCtx, etcd)
			latestSTS, getErr := getLatestStatefulSet(cl, etcd)
			if tc.expectedErr != nil {
				testutils.CheckDruidError(g, tc.expectedErr, syncErr)
				g.Expect(getErr).To(MatchError(apierrors.IsNotFound, "IsNotFound"))
			} else {
				g.Expect(syncErr).To(Succeed())
				g.Expect(getErr).To(Succeed())
				g.Expect(latestSTS).ToNot(BeNil())
				g.Expect(*latestSTS).Should(stsMatcher.MatchStatefulSet())
			}
		})
	}
}

// TestSyncScaleInShrinksStatefulSet is the regression guard for the DEP-08
// scale-in deadlock. A scale-in Sync must, in a single pass, delete the surplus
// PVCs (ordinals >= spec.replicas) AND shrink the StatefulSet to spec.replicas.
// The earlier bug requeued after issuing the PVC deletes, so the StatefulSet was
// never shrunk: the surplus pods that held the pvc-protection finalizer were
// never removed, the PVCs stayed Terminating, and the reconcile requeued forever.
func TestSyncScaleInShrinksStatefulSet(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)
	iv := testutils.CreateImageVector(true, true)

	const (
		initialReplicas int32 = 5
		targetReplicas  int32 = 3
	)

	// Seed a fully in-sync StatefulSet at the initial size by running createOrPatch
	// once, so the subsequent scale-in Sync goes straight to the shrink path
	// (handleTLSChanges is a no-op when the STS already matches the etcd spec).
	etcd := testutils.EtcdBuilderWithDefaults(testutils.TestEtcdName, testutils.TestNamespace).
		WithReplicas(initialReplicas).
		Build()
	cl := testutils.CreateTestFakeClientForObjects(nil, nil, nil, nil, []client.Object{buildBackupSecret()}, getObjectKey(etcd.ObjectMeta))
	operator := New(cl, iv, &etcdfake.Factory{Client: etcdfake.NewClient("etcd-test", 1)})
	opCtx := component.NewOperatorContext(context.Background(), logr.Discard(), uuid.NewString())
	opCtx.Data[common.CheckSumKeyConfigMap] = testutils.TestConfigMapCheckSum

	g.Expect(operator.Sync(opCtx, etcd)).To(Succeed())
	seededSTS, err := getLatestStatefulSet(cl, etcd)
	g.Expect(err).To(Succeed())
	g.Expect(seededSTS.Spec.Replicas).To(HaveValue(Equal(initialReplicas)))

	// Create the PVCs the StatefulSet would have provisioned for ordinals 0..4.
	for i := int32(0); i < initialReplicas; i++ {
		podName := druidv1alpha1.GetOrdinalPodName(etcd.ObjectMeta, int(i))
		g.Expect(cl.Create(context.Background(), testutils.CreatePVC(seededSTS, podName, corev1.ClaimBound))).To(Succeed())
	}

	// Create member leases for all initial members so that handleTLSChanges can
	// confirm peer-TLS state via IsPeerURLInSyncForAllMembers and return nil.
	for _, leaseName := range druidv1alpha1.GetMemberLeaseNames(etcd) {
		lease := testutils.CreateLease(leaseName, etcd.Namespace, etcd.Name, etcd.UID, common.ComponentNameMemberLease)
		g.Expect(cl.Create(context.Background(), lease)).To(Succeed())
	}

	// Request a scale-in and mark the in-flight condition the detector would have set.
	etcd.Spec.Replicas = targetReplicas
	etcd.Status.Conditions = []druidv1alpha1.Condition{{
		Type:   druidv1alpha1.ConditionTypeScaleOperationComplete,
		Status: druidv1alpha1.ConditionFalse,
		Reason: druidv1alpha1.ScaleOperationReasonScalingIn,
	}}

	// First Sync: deletes the surplus PVCs and requeues (PVC protection finalizer
	// on real clusters means the STS cannot shrink until PVCs are gone).
	firstSyncErr := operator.Sync(opCtx, etcd)
	g.Expect(druiderr.AsDruidError(firstSyncErr)).NotTo(BeNil())
	g.Expect(string(druiderr.AsDruidError(firstSyncErr).Code)).To(Equal(druiderr.ErrRequeueAfter),
		"first Sync should requeue after deleting surplus PVCs")

	// Surplus PVCs must be deleted after the first Sync pass.
	vctName := ptr.Deref(etcd.Spec.VolumeClaimTemplate, etcd.Name)
	stsName := druidv1alpha1.GetStatefulSetName(etcd.ObjectMeta)
	getPVCErr := func(ordinal int32) error {
		pvcName := fmt.Sprintf("%s-%s-%d", vctName, stsName, ordinal)
		return cl.Get(context.Background(), client.ObjectKey{Namespace: etcd.Namespace, Name: pvcName}, &corev1.PersistentVolumeClaim{})
	}
	for _, ordinal := range []int32{3, 4} {
		g.Expect(getPVCErr(ordinal)).To(MatchError(apierrors.IsNotFound, "IsNotFound"),
			"surplus PVC for ordinal %d must be deleted in the first Sync pass", ordinal)
	}

	// Second Sync: with surplus PVCs gone, shrinks the StatefulSet.
	g.Expect(operator.Sync(opCtx, etcd)).To(Succeed())

	shrunkSTS, err := getLatestStatefulSet(cl, etcd)
	g.Expect(err).To(Succeed())
	g.Expect(shrunkSTS.Spec.Replicas).To(HaveValue(Equal(targetReplicas)), "StatefulSet must be shrunk to spec.replicas after surplus PVCs are gone")
	for _, ordinal := range []int32{0, 1, 2} {
		g.Expect(getPVCErr(ordinal)).To(Succeed(), "retained PVC for ordinal %d must not be deleted", ordinal)
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

func buildStatefulSetWithImages(objMeta metav1.ObjectMeta, replicas int32, images map[string]string) *appsv1.StatefulSet {
	containers := make([]corev1.Container, 0, 2)
	if img, ok := images[common.ContainerNameEtcd]; ok {
		containers = append(containers, corev1.Container{
			Name:  common.ContainerNameEtcd,
			Image: img,
		})
	}
	if img, ok := images[common.ContainerNameEtcdBackupRestore]; ok {
		containers = append(containers, corev1.Container{
			Name:  common.ContainerNameEtcdBackupRestore,
			Image: img,
		})
	}
	var initContainers []corev1.Container
	if img, ok := images[common.InitContainerNameChangeBackupBucketPermissions]; ok {
		initContainers = append(initContainers, corev1.Container{
			Name:  common.InitContainerNameChangeBackupBucketPermissions,
			Image: img,
		})
	}
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
					InitContainers: initContainers,
					Containers:     containers,
				},
			},
		},
		Status: appsv1.StatefulSetStatus{
			Replicas: replicas,
		},
	}
}
