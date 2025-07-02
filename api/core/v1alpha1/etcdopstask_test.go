package v1alpha1_test

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	. "github.com/gardener/etcd-druid/api/core/v1alpha1"
	. "github.com/onsi/gomega"
)

func TestGetTimeToExpiryAndHasTTLExpired(t *testing.T) {
	g := NewWithT(t)
	now := time.Now().UTC()

	tests := []struct {
		name    string
		task    *EtcdOpsTask
		expired bool
	}{
		{
			name: "unfinished before TTL",
			task: &EtcdOpsTask{
				ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(now.Add(-30 * time.Second))},
				Spec:       EtcdOpsTaskSpec{TTLSecondsAfterFinished: ptr.To(int32(60))},
			},
			expired: false,
		},
		{
			name: "finished before TTL",
			task: &EtcdOpsTask{
				ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(now)},
				Spec:       EtcdOpsTaskSpec{TTLSecondsAfterFinished: ptr.To(int32(60))},
				Status: EtcdOpsTaskStatus{
					State:              ptr.To(TaskStateSucceeded),
					LastTransitionTime: &metav1.Time{Time: now.Add(-30 * time.Second)},
				},
			},
			expired: false,
		},
		{
			name: "unfinished TTL expired",
			task: &EtcdOpsTask{
				ObjectMeta: metav1.ObjectMeta{CreationTimestamp: metav1.NewTime(now.Add(-120 * time.Second))},
				Spec:       EtcdOpsTaskSpec{TTLSecondsAfterFinished: ptr.To(int32(60))},
			},
			expired: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			remaining := tc.task.GetTimeToExpiry()
			if tc.expired {
				g.Expect(remaining).To(Equal(time.Duration(0)))
				g.Expect(tc.task.HasTTLExpired()).To(BeTrue())
			} else {
				g.Expect(remaining).To(BeNumerically(">", 0))
				g.Expect(tc.task.HasTTLExpired()).To(BeFalse())
			}
		})
	}
}
