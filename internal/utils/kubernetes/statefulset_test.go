// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"strings"
	"testing"

	"github.com/gardener/etcd-druid/internal/client/kubernetes"
	"github.com/gardener/etcd-druid/internal/common"
	"github.com/gardener/etcd-druid/internal/utils"
	testutils "github.com/gardener/etcd-druid/test/utils"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/uuid"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/gomega"
)

const (
	stsNamespace = "test-ns"
	stsName      = "etcd-test"
	eventName    = "test-event"
)

func TestIsStatefulSetReady(t *testing.T) {
	testCases := []struct {
		name                          string
		specGeneration                int64
		statusObservedGeneration      int64
		etcdSpecReplicas              int32
		statusReadyReplicas           int32
		statusCurrentReplicas         int32
		statusUpdatedReplicas         int32
		statusCurrentRevision         string
		statusUpdateRevision          string
		expectedStsReady              bool
		expectedNotReadyReasonPresent bool
	}{
		{
			name:                          "sts has less number of ready replicas as compared to configured etcd replicas",
			specGeneration:                1,
			statusObservedGeneration:      1,
			etcdSpecReplicas:              3,
			statusReadyReplicas:           2,
			expectedStsReady:              false,
			expectedNotReadyReasonPresent: true,
		},
		{
			name:                          "sts has equal number of replicas as defined in etcd but observed generation is outdated",
			specGeneration:                2,
			statusObservedGeneration:      1,
			etcdSpecReplicas:              3,
			statusReadyReplicas:           3,
			expectedStsReady:              false,
			expectedNotReadyReasonPresent: true,
		},
		{
			name:                          "sts has mismatching current and update revision",
			specGeneration:                2,
			statusObservedGeneration:      2,
			etcdSpecReplicas:              3,
			statusReadyReplicas:           3,
			statusCurrentRevision:         "etcd-main-6d5cc8f559",
			statusUpdateRevision:          "etcd-main-bf6b695326",
			expectedStsReady:              false,
			expectedNotReadyReasonPresent: true,
		},
		{
			name:                          "sts has mismatching status ready and updated replicas",
			specGeneration:                2,
			statusObservedGeneration:      2,
			etcdSpecReplicas:              3,
			statusReadyReplicas:           3,
			statusCurrentRevision:         "etcd-main-6d5cc8f559",
			statusUpdateRevision:          "etcd-main-bf6b695326",
			statusCurrentReplicas:         3,
			statusUpdatedReplicas:         2,
			expectedStsReady:              false,
			expectedNotReadyReasonPresent: true,
		},
		{
			name:                          "sts is completely up-to-date",
			specGeneration:                2,
			statusObservedGeneration:      2,
			etcdSpecReplicas:              3,
			statusReadyReplicas:           3,
			statusCurrentRevision:         "etcd-main-6d5cc8f559",
			statusUpdateRevision:          "etcd-main-6d5cc8f559",
			statusCurrentReplicas:         3,
			statusUpdatedReplicas:         3,
			expectedStsReady:              true,
			expectedNotReadyReasonPresent: false,
		},
	}

	g := NewWithT(t)
	t.Parallel()
	for _, tc := range testCases {
		t.Run(tc.name, func(_ *testing.T) {
			sts := testutils.CreateStatefulSet(stsName, stsNamespace, uuid.NewUUID(), 2)
			sts.Generation = tc.specGeneration
			sts.Status.ObservedGeneration = tc.statusObservedGeneration
			sts.Status.ReadyReplicas = tc.statusReadyReplicas
			sts.Status.CurrentReplicas = tc.statusCurrentReplicas
			sts.Status.UpdatedReplicas = tc.statusUpdatedReplicas
			sts.Status.CurrentRevision = tc.statusCurrentRevision
			sts.Status.UpdateRevision = tc.statusUpdateRevision
			stsReady, reasonMsg := IsStatefulSetReady(context.Background(), nil, tc.etcdSpecReplicas, sts)
			g.Expect(stsReady).To(Equal(tc.expectedStsReady))
			g.Expect(!utils.IsEmptyString(reasonMsg)).To(Equal(tc.expectedNotReadyReasonPresent))
		})
	}
}

func TestGetStatefulSet(t *testing.T) {
	internalErr := errors.New("test internal error")
	apiInternalErr := apierrors.NewInternalError(internalErr)

	etcd := testutils.EtcdBuilderWithDefaults(testutils.TestEtcdName, testutils.TestNamespace).WithReplicas(3).Build()
	testCases := []struct {
		name         string
		isStsPresent bool
		ownedByEtcd  bool
		getErr       *apierrors.StatusError
		expectedErr  *apierrors.StatusError
	}{
		{
			name:         "no sts found",
			isStsPresent: false,
			ownedByEtcd:  false,
		},
		{
			name:         "sts found but not owned by etcd",
			isStsPresent: true,
			ownedByEtcd:  false,
		},
		{
			name:         "sts found and owned by etcd",
			isStsPresent: true,
			ownedByEtcd:  true,
		},
		{
			name:         "returns error when client get fails",
			isStsPresent: true,
			ownedByEtcd:  true,
			getErr:       apiInternalErr,
			expectedErr:  apiInternalErr,
		},
	}

	g := NewWithT(t)
	t.Parallel()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var existingObjects []client.Object
			if tc.isStsPresent {
				etcdUID := etcd.UID
				if !tc.ownedByEtcd {
					etcdUID = uuid.NewUUID()
				}
				sts := testutils.CreateStatefulSet(etcd.Name, etcd.Namespace, etcdUID, etcd.Spec.Replicas)
				existingObjects = append(existingObjects, sts)
			}
			cl := testutils.CreateTestFakeClientWithSchemeForObjects(kubernetes.Scheme, tc.getErr, nil, nil, nil, existingObjects, client.ObjectKey{Name: testutils.TestEtcdName, Namespace: testutils.TestNamespace})
			foundSts, err := GetStatefulSet(context.Background(), cl, etcd)
			if tc.expectedErr != nil {
				g.Expect(err).To(HaveOccurred())
				g.Expect(errors.Is(err, tc.expectedErr)).To(BeTrue())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
				expectedStsToBeFound := tc.isStsPresent && tc.ownedByEtcd
				g.Expect(foundSts != nil).To(Equal(expectedStsToBeFound))
			}
		})
	}
}

func TestFetchPVCWarningMessagesForStatefulSet(t *testing.T) {
	internalErr := errors.New("test internal error")
	apiInternalErr := apierrors.NewInternalError(internalErr)

	etcd := testutils.EtcdBuilderWithDefaults(testutils.TestEtcdName, testutils.TestNamespace).WithReplicas(3).Build()
	sts := testutils.CreateStatefulSet(etcd.Name, etcd.Namespace, etcd.UID, etcd.Spec.Replicas)
	pvcPending := testutils.CreatePVC(sts, fmt.Sprintf("%s-0", sts.Name), corev1.ClaimPending)
	pvcBound := testutils.CreatePVC(sts, fmt.Sprintf("%s-1", sts.Name), corev1.ClaimBound)
	eventWarning := testutils.CreateEvent(eventName, sts.Namespace, "FailedMount", "test pvc warning message", corev1.EventTypeWarning, pvcPending, schema.GroupVersionKind{Group: "", Version: "v1", Kind: "PersistentVolumeClaim"})

	testCases := []struct {
		name        string
		sts         *appsv1.StatefulSet
		pvcList     []client.Object
		eventList   []client.Object
		errors      []testutils.ErrorsForGVK
		expectedErr *apierrors.StatusError
		expectedMsg string
	}{
		{
			name: "error in listing PVCs",
			errors: []testutils.ErrorsForGVK{
				{
					GVK:     corev1.SchemeGroupVersion.WithKind("PersistentVolumeClaimList"),
					ListErr: apiInternalErr,
				},
			},
			expectedErr: apiInternalErr,
		},
		{
			name:        "no PVCs found",
			pvcList:     nil,
			expectedErr: nil,
			expectedMsg: "",
		},
		{
			name:    "PVCs found but error in listing events",
			pvcList: []client.Object{pvcPending},
			errors: []testutils.ErrorsForGVK{
				{
					GVK:     corev1.SchemeGroupVersion.WithKind("EventList"),
					ListErr: apiInternalErr,
				},
			},
			expectedErr: apiInternalErr,
			expectedMsg: "",
		},
		{
			name:        "PVCs found with warning events",
			pvcList:     []client.Object{pvcPending},
			eventList:   []client.Object{eventWarning},
			expectedErr: nil,
			expectedMsg: eventWarning.Message,
		},
		{
			name:        "PVCs found but no warning events",
			pvcList:     []client.Object{pvcBound},
			expectedMsg: "",
		},
	}

	g := NewWithT(t)
	t.Parallel()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var existingObjects []client.Object
			if tc.pvcList != nil {
				existingObjects = append(existingObjects, tc.pvcList...)
			}
			if tc.eventList != nil {
				existingObjects = append(existingObjects, tc.eventList...)
			}

			cl := testutils.CreateTestFakeClientForObjectsInNamespaceWithGVK(tc.errors, etcd.Namespace, existingObjects...)
			messages, err := FetchPVCWarningMessagesForStatefulSet(context.Background(), cl, sts)
			if tc.expectedErr != nil {
				g.Expect(err).To(HaveOccurred())
				g.Expect(errors.Is(err, tc.expectedErr)).To(BeTrue())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(strings.Contains(messages, tc.expectedMsg)).To(BeTrue())
			}
		})
	}
}

func TestGetEtcdContainerPeerTLSVolumeMounts(t *testing.T) {
	testCases := []struct {
		name                 string
		isSTSNil             bool
		oldVolMountNames     bool
		expectedVolumeMounts []corev1.VolumeMount
	}{
		{
			name:                 "sts is nil",
			isSTSNil:             true,
			expectedVolumeMounts: []corev1.VolumeMount{},
		},
		{
			name:             "sts with old volume mount names",
			oldVolMountNames: true,
			expectedVolumeMounts: []corev1.VolumeMount{
				{Name: common.OldVolumeNameEtcdPeerCA, MountPath: common.VolumeMountPathEtcdPeerCA},
				{Name: common.OldVolumeNameEtcdPeerServerTLS, MountPath: common.VolumeMountPathEtcdPeerServerTLS},
			},
		},
		{
			name: "sts with new volume mount names",
			expectedVolumeMounts: []corev1.VolumeMount{
				{Name: common.VolumeNameEtcdPeerCA, MountPath: common.VolumeMountPathEtcdPeerCA},
				{Name: common.VolumeNameEtcdPeerServerTLS, MountPath: common.VolumeMountPathEtcdPeerServerTLS},
			},
		},
	}
	g := NewWithT(t)
	t.Parallel()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var sts *appsv1.StatefulSet
			if tc.isSTSNil {
				g.Expect(GetEtcdContainerPeerTLSVolumeMounts(sts)).To(HaveLen(0))
			} else {
				sts = testutils.CreateStatefulSet("test-sts", "test-ns", uuid.NewUUID(), 3)
				sts.Spec.Template.Spec.Containers = append(sts.Spec.Template.Spec.Containers, corev1.Container{Name: common.ContainerNameEtcd})
				if tc.oldVolMountNames {
					sts.Spec.Template.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{
						{Name: common.OldVolumeNameEtcdPeerCA, MountPath: common.VolumeMountPathEtcdPeerCA},
						{Name: common.OldVolumeNameEtcdPeerServerTLS, MountPath: common.VolumeMountPathEtcdPeerServerTLS},
					}
				} else {
					sts.Spec.Template.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{
						{Name: common.VolumeNameEtcdPeerCA, MountPath: common.VolumeMountPathEtcdPeerCA},
						{Name: common.VolumeNameEtcdPeerServerTLS, MountPath: common.VolumeMountPathEtcdPeerServerTLS},
					}
				}
				g.Expect(GetEtcdContainerPeerTLSVolumeMounts(sts)).To(Equal(tc.expectedVolumeMounts))
			}
		})
	}
}

func TestGetStatefulSetContainerTLSVolumeMounts(t *testing.T) {
	testCases := []struct {
		name                 string
		isSTSNil             bool
		peerTLSEnabled       bool
		expectedVolumeMounts map[string][]corev1.VolumeMount
	}{
		{
			name:                 "sts is nil",
			isSTSNil:             true,
			expectedVolumeMounts: map[string][]corev1.VolumeMount{},
		},
		{
			name:           "sts with peer TLS enabled",
			peerTLSEnabled: true,
			expectedVolumeMounts: map[string][]corev1.VolumeMount{
				common.ContainerNameEtcd: {
					{Name: common.VolumeNameEtcdPeerCA, MountPath: common.VolumeMountPathEtcdPeerCA},
					{Name: common.VolumeNameEtcdPeerServerTLS, MountPath: common.VolumeMountPathEtcdPeerServerTLS},
				},
				common.ContainerNameEtcdBackupRestore: {
					{Name: common.VolumeNameBackupRestoreServerTLS, MountPath: common.VolumeMountPathBackupRestoreServerTLS},
					{Name: common.VolumeNameEtcdCA, MountPath: common.VolumeMountPathEtcdCA},
					{Name: common.VolumeNameEtcdClientTLS, MountPath: common.VolumeMountPathEtcdClientTLS},
				},
			},
		},
		{
			name:           "sts with peer TLS disabled",
			peerTLSEnabled: false,
			expectedVolumeMounts: map[string][]corev1.VolumeMount{
				common.ContainerNameEtcd: {},
				common.ContainerNameEtcdBackupRestore: {
					{Name: common.VolumeNameBackupRestoreServerTLS, MountPath: common.VolumeMountPathBackupRestoreServerTLS},
					{Name: common.VolumeNameEtcdCA, MountPath: common.VolumeMountPathEtcdCA},
					{Name: common.VolumeNameEtcdClientTLS, MountPath: common.VolumeMountPathEtcdClientTLS},
				},
			},
		},
	}
	g := NewWithT(t)
	t.Parallel()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var sts *appsv1.StatefulSet
			if tc.isSTSNil {
				g.Expect(GetStatefulSetContainerTLSVolumeMounts(sts)).To(HaveLen(0))
			} else {
				sts = testutils.CreateStatefulSet("test-sts", "test-ns", uuid.NewUUID(), 3)
				sts.Spec.Template.Spec.Containers = []corev1.Container{
					{Name: common.ContainerNameEtcd},
					{Name: common.ContainerNameEtcdBackupRestore},
				}
				if tc.peerTLSEnabled {
					sts.Spec.Template.Spec.Containers[0].VolumeMounts = []corev1.VolumeMount{
						{Name: common.VolumeNameEtcdPeerCA, MountPath: common.VolumeMountPathEtcdPeerCA},
						{Name: common.VolumeNameEtcdPeerServerTLS, MountPath: common.VolumeMountPathEtcdPeerServerTLS},
					}
				}
				sts.Spec.Template.Spec.Containers[1].VolumeMounts = []corev1.VolumeMount{
					{Name: common.VolumeNameBackupRestoreServerTLS, MountPath: common.VolumeMountPathBackupRestoreServerTLS},
					{Name: common.VolumeNameEtcdCA, MountPath: common.VolumeMountPathEtcdCA},
					{Name: common.VolumeNameEtcdClientTLS, MountPath: common.VolumeMountPathEtcdClientTLS},
				}
				g.Expect(GetStatefulSetContainerTLSVolumeMounts(sts)).To(Equal(tc.expectedVolumeMounts))
			}
		})
	}
}

func TestGetSecretNameFromVolume(t *testing.T) {
	const etcdbrCASecretName = "ca-etcdbr"
	stsWithCAVolume := testutils.AddBackupRestoreCAVolume(
		testutils.CreateStatefulSet("test-sts", "test-ns", uuid.NewUUID(), 1),
		etcdbrCASecretName,
	)
	stsWithoutCAVolume := testutils.CreateStatefulSet("test-sts", "test-ns", uuid.NewUUID(), 1)
	stsWithEmptyDirNamedAsCAVolume := testutils.AddEmptyDirVolume(
		testutils.CreateStatefulSet("test-sts", "test-ns", uuid.NewUUID(), 1),
		common.VolumeNameBackupRestoreCA,
	)

	testCases := []struct {
		name          string
		sts           *appsv1.StatefulSet
		volumeName    string
		expectedName  string
		expectedFound bool
	}{
		{
			name:          "sts is nil",
			sts:           nil,
			volumeName:    common.VolumeNameBackupRestoreCA,
			expectedName:  "",
			expectedFound: false,
		},
		{
			name:          "volume not present on sts",
			sts:           stsWithoutCAVolume,
			volumeName:    common.VolumeNameBackupRestoreCA,
			expectedName:  "",
			expectedFound: false,
		},
		{
			name:          "secret-typed volume found, returns secret name",
			sts:           stsWithCAVolume,
			volumeName:    common.VolumeNameBackupRestoreCA,
			expectedName:  etcdbrCASecretName,
			expectedFound: true,
		},
		{
			name:          "volume name matches but volume source is not a secret",
			sts:           stsWithEmptyDirNamedAsCAVolume,
			volumeName:    common.VolumeNameBackupRestoreCA,
			expectedName:  "",
			expectedFound: false,
		},
		{
			name:          "different volume name queried",
			sts:           stsWithCAVolume,
			volumeName:    "nonexistent",
			expectedName:  "",
			expectedFound: false,
		},
	}

	g := NewWithT(t)
	t.Parallel()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			name, found := GetSecretNameFromVolume(tc.sts, tc.volumeName)
			g.Expect(found).To(Equal(tc.expectedFound))
			g.Expect(name).To(Equal(tc.expectedName))
		})
	}
}

func TestIsStatefulSetReady_OnDelete(t *testing.T) {
	tests := []struct {
		name                 string
		buildObjects         func() (*appsv1.StatefulSet, []client.Object)
		expectedReady        bool
		expectedReasonSubstr string
	}{
		{
			name: "OnDelete: all pods at target revision -> ready",
			buildObjects: func() (*appsv1.StatefulSet, []client.Object) {
				sts := onDeleteSTS("rev-2", 3)
				pods := []client.Object{
					stsPod("p-0", "rev-2", sts.Spec.Selector.MatchLabels),
					stsPod("p-1", "rev-2", sts.Spec.Selector.MatchLabels),
					stsPod("p-2", "rev-2", sts.Spec.Selector.MatchLabels),
				}
				return sts, pods
			},
			expectedReady: true,
		},
		{
			name: "OnDelete: one pod at older revision -> not ready with pod-name reason",
			buildObjects: func() (*appsv1.StatefulSet, []client.Object) {
				sts := onDeleteSTS("rev-2", 3)
				pods := []client.Object{
					stsPod("p-0", "rev-2", sts.Spec.Selector.MatchLabels),
					stsPod("p-1", "rev-1", sts.Spec.Selector.MatchLabels), // outdated
					stsPod("p-2", "rev-2", sts.Spec.Selector.MatchLabels),
				}
				return sts, pods
			},
			expectedReady:        false,
			expectedReasonSubstr: "p-1",
		},
		{
			name: "OnDelete: works even when status fields (currentRevision, currentReplicas) are stale",
			buildObjects: func() (*appsv1.StatefulSet, []client.Object) {
				sts := onDeleteSTS("rev-2", 3)
				// Emulate the pre-K8s-v1.37 bug: currentRevision != updateRevision even after rollout.
				sts.Status.CurrentRevision = "rev-1"
				sts.Status.CurrentReplicas = 0
				sts.Status.UpdatedReplicas = 0
				pods := []client.Object{
					stsPod("p-0", "rev-2", sts.Spec.Selector.MatchLabels),
					stsPod("p-1", "rev-2", sts.Spec.Selector.MatchLabels),
					stsPod("p-2", "rev-2", sts.Spec.Selector.MatchLabels),
				}
				return sts, pods
			},
			expectedReady: true,
		},
	}

	g := NewWithT(t)
	t.Parallel()
	for _, tc := range tests {
		t.Run(tc.name, func(_ *testing.T) {
			sts, objs := tc.buildObjects()
			cl := testutils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).WithObjects(objs...).Build()
			ready, reason := IsStatefulSetReady(context.Background(), cl, *sts.Spec.Replicas, sts)
			g.Expect(ready).To(Equal(tc.expectedReady))
			if tc.expectedReasonSubstr != "" {
				g.Expect(reason).To(ContainSubstring(tc.expectedReasonSubstr))
			}
		})
	}
}

func TestAreAllStsPodsAtUpdateRevision(t *testing.T) {
	g := NewWithT(t)
	t.Parallel()

	t.Run("empty updateRevision -> (false, empty, no error)", func(_ *testing.T) {
		sts := onDeleteSTS("", 3)
		cl := testutils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).Build()
		all, name, err := AreAllStsPodsAtUpdateRevision(context.Background(), cl, sts)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(all).To(BeFalse())
		g.Expect(name).To(BeEmpty())
	})

	t.Run("nil selector -> error", func(_ *testing.T) {
		sts := onDeleteSTS("rev-2", 3)
		sts.Spec.Selector = nil
		cl := testutils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).Build()
		_, _, err := AreAllStsPodsAtUpdateRevision(context.Background(), cl, sts)
		g.Expect(err).To(HaveOccurred())
	})

	t.Run("returns first mismatching pod's name", func(_ *testing.T) {
		sts := onDeleteSTS("rev-2", 3)
		objs := []client.Object{
			stsPod("p-a", "rev-2", sts.Spec.Selector.MatchLabels),
			stsPod("p-b", "rev-1", sts.Spec.Selector.MatchLabels),
		}
		cl := testutils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).WithObjects(objs...).Build()
		all, name, err := AreAllStsPodsAtUpdateRevision(context.Background(), cl, sts)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(all).To(BeFalse())
		g.Expect(name).To(Equal("p-b"))
	})

	t.Run("terminating pods are excluded from the check", func(_ *testing.T) {
		sts := onDeleteSTS("rev-2", 3)
		// Only a terminating outdated pod is present; the answer should be true.
		terminating := stsPod("p-terminating", "rev-1", sts.Spec.Selector.MatchLabels)
		now := metav1.Now()
		terminating.DeletionTimestamp = &now
		terminating.Finalizers = []string{"kubernetes"}
		cl := testutils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).WithObjects(terminating).Build()
		all, name, err := AreAllStsPodsAtUpdateRevision(context.Background(), cl, sts)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(all).To(BeTrue())
		g.Expect(name).To(BeEmpty())
	})

	t.Run("empty pod set -> all-at-revision is true", func(_ *testing.T) {
		sts := onDeleteSTS("rev-2", 3)
		cl := testutils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).Build()
		all, name, err := AreAllStsPodsAtUpdateRevision(context.Background(), cl, sts)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(all).To(BeTrue())
		g.Expect(name).To(BeEmpty())
	})
}

func onDeleteSTS(updateRevision string, replicas int32) *appsv1.StatefulSet {
	sts := testutils.CreateStatefulSet(stsName, stsNamespace, "some-uid", replicas)
	sts.Spec.UpdateStrategy.Type = appsv1.OnDeleteStatefulSetStrategyType
	sts.Status.ObservedGeneration = sts.Generation
	sts.Status.ReadyReplicas = replicas
	sts.Status.UpdateRevision = updateRevision
	return sts
}

func stsPod(name, revision string, selector map[string]string) *corev1.Pod {
	labels := map[string]string{}
	maps.Copy(labels, selector)
	labels[appsv1.StatefulSetRevisionLabel] = revision
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: stsNamespace, Labels: labels},
	}
}
