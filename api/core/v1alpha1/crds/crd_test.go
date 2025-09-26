// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package crds

import (
	"testing"

	. "github.com/onsi/gomega"
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
				ResourceNameEtcd:                etcdCRD,
				ResourceNameEtcdCopyBackupsTask: etcdCopyBackupsTaskCRD,
				ResourceNameEtcdOpsTask:         etcdOpsTaskCRD,
			},
		},
		{
			name:       "k8s version is v1.29.0",
			k8sVersion: "v1.29.0",
			expectedCRDs: map[string]string{
				ResourceNameEtcd:                etcdCRD,
				ResourceNameEtcdCopyBackupsTask: etcdCopyBackupsTaskCRD,
				ResourceNameEtcdOpsTask:         etcdOpsTaskCRD,
			},
		},
		{
			name:       "k8s version is below 1.29",
			k8sVersion: "1.28",
			expectedCRDs: map[string]string{
				ResourceNameEtcd:                etcdCRDWithoutCEL,
				ResourceNameEtcdCopyBackupsTask: etcdCopyBackupsTaskCRD,
				ResourceNameEtcdOpsTask:         etcdOpsTaskCRD,
			},
		},
		{
			name:       "k8s version is below v1.29",
			k8sVersion: "v1.28.3",
			expectedCRDs: map[string]string{
				ResourceNameEtcd:                etcdCRDWithoutCEL,
				ResourceNameEtcdCopyBackupsTask: etcdCopyBackupsTaskCRD,
				ResourceNameEtcdOpsTask:         etcdOpsTaskCRD,
			},
		},
		{
			name:       "k8s version is above 1.29",
			k8sVersion: "1.30",
			expectedCRDs: map[string]string{
				ResourceNameEtcd:                etcdCRD,
				ResourceNameEtcdCopyBackupsTask: etcdCopyBackupsTaskCRD,
				ResourceNameEtcdOpsTask:         etcdOpsTaskCRD,
			},
		},
		{
			name:       "k8s version is above v1.29",
			k8sVersion: "v1.30.1",
			expectedCRDs: map[string]string{
				ResourceNameEtcd:                etcdCRD,
				ResourceNameEtcdCopyBackupsTask: etcdCopyBackupsTaskCRD,
				ResourceNameEtcdOpsTask:         etcdOpsTaskCRD,
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
