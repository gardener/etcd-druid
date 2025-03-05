// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package etcd

import (
	"testing"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/test/utils"
)

// tests for etcd.spec.backup fields:

// TestValidateGarbageCollectionPolicy tests the validation of `Spec.Backup.GarbageCollectionPolicy` field in the Etcd resource.
func TestValidateSpecBackupGarbageCollectionPolicy(t *testing.T) {
	tests := []struct {
		name                    string
		etcdName                string
		garbageCollectionPolicy string
		expectErr               bool
	}{
		{"valid garbage collection policy (Exponential)", "etcd1", "Exponential", false},
		{"valid garbage collection policy (LimitBased)", "etcd2", "LimitBased", false},
		{"invalid garbage collection policy", "etcd3", "Invalid", true},
		{"empty garbage collection policy", "etcd4", "", true},
	}

	testNs, g := setupTestEnvironment(t)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			etcd := utils.EtcdBuilderWithoutDefaults(test.etcdName, testNs).WithReplicas(3).Build()
			etcd.Spec.Backup.GarbageCollectionPolicy = (*druidv1alpha1.GarbageCollectionPolicy)(&test.garbageCollectionPolicy)
			validateEtcdCreation(t, g, etcd, test.expectErr)
		})
	}
}

// runs validation on the field etcd.spec.backup.compression.policy. Accepted values: gzip, lzw, zlib
func TestValidateSpecBackupCompressionPolicy(t *testing.T) {
	tests := []struct {
		name      string
		etcdName  string
		policy    string
		expectErr bool
	}{
		{
			name:      "Valid compression Policy #1: gzip",
			etcdName:  "etcd-valid-1",
			policy:    "gzip",
			expectErr: false,
		},
		{
			name:      "Valid compression Policy #2: lzw",
			etcdName:  "etcd-valid-2",
			policy:    "lzw",
			expectErr: false,
		},
		{
			name:      "Valid compression Policy #3: zlib",
			etcdName:  "etcd-valid-3",
			policy:    "zlib",
			expectErr: false,
		},
		{
			name:      "Invalid compression Policy #1: invalid value",
			etcdName:  "etcd-invalid-1",
			policy:    "7zip",
			expectErr: true,
		},
		{
			name:      "Invalid compression Policy #2: empty string",
			etcdName:  "etcd-invalid-2",
			policy:    "",
			expectErr: true,
		},
	}

	testNs, g := setupTestEnvironment(t)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			etcd := utils.EtcdBuilderWithoutDefaults(test.etcdName, testNs).WithReplicas(3).Build()
			etcd.Spec.Backup.SnapshotCompression = &druidv1alpha1.CompressionSpec{}
			etcd.Spec.Backup.SnapshotCompression.Policy = (*druidv1alpha1.CompressionPolicy)(&test.policy)
			validateEtcdCreation(t, g, etcd, test.expectErr)
		})
	}
}

// validates the duration passed into the etcd.spec.backup.deltaSnapshotRetentionPeriod field.
func TestValidateSpecBackupDeltaSnapshotRetentionPeriod(t *testing.T) {
	testNs, g := setupTestEnvironment(t)

	for _, test := range durationFieldTestCases {
		t.Run(test.name, func(t *testing.T) {
			duration, shouldContinue := validateDuration(t, test.value, test.expectErr)
			if !shouldContinue {
				return
			}

			etcd := utils.EtcdBuilderWithoutDefaults(test.etcdName, testNs).WithReplicas(3).Build()
			etcd.Spec.Backup.DeltaSnapshotRetentionPeriod = duration

			validateEtcdCreation(t, g, etcd, test.expectErr)
		})
	}
}

// validates the duration passed into the etcd.spec.backup.etcdSnapshotTimeout field.
func TestValidateSpecBackupEtcdSnapshotTimeout(t *testing.T) {
	testNs, g := setupTestEnvironment(t)

	for _, test := range durationFieldTestCases {
		t.Run(test.name, func(t *testing.T) {
			duration, shouldContinue := validateDuration(t, test.value, test.expectErr)
			if !shouldContinue {
				return
			}
			etcd := utils.EtcdBuilderWithoutDefaults(test.etcdName, testNs).WithReplicas(3).Build()
			etcd.Spec.Backup.EtcdSnapshotTimeout = duration
			validateEtcdCreation(t, g, etcd, test.expectErr)
		})
	}
}

// validates the duration passed into the etcd.spec.backup.leaderElection.reelectionPeriod field.
func TestValidateSpecBackupLeaderElectionReelectionPeriod(t *testing.T) {
	testNs, g := setupTestEnvironment(t)

	for _, test := range durationFieldTestCases {
		t.Run(test.name, func(t *testing.T) {
			duration, shouldContinue := validateDuration(t, test.value, test.expectErr)
			if !shouldContinue {
				return
			}
			etcd := utils.EtcdBuilderWithoutDefaults(test.etcdName, testNs).WithReplicas(3).Build()
			etcd.Spec.Backup.LeaderElection = &druidv1alpha1.LeaderElectionSpec{}
			etcd.Spec.Backup.LeaderElection.ReelectionPeriod = duration

			validateEtcdCreation(t, g, etcd, test.expectErr)
		})
	}
}

// validates the duration passed into the etcd.spec.backup.leaderElection.etcdConnectionTimeout field.
func TestValidateSpecBackupLeaderElectionEtcdConnectionTimeout(t *testing.T) {
	testNs, g := setupTestEnvironment(t)

	for _, test := range durationFieldTestCases {
		t.Run(test.name, func(t *testing.T) {
			duration, shouldContinue := validateDuration(t, test.value, test.expectErr)
			if !shouldContinue {
				return
			}
			etcd := utils.EtcdBuilderWithoutDefaults(test.etcdName, testNs).WithReplicas(3).Build()
			etcd.Spec.Backup.LeaderElection = &druidv1alpha1.LeaderElectionSpec{}
			etcd.Spec.Backup.LeaderElection.EtcdConnectionTimeout = duration

			validateEtcdCreation(t, g, etcd, test.expectErr)
		})
	}
}

// validates the duration passed into the etcd.spec.backup.garbageCollectionPeriod field.
func TestValidateSpecBackupGarbageCollectionPeriod(t *testing.T) {
	testNs, g := setupTestEnvironment(t)

	for _, test := range durationFieldTestCases {
		t.Run(test.name, func(t *testing.T) {
			duration, shouldContinue := validateDuration(t, test.value, test.expectErr)
			if !shouldContinue {
				return
			}
			etcd := utils.EtcdBuilderWithoutDefaults(test.etcdName, testNs).WithReplicas(3).Build()
			etcd.Spec.Backup.GarbageCollectionPeriod = duration

			validateEtcdCreation(t, g, etcd, test.expectErr)
		})
	}
}

// validates the duration passed into the etcd.spec.backup.deltaSnapshotPeriod field.
func TestValidateSpecBackupDeltaSnapshotPeriod(t *testing.T) {
	testNs, g := setupTestEnvironment(t)

	for _, test := range durationFieldTestCases {
		t.Run(test.name, func(t *testing.T) {
			duration, shouldContinue := validateDuration(t, test.value, test.expectErr)
			if !shouldContinue {
				return
			}
			etcd := utils.EtcdBuilderWithoutDefaults(test.etcdName, testNs).WithReplicas(3).Build()
			etcd.Spec.Backup.DeltaSnapshotPeriod = duration

			validateEtcdCreation(t, g, etcd, test.expectErr)
		})
	}
}

// Checks for valid duration values passed to etcd.spec.backup.garbageCollectionPeriod and etcd.spec.backup.deltaSnapshotPeriod, the value of GarbageCollectionPolicy is greater than the DeltaSnapshotPeriod
func TestValidateSpecBackupGCDeltaSnapshotPeriodRelation(t *testing.T) {
	skipTestBasedOnK8sVersion(t)
	tests := []struct {
		name                string
		etcdName            string
		gcPeriod            string
		deltaSnapshotPeriod string
		expectErr           bool
	}{
		{
			name:                "Valid durations passed; gcperiod > deltaSnapshotPeriod; valid",
			etcdName:            "etcd-valid-1",
			gcPeriod:            "10m5s",
			deltaSnapshotPeriod: "5m12s",
			expectErr:           false,
		},
		{
			name:                "Valid durations passed; gcperiod < deltaSnapshotPeriod; invalid",
			etcdName:            "etcd-invalid-1",
			gcPeriod:            "1m5s",
			deltaSnapshotPeriod: "5m12s",
			expectErr:           true,
		},
		{
			name:                "Invalid durations passed",
			etcdName:            "etcd-invalid-2",
			gcPeriod:            "10m5sddd",
			deltaSnapshotPeriod: "5m12s",
			expectErr:           true,
		},
	}

	testNs, g := setupTestEnvironment(t)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			garbageCollectionDuration, shouldContinue := validateDuration(t, test.gcPeriod, test.expectErr)
			if !shouldContinue {
				return
			}

			deltaSnapshotDuration, shouldContinue := validateDuration(t, test.deltaSnapshotPeriod, test.expectErr)
			if !shouldContinue {
				return
			}
			etcd := utils.EtcdBuilderWithoutDefaults(test.etcdName, testNs).WithReplicas(3).Build()
			etcd.Spec.Backup.GarbageCollectionPeriod = garbageCollectionDuration
			etcd.Spec.Backup.DeltaSnapshotPeriod = deltaSnapshotDuration

			validateEtcdCreation(t, g, etcd, test.expectErr)

		})
	}
}

// validates the cron expression passed into the etcd.spec.backup.fullSnapshotSchedule field.
func TestValidateSpecBackupFullSnapshotSchedule(t *testing.T) {
	testNs, g := setupTestEnvironment(t)

	for _, test := range cronFieldTestCases {
		t.Run(test.name, func(t *testing.T) {
			etcd := utils.EtcdBuilderWithoutDefaults(test.etcdName, testNs).WithReplicas(3).Build()
			etcd.Spec.Backup.FullSnapshotSchedule = &test.value

			validateEtcdCreation(t, g, etcd, test.expectErr)
		})
	}
}
