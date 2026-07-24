// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

// Testing validations of etcd.spec.backup fields.

package etcd

import (
	"testing"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/test/utils"
)

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
			validateEtcdCreation(g, etcd, test.expectErr)
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
			validateEtcdCreation(g, etcd, test.expectErr)
		})
	}
}

// TestValidateSpecBackupStoreContainer validates the pattern/maxLength constraints on the
// etcd.spec.backup.store.container field. The field flows into a root chown init container, so
// it must reject shell metacharacters and path separators.
func TestValidateSpecBackupStoreContainer(t *testing.T) {
	tests := []struct {
		name      string
		etcdName  string
		container string
		expectErr bool
	}{
		{"valid: simple name", "etcd-c-1", "etcd-bucket", false},
		{"valid: with dot", "etcd-c-2", "default.bkp", false},
		{"valid: with hyphens", "etcd-c-3", "object-storage-container-name", false},
		{"valid: three-char minimum", "etcd-c-4", "foo", false},
		{"invalid: shell injection", "etcd-c-5", "x; id > /tmp/pwned; #", true},
		{"invalid: contains space", "etcd-c-6", "a b", true},
		{"invalid: leading hyphen", "etcd-c-7", "-rf", true},
		{"invalid: path traversal", "etcd-c-8", "../etc", true},
		{"invalid: command substitution", "etcd-c-9", "$(whoami)", true},
		{"invalid: trailing hyphen", "etcd-c-10", "foo-", true},
		{"invalid: trailing dot", "etcd-c-11", "foo.", true},
		{"invalid: below minimum length", "etcd-c-12", "ab", true},
	}

	testNs, g := setupTestEnvironment(t)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			etcd := utils.EtcdBuilderWithoutDefaults(test.etcdName, testNs).WithReplicas(3).Build()
			etcd.Spec.Backup.Store = &druidv1alpha1.StoreSpec{
				Prefix:    test.etcdName,
				Container: &test.container,
			}
			validateEtcdCreation(g, etcd, test.expectErr)
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

			validateEtcdCreation(g, etcd, test.expectErr)
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
			validateEtcdCreation(g, etcd, test.expectErr)
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

			validateEtcdCreation(g, etcd, test.expectErr)
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

			validateEtcdCreation(g, etcd, test.expectErr)
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

			validateEtcdCreation(g, etcd, test.expectErr)
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

			validateEtcdCreation(g, etcd, test.expectErr)
		})
	}
}

// Checks for valid duration values passed to etcd.spec.backup.garbageCollectionPeriod and etcd.spec.backup.deltaSnapshotPeriod, the value of GarbageCollectionPolicy is greater than the DeltaSnapshotPeriod
func TestValidateSpecBackupGCDeltaSnapshotPeriodRelation(t *testing.T) {
	skipCELTestsForOlderK8sVersions(t)
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

			validateEtcdCreation(g, etcd, test.expectErr)

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

			validateEtcdCreation(g, etcd, test.expectErr)
		})
	}
}
