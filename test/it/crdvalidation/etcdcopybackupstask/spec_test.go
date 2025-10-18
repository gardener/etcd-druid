// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package etcdcopybackupstask

import (
	"testing"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/test/utils"

	. "github.com/onsi/gomega"
)

// testValidateSpecWaitForFinalSnapshotTimeout tests the validation of the `spec.waitForFinalSnapshot.timeout` field
// of the EtcdCopyBackupsTask CR.
func TestValidateSpecWaitForFinalSnapshotTimeout(t *testing.T) {
	testNs, g := setupTestEnvironment(t)
	var tests = []struct {
		name      string
		taskName  string
		value     string
		expectErr bool
	}{
		{
			name:      "valid duration #1",
			taskName:  "task-valid-1",
			value:     "1h2m3s",
			expectErr: false,
		},
		{
			name:      "valid duration #2",
			taskName:  "task-valid-2",
			value:     "3m735s",
			expectErr: false,
		},
		{
			name:      "valid duration #3",
			taskName:  "task-valid-3",
			value:     "0",
			expectErr: false,
		},
		{
			name:      "valid duration #4",
			taskName:  "task-valid-4",
			value:     "1.5h",
			expectErr: false,
		},
		{
			name:      "valid duration #5",
			taskName:  "task-valid-5",
			value:     "10ms",
			expectErr: false,
		},
		{
			name:      "invalid duration #1",
			taskName:  "task-invalid-1",
			value:     "hms",
			expectErr: true,
		},
		{
			name:      "invalid duration #2",
			taskName:  "task-invalid-2",
			value:     "-10h2m3s",
			expectErr: true,
		},
		{
			name:      "invalid duration #3",
			taskName:  "task-invalid-3",
			value:     "10d",
			expectErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			copyBackupTask := utils.CreateEtcdCopyBackupsTask(test.taskName, testNs, "aws", true)
			patchedCopyBackupTask, err := patchObject(copyBackupTask, []string{
				"spec", "waitForFinalSnapshot", "timeout",
			}, test.value)
			g.Expect(err).ToNot(HaveOccurred(), "failed to patch object: %v", err)
			validatePatchedObjectCreation[*druidv1alpha1.EtcdCopyBackupsTask](g, patchedCopyBackupTask, test.expectErr)
		})
	}
}

// testValidateSpecMaxBackupAge tests the validation of the `spec.maxBackupAge` field of the EtcdCopyBackupsTask CR.
func TestValidateSpecMaxBackupAge(t *testing.T) {
	testNs, g := setupTestEnvironment(t)
	var tests = []struct {
		name      string
		taskName  string
		value     int
		expectErr bool
	}{
		{
			name:      "valid age #1",
			taskName:  "task-valid-1",
			value:     1,
			expectErr: false,
		},
		{
			name:      "valid age #2",
			taskName:  "task-valid-2",
			value:     0,
			expectErr: false,
		},
		{
			name:      "invalid age #1",
			taskName:  "task-invalid-1",
			value:     -1,
			expectErr: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			copyBackupTask := utils.CreateEtcdCopyBackupsTask(test.taskName, testNs, "aws", true)
			patchedCopyBackupTask, err := patchObject(copyBackupTask, []string{
				"spec", "maxBackupAge",
			}, test.value)
			g.Expect(err).ToNot(HaveOccurred(), "failed to patch object: %v", err)
			validatePatchedObjectCreation[*druidv1alpha1.EtcdCopyBackupsTask](g, patchedCopyBackupTask, test.expectErr)
		})
	}
}

// testValidateSpecMaxBackups tests the validation of the `spec.maxBackups` field of the EtcdCopyBackupsTask CR.
func TestValidateSpecMaxBackups(t *testing.T) {
	testNs, g := setupTestEnvironment(t)
	var tests = []struct {
		name      string
		taskName  string
		value     int
		expectErr bool
	}{
		{
			name:      "valid max backups #1",
			taskName:  "task-valid-1",
			value:     10,
			expectErr: false,
		},
		{
			name:      "valid max backups #2",
			taskName:  "task-valid-2",
			value:     0,
			expectErr: false,
		},
		{
			name:      "invalid max backups #1",
			taskName:  "task-invalid-1",
			value:     -10,
			expectErr: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			copyBackupTask := utils.CreateEtcdCopyBackupsTask(test.taskName, testNs, "aws", true)
			patchedCopyBackupTask, err := patchObject(copyBackupTask, []string{
				"spec", "maxBackups",
			}, test.value)
			g.Expect(err).ToNot(HaveOccurred(), "failed to patch object: %v", err)
			validatePatchedObjectCreation[*druidv1alpha1.EtcdCopyBackupsTask](g, patchedCopyBackupTask, test.expectErr)
		})
	}
}
