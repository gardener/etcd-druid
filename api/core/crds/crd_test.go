package crds

import (
	. "github.com/onsi/gomega"
	"testing"
)

func TestGetAll(t *testing.T) {
	testCases := []struct {
		name         string
		k8sVersion   string
		expectedCRDs map[string]string
	}{
		{
			name:       "k8s version is 1.29",
			k8sVersion: "1.29",
			expectedCRDs: map[string]string{
				KindEtcd:                etcdCRD,
				KindEtcdCopyBackupsTask: etcdCopyBackupsTaskCRD,
			},
		},
		{
			name:       "k8s version is below 1.29",
			k8sVersion: "1.28",
			expectedCRDs: map[string]string{
				KindEtcd:                etcdCRDWithoutCEL,
				KindEtcdCopyBackupsTask: etcdCopyBackupsTaskCRD,
			},
		},
		{
			name:       "k8s version is above 1.29",
			k8sVersion: "1.30",
			expectedCRDs: map[string]string{
				KindEtcd:                etcdCRD,
				KindEtcdCopyBackupsTask: etcdCopyBackupsTaskCRD,
			},
		},
	}

	g := NewWithT(t)
	t.Parallel()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			crds, err := GetAll(tc.k8sVersion)
			g.Expect(err).To(BeNil())
			g.Expect(crds).To(HaveLen(len(tc.expectedCRDs)))
			g.Expect(crds).To(Equal(tc.expectedCRDs))
		})
	}

}
