// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package compaction

import (
	"context"
	"testing"
	"time"

	"github.com/gardener/etcd-druid/internal/utils"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/gomega"
	testutils "github.com/gardener/etcd-druid/test/utils"
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
					Reason: podFailureReasonPreemptionByScheduler,
					LastTransitionTime: metav1.Time{
						Time: time.Now().Add(-time.Hour),
					},
				},
			},
			expectedReason:         podFailureReasonPreemptionByScheduler,
			expectedTransitionTime: time.Now().Add(-time.Hour),
		},
		{
			name: "Pod has DisruptionTarget condition with reason DeletionByTaintManager",
			podConditions: []corev1.PodCondition{
				{
					Type:   corev1.DisruptionTarget,
					Status: corev1.ConditionTrue,
					Reason: podFailureReasonDeletionByTaintManager,
					LastTransitionTime: metav1.Time{
						Time: time.Now().Add(-2 * time.Hour),
					},
				},
			},
			expectedReason:         podFailureReasonDeletionByTaintManager,
			expectedTransitionTime: time.Now().Add(-2 * time.Hour),
		},
		{
			name: "Pod has DisruptionTarget condition with reason EvictionByEvictionAPI",
			podConditions: []corev1.PodCondition{
				{
					Type:   corev1.DisruptionTarget,
					Status: corev1.ConditionTrue,
					Reason: podFailureReasonEvictionByEvictionAPI,
					LastTransitionTime: metav1.Time{
						Time: time.Now().Add(-3 * time.Hour),
					},
				},
			},
			expectedReason:         podFailureReasonEvictionByEvictionAPI,
			expectedTransitionTime: time.Now().Add(-3 * time.Hour),
		},
		{
			name: "Pod has DisruptionTarget condition with reason TerminationByKubelet",
			podConditions: []corev1.PodCondition{
				{
					Type:   corev1.DisruptionTarget,
					Status: corev1.ConditionTrue,
					Reason: podFailureReasonTerminationByKubelet,
					LastTransitionTime: metav1.Time{
						Time: time.Now().Add(-4 * time.Hour),
					},
				},
			},
			expectedReason:         podFailureReasonTerminationByKubelet,
			expectedTransitionTime: time.Now().Add(-4 * time.Hour),
		},
		{
			name: "Pod has no DisruptionTarget condition but terminated container with process failure",
			containerStatuses: []corev1.ContainerStatus{
				{
					State: corev1.ContainerState{
						Terminated: &corev1.ContainerStateTerminated{
							Reason: podFailureReasonProcessFailure,
							FinishedAt: metav1.Time{
								Time: time.Now().Add(-30 * time.Minute),
							},
						},
					},
				},
			},
			expectedReason:         podFailureReasonProcessFailure,
			expectedTransitionTime: time.Now().Add(-30 * time.Minute),
		},
		{
			name:                   "Pod has no relevant conditions or terminated containers",
			podConditions:          []corev1.PodCondition{},
			containerStatuses:      []corev1.ContainerStatus{},
			expectedReason:         podFailureReasonUnknown,
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
			expectedReason:         podFailureReasonUnknown,
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
