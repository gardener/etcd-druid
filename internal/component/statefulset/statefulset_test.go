// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
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
		oldWrapperImage       = "europe-docker.pkg.dev/gardener-project/public/gardener/etcd-wrapper:v0.6.2"
		oldBackupRestoreImage = "europe-docker.pkg.dev/gardener-project/public/gardener/etcdbrctl:v0.30.0"
		oldInitImage          = "europe-docker.pkg.dev/gardener-project/public/3rd/alpine:3.18.4"
	)

	testCases := []struct {
		name            string
		backupEnabled   bool
		stsExists       bool
		stsReplicas     int32
		etcdReplicas    int32
		stsImages       map[string]string // keyed by container name; nil = use current image-vector defaults for all containers
		existingTasks   []*druidv1alpha1.EtcdOpsTask
		expectedErrCode *druidapicommon.ErrorCode
	}{
		// ---------------- No-op rows ----------------
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
		// ---------------- Hibernation rows (regression) ----------------
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
			existingTasks: []*druidv1alpha1.EtcdOpsTask{buildPreSyncTask(preSyncTaskPrefixHibernation, 0, ptr.To(druidv1alpha1.TaskStateSucceeded))},
		},
		{
			name:          "hibernation proceeds after max retries exceeded",
			backupEnabled: true,
			stsExists:     true,
			stsReplicas:   3,
			etcdReplicas:  0,
			existingTasks: []*druidv1alpha1.EtcdOpsTask{buildPreSyncTask(preSyncTaskPrefixHibernation, maxPreSyncRetries-1, ptr.To(druidv1alpha1.TaskStateFailed))},
		},
		// ---------------- Image-change rows ----------------
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
			existingTasks: []*druidv1alpha1.EtcdOpsTask{buildPreSyncTask(preSyncTaskPrefixUpdate, 0, ptr.To(druidv1alpha1.TaskStateSucceeded))},
		},
		{
			name:          "update proceeds after max retries exceeded",
			backupEnabled: true,
			stsExists:     true,
			stsReplicas:   3,
			etcdReplicas:  3,
			stsImages:     map[string]string{common.ContainerNameEtcd: oldWrapperImage},
			existingTasks: []*druidv1alpha1.EtcdOpsTask{buildPreSyncTask(preSyncTaskPrefixUpdate, maxPreSyncRetries-1, ptr.To(druidv1alpha1.TaskStateFailed))},
		},
		{
			name:            "update requeues when task is in progress",
			backupEnabled:   true,
			stsExists:       true,
			stsReplicas:     3,
			etcdReplicas:    3,
			stsImages:       map[string]string{common.ContainerNameEtcd: oldWrapperImage},
			existingTasks:   []*druidv1alpha1.EtcdOpsTask{buildPreSyncTask(preSyncTaskPrefixUpdate, 0, ptr.To(druidv1alpha1.TaskStateInProgress))},
			expectedErrCode: ptr.To(druidapicommon.ErrorCode(druiderr.ErrRequeueAfter)),
		},
		{
			name:            "update requeues when task failed and retries remain",
			backupEnabled:   true,
			stsExists:       true,
			stsReplicas:     3,
			etcdReplicas:    3,
			stsImages:       map[string]string{common.ContainerNameEtcd: oldWrapperImage},
			existingTasks:   []*druidv1alpha1.EtcdOpsTask{buildPreSyncTask(preSyncTaskPrefixUpdate, 1, ptr.To(druidv1alpha1.TaskStateFailed))},
			expectedErrCode: ptr.To(druidapicommon.ErrorCode(druiderr.ErrRequeueAfter)),
		},
		// ---------------- Replica-change rows ----------------
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
		// ---------------- Combination row ----------------
		{
			name:          "hibernation prefix wins when replicas go to 0 even if images also changed",
			backupEnabled: true,
			stsExists:     true,
			stsReplicas:   3,
			etcdReplicas:  0,
			stsImages:     map[string]string{common.ContainerNameEtcd: oldWrapperImage},
			existingTasks: []*druidv1alpha1.EtcdOpsTask{buildPreSyncTask(preSyncTaskPrefixHibernation, 0, ptr.To(druidv1alpha1.TaskStateSucceeded))},
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
			etcd := etcdBuilder.Build()

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
		expectNoService             bool
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
