// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package compaction

import (
	"context"
	"testing"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/utils"
	testutils "github.com/gardener/etcd-druid/test/utils"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/gomega"
)

func TestGetEtcdCompactionAnnotations(t *testing.T) {
	test1EtcdAnnotation := map[string]string{
		"dummy-annotation": "dummy",
	}

	test2EtcdAnnotation := map[string]string{
		SafeToEvictKey: "false",
	}

	g := NewWithT(t)
	compactionAnnotation := getEtcdCompactionAnnotations(utils.MergeMaps(test1EtcdAnnotation, test2EtcdAnnotation))
	g.Expect(compactionAnnotation).To(Equal(test1EtcdAnnotation))
}

func TestGetJobCompletionStateAndReason(t *testing.T) {
	tests := []struct {
		name           string
		jobConditions  []batchv1.JobCondition
		expectedStatus int
		expectedReason string
	}{
		{
			name: "Job is successful with type Complete and reason CompletionsReached",
			jobConditions: []batchv1.JobCondition{
				{
					Type:   batchv1.JobComplete,
					Status: corev1.ConditionTrue,
					Reason: batchv1.JobReasonCompletionsReached,
				},
			},
			expectedStatus: jobSucceeded,
			expectedReason: batchv1.JobReasonCompletionsReached,
		},
		{
			name: "Job is successful with type SuccessCriteriaMet and reason CompletionsReached",
			jobConditions: []batchv1.JobCondition{
				{
					Type:   batchv1.JobSuccessCriteriaMet,
					Status: corev1.ConditionTrue,
					Reason: batchv1.JobReasonCompletionsReached,
				},
			},
			expectedStatus: jobSucceeded,
			expectedReason: batchv1.JobReasonCompletionsReached,
		},
		{
			name: "Job failed with type FailureTarget and reason DeadlineExceeded",
			jobConditions: []batchv1.JobCondition{
				{
					Type:   batchv1.JobFailureTarget,
					Status: corev1.ConditionTrue,
					Reason: batchv1.JobReasonDeadlineExceeded,
				},
			},
			expectedStatus: jobFailed,
			expectedReason: batchv1.JobReasonDeadlineExceeded,
		},
		{
			name: "Job failed with type Failed and reason DeadlineExceeded",
			jobConditions: []batchv1.JobCondition{
				{
					Type:   batchv1.JobFailed,
					Status: corev1.ConditionTrue,
					Reason: batchv1.JobReasonDeadlineExceeded,
				},
			},
			expectedStatus: jobFailed,
			expectedReason: batchv1.JobReasonDeadlineExceeded,
		},
		{
			name: "Job failed with type FailureTarget and reason BackoffLimitExceeded",
			jobConditions: []batchv1.JobCondition{
				{
					Type:   batchv1.JobFailureTarget,
					Status: corev1.ConditionTrue,
					Reason: batchv1.JobReasonBackoffLimitExceeded,
				},
			},
			expectedStatus: jobFailed,
			expectedReason: batchv1.JobReasonBackoffLimitExceeded,
		},
		{
			name: "Job failed with type Failed and reason BackoffLimitExceeded",
			jobConditions: []batchv1.JobCondition{
				{
					Type:   batchv1.JobFailed,
					Status: corev1.ConditionTrue,
					Reason: batchv1.JobReasonBackoffLimitExceeded,
				},
			},
			expectedStatus: jobFailed,
			expectedReason: batchv1.JobReasonBackoffLimitExceeded,
		},
		{
			name:           "Job has no conditions",
			jobConditions:  []batchv1.JobCondition{},
			expectedStatus: jobFailed,
			expectedReason: "",
		},
		{
			name: "Job has irrelevant conditions",
			jobConditions: []batchv1.JobCondition{
				{
					Type:   "IrrelevantCondition",
					Status: corev1.ConditionTrue,
					Reason: "IrrelevantReason",
				},
			},
			expectedStatus: jobFailed,
			expectedReason: "",
		},
	}

	g := NewWithT(t)
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			job := &batchv1.Job{
				Status: batchv1.JobStatus{
					Conditions: test.jobConditions,
				},
			}
			status, reason := getJobCompletionStateAndReason(job)
			g.Expect(status).To(Equal(test.expectedStatus))
			g.Expect(reason).To(Equal(test.expectedReason))
		})
	}
}

func TestGetPodFailureReasonAndLastTransitionTime(t *testing.T) {
	tests := []struct {
		name                   string
		podConditions          []corev1.PodCondition
		containerStatuses      []corev1.ContainerStatus
		expectedReason         string
		expectedTransitionTime time.Time
	}{
		{
			name: "Pod has DisruptionTarget condition with reason PreemptionByScheduler",
			podConditions: []corev1.PodCondition{
				{
					Type:   corev1.DisruptionTarget,
					Status: corev1.ConditionTrue,
					Reason: druidv1alpha1.PodFailureReasonPreemptionByScheduler,
					LastTransitionTime: metav1.Time{
						Time: time.Now().Add(-time.Hour),
					},
				},
			},
			expectedReason:         druidv1alpha1.PodFailureReasonPreemptionByScheduler,
			expectedTransitionTime: time.Now().Add(-time.Hour),
		},
		{
			name: "Pod has DisruptionTarget condition with reason DeletionByTaintManager",
			podConditions: []corev1.PodCondition{
				{
					Type:   corev1.DisruptionTarget,
					Status: corev1.ConditionTrue,
					Reason: druidv1alpha1.PodFailureReasonDeletionByTaintManager,
					LastTransitionTime: metav1.Time{
						Time: time.Now().Add(-2 * time.Hour),
					},
				},
			},
			expectedReason:         druidv1alpha1.PodFailureReasonDeletionByTaintManager,
			expectedTransitionTime: time.Now().Add(-2 * time.Hour),
		},
		{
			name: "Pod has DisruptionTarget condition with reason EvictionByEvictionAPI",
			podConditions: []corev1.PodCondition{
				{
					Type:   corev1.DisruptionTarget,
					Status: corev1.ConditionTrue,
					Reason: druidv1alpha1.PodFailureReasonEvictionByEvictionAPI,
					LastTransitionTime: metav1.Time{
						Time: time.Now().Add(-3 * time.Hour),
					},
				},
			},
			expectedReason:         druidv1alpha1.PodFailureReasonEvictionByEvictionAPI,
			expectedTransitionTime: time.Now().Add(-3 * time.Hour),
		},
		{
			name: "Pod has DisruptionTarget condition with reason TerminationByKubelet",
			podConditions: []corev1.PodCondition{
				{
					Type:   corev1.DisruptionTarget,
					Status: corev1.ConditionTrue,
					Reason: druidv1alpha1.PodFailureReasonTerminationByKubelet,
					LastTransitionTime: metav1.Time{
						Time: time.Now().Add(-4 * time.Hour),
					},
				},
			},
			expectedReason:         druidv1alpha1.PodFailureReasonTerminationByKubelet,
			expectedTransitionTime: time.Now().Add(-4 * time.Hour),
		},
		{
			name: "Pod has no DisruptionTarget condition but terminated container with process failure",
			containerStatuses: []corev1.ContainerStatus{
				{
					State: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							Reason: druidv1alpha1.PodFailureReasonProcessFailure,
							FinishedAt: metav1.Time{
								Time: time.Now().Add(-30 * time.Minute),
							},
						},
					},
				},
			},
			expectedReason:         druidv1alpha1.PodFailureReasonProcessFailure,
			expectedTransitionTime: time.Now().Add(-30 * time.Minute),
		},
		{
			name:                   "Pod has no relevant conditions or terminated containers",
			podConditions:          []corev1.PodCondition{},
			containerStatuses:      []corev1.ContainerStatus{},
			expectedReason:         druidv1alpha1.PodFailureReasonUnknown,
			expectedTransitionTime: time.Now().UTC(),
		},
		{
			name: "Pod has irrelevant conditions and no terminated containers",
			podConditions: []corev1.PodCondition{
				{
					Type:   "IrrelevantCondition",
					Status: corev1.ConditionTrue,
					Reason: "IrrelevantReason",
					LastTransitionTime: metav1.Time{
						Time: time.Now().Add(-time.Hour),
					},
				},
			},
			containerStatuses:      []corev1.ContainerStatus{},
			expectedReason:         druidv1alpha1.PodFailureReasonUnknown,
			expectedTransitionTime: time.Now().UTC(),
		},
	}

	g := NewWithT(t)
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			pod := &corev1.Pod{
				Status: corev1.PodStatus{
					Conditions:        test.podConditions,
					ContainerStatuses: test.containerStatuses,
				},
			}
			reason, lastTransitionTime := getPodFailureReasonAndLastTransitionTime(pod)
			g.Expect(reason).To(Equal(test.expectedReason))
			g.Expect(lastTransitionTime).To(BeTemporally("~", test.expectedTransitionTime, time.Second))
		})
	}
}

func TestGetCompactionJobArgs(t *testing.T) {
	const (
		testEtcdName      = "test-etcd"
		testNamespace     = "test-ns"
		testMetricsScrape = "10s"
		testPrefix        = "test-prefix"
		testContainer     = "test-bucket"
		testEndpoint      = "http://localhost:4566"
	)

	s3Provider := druidv1alpha1.StorageProvider("aws")
	absProvider := druidv1alpha1.StorageProvider("azure")
	gcsProvider := druidv1alpha1.StorageProvider("gcp")

	tests := []struct {
		name                        string
		etcdName                    string
		namespace                   string
		metricsScrapeWait           string
		storeProvider               *druidv1alpha1.StorageProvider
		storePrefix                 string
		storeContainer              *string
		storeEndpointOverride       *string
		etcdDefragTimeout           *metav1.Duration
		etcdSnapshotTimeout         *metav1.Duration
		expectedArgsContains        []string
		expectedArgsNotContainFlags []string
	}{
		{
			name:              "basic args with S3 provider without endpoint override",
			etcdName:          testEtcdName,
			namespace:         testNamespace,
			metricsScrapeWait: testMetricsScrape,
			storeProvider:     &s3Provider,
			storePrefix:       testPrefix,
			storeContainer:    ptr.To(testContainer),
			expectedArgsContains: []string{
				"compact",
				"--data-dir=/var/etcd/data/compaction.etcd",
				"--restoration-temp-snapshots-dir=/var/etcd/data/compaction.restoration.temp",
				"--snapstore-temp-directory=/var/etcd/data/tmp",
				"--metrics-scrape-wait-duration=" + testMetricsScrape,
				"--enable-snapshot-lease-renewal=true",
				"--full-snapshot-lease-name=" + testEtcdName + "-full-snap",
				"--delta-snapshot-lease-name=" + testEtcdName + "-delta-snap",
				"--embedded-etcd-quota-bytes=8589934592",
				"--storage-provider=S3",
				"--store-prefix=" + testPrefix,
				"--store-container=" + testContainer,
			},
			expectedArgsNotContainFlags: []string{
				"--store-endpoint-override",
			},
		},
		{
			name:                  "args with S3 provider and endpoint override for localstack",
			etcdName:              testEtcdName,
			namespace:             testNamespace,
			metricsScrapeWait:     testMetricsScrape,
			storeProvider:         &s3Provider,
			storePrefix:           testPrefix,
			storeContainer:        ptr.To(testContainer),
			storeEndpointOverride: ptr.To(testEndpoint),
			expectedArgsContains: []string{
				"compact",
				"--storage-provider=S3",
				"--store-prefix=" + testPrefix,
				"--store-container=" + testContainer,
				"--store-endpoint-override=" + testEndpoint,
			},
		},
		{
			name:                  "args with ABS provider and endpoint override for azurite",
			etcdName:              testEtcdName,
			namespace:             testNamespace,
			metricsScrapeWait:     testMetricsScrape,
			storeProvider:         &absProvider,
			storePrefix:           testPrefix,
			storeContainer:        ptr.To(testContainer),
			storeEndpointOverride: ptr.To("http://localhost:10000/devstoreaccount1/"),
			expectedArgsContains: []string{
				"--storage-provider=ABS",
				"--store-endpoint-override=http://localhost:10000/devstoreaccount1/",
			},
		},
		{
			name:                  "args with GCS provider and endpoint override for fake-gcs",
			etcdName:              testEtcdName,
			namespace:             testNamespace,
			metricsScrapeWait:     testMetricsScrape,
			storeProvider:         &gcsProvider,
			storePrefix:           testPrefix,
			storeContainer:        ptr.To(testContainer),
			storeEndpointOverride: ptr.To("http://localhost:8000/storage/v1/"),
			expectedArgsContains: []string{
				"--storage-provider=GCS",
				"--store-endpoint-override=http://localhost:8000/storage/v1/",
			},
		},
		{
			name:                "args with optional etcd defrag and snapshot timeouts",
			etcdName:            testEtcdName,
			namespace:           testNamespace,
			metricsScrapeWait:   testMetricsScrape,
			storeProvider:       &s3Provider,
			storePrefix:         testPrefix,
			storeContainer:      ptr.To(testContainer),
			etcdDefragTimeout:   &metav1.Duration{Duration: 15 * time.Minute},
			etcdSnapshotTimeout: &metav1.Duration{Duration: 20 * time.Minute},
			expectedArgsContains: []string{
				"--etcd-defrag-timeout=15m0s",
				"--etcd-snapshot-timeout=20m0s",
			},
		},
		{
			name:              "args without store values",
			etcdName:          testEtcdName,
			namespace:         testNamespace,
			metricsScrapeWait: testMetricsScrape,
			storeProvider:     nil,
			expectedArgsContains: []string{
				"compact",
				"--data-dir=/var/etcd/data/compaction.etcd",
				"--enable-snapshot-lease-renewal=true",
			},
			expectedArgsNotContainFlags: []string{
				"--storage-provider",
				"--store-prefix",
				"--store-container",
				"--store-endpoint-override",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			g := NewWithT(t)

			etcd := &druidv1alpha1.Etcd{
				ObjectMeta: metav1.ObjectMeta{
					Name:      tc.etcdName,
					Namespace: tc.namespace,
				},
				Spec: druidv1alpha1.EtcdSpec{
					Etcd:   druidv1alpha1.EtcdConfig{},
					Backup: druidv1alpha1.BackupSpec{},
				},
			}

			if tc.etcdDefragTimeout != nil {
				etcd.Spec.Etcd.EtcdDefragTimeout = tc.etcdDefragTimeout
			}
			if tc.etcdSnapshotTimeout != nil {
				etcd.Spec.Backup.EtcdSnapshotTimeout = tc.etcdSnapshotTimeout
			}

			if tc.storeProvider != nil {
				etcd.Spec.Backup.Store = &druidv1alpha1.StoreSpec{
					Provider:         tc.storeProvider,
					Prefix:           tc.storePrefix,
					Container:        tc.storeContainer,
					EndpointOverride: tc.storeEndpointOverride,
				}
			}

			args := getCompactionJobArgs(etcd, tc.metricsScrapeWait)

			for _, expectedArg := range tc.expectedArgsContains {
				g.Expect(args).To(ContainElement(expectedArg), "Expected arg %q to be present", expectedArg)
			}

			for _, notExpectedArg := range tc.expectedArgsNotContainFlags {
				for _, arg := range args {
					g.Expect(arg).NotTo(HavePrefix(notExpectedArg), "Arg with prefix %q should not be present", notExpectedArg)
				}
			}
		})
	}
}

func TestGetPodForJob(t *testing.T) {
	tests := []struct {
		name                      string
		jobMeta                   metav1.ObjectMeta
		pods                      []corev1.Pod
		expectedPodNamespacedName *types.NamespacedName
		expectedError             bool
		expectedErrMsg            string
		expectedPod               *corev1.Pod
	}{
		{
			name: "Single pod has the same job name",
			jobMeta: metav1.ObjectMeta{
				Name:      "test-job",
				Namespace: "default",
			},
			pods: []corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod",
						Namespace: "default",
						Labels:    client.MatchingLabels{batchv1.JobNameLabel: "test-job"},
					},
				},
			},
			expectedPodNamespacedName: &types.NamespacedName{
				Name:      "test-pod",
				Namespace: "default",
			},
			expectedError: false,
		},
		{
			name: "No pods match the job name",
			jobMeta: metav1.ObjectMeta{
				Name:      "test-job",
				Namespace: "default",
			},
			pods:                      []corev1.Pod{},
			expectedPodNamespacedName: nil,
			expectedError:             false,
		},
		{
			name: "Multiple pods match the job name",
			jobMeta: metav1.ObjectMeta{
				Name:      "test-job",
				Namespace: "default",
			},
			pods: []corev1.Pod{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod-1",
						Namespace: "default",
						Labels:    client.MatchingLabels{batchv1.JobNameLabel: "test-job"},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pod-2",
						Namespace: "default",
						Labels:    client.MatchingLabels{batchv1.JobNameLabel: "test-job"},
					},
				},
			},
			expectedPodNamespacedName: &types.NamespacedName{
				Name:      "test-pod-1",
				Namespace: "default",
			},
			expectedError: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			g := NewWithT(t)

			objects := []client.Object{}
			for _, pod := range test.pods {
				objects = append(objects, &pod)
			}

			fakeClient := testutils.CreateTestFakeClientForObjects(nil, nil, nil, nil, objects)

			pod, err := getPodForJob(context.TODO(), fakeClient, &test.jobMeta)

			if test.expectedError {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(ContainSubstring(test.expectedErrMsg))
			} else {
				g.Expect(err).NotTo(HaveOccurred())
				if pod != nil {
					g.Expect(pod.Name).To(Equal(test.expectedPodNamespacedName.Name))
					g.Expect(pod.Namespace).To(Equal(test.expectedPodNamespacedName.Namespace))
				}
			}
		})
	}
}
