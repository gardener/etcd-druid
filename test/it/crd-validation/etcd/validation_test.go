package etcd

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/test/it/assets"
	"github.com/gardener/etcd-druid/test/it/setup"
	"github.com/gardener/etcd-druid/test/utils"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	. "github.com/onsi/gomega"
)

const testNamespacePrefix = "etcd-validation-test"

var (
	itTestEnv setup.DruidTestEnvironment
)

func TestMain(m *testing.M) {
	var (
		itTestEnvCloser setup.DruidTestEnvCloser
		err             error
	)
	itTestEnv, itTestEnvCloser, err = setup.NewDruidTestEnvironment("etcd-validation", []string{assets.GetEtcdCrdPath()})
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "failed to create integration test environment: %v\n", err)
		os.Exit(1)
	}

	// os.Exit() does not respect defer statements
	exitCode := m.Run()
	itTestEnvCloser()
	os.Exit(exitCode)
}
// initial setup for setting up namespace and test environment
func setupTestEnvironment(t *testing.T) (string, *WithT) {
    g := NewWithT(t)
    testNs := utils.GenerateTestNamespaceName(t, testNamespacePrefix)
    
    t.Logf("successfully create namespace: %s to run test => '%s'", testNs, t.Name())
    t.Log("Setting up Client")
    
    g.Expect(itTestEnv.CreateManager(utils.NewTestClientBuilder())).To(Succeed())
    g.Expect(itTestEnv.CreateTestNamespace(testNs)).To(Succeed())
    
    return testNs, g
}

// common function used by all the test cases to create an etcd resource based on the individual test case's specifications.
func validateEtcdCreation(t *testing.T, g *WithT, etcd *druidv1alpha1.Etcd, expectErr bool) {
    cl := itTestEnv.GetClient()
    ctx := context.Background()

    if expectErr {
        g.Expect(cl.Create(ctx, etcd)).NotTo(Succeed())
        return
    }
    g.Expect(cl.Create(ctx, etcd)).To(Succeed())
}

var cronFieldTestCases = []struct {
	name string
	etcdName string
	value string
	expectErr bool
} {
	{
		name: "Valid cron expression #1",
		etcdName: "etcd-valid-1",
		value: "* * * * *",
		expectErr: false,
	},
	{
		name: "Valid cron expression #2",
		etcdName: "etcd-valid-2",
		value: "0 */24 * * *",
		expectErr: false,
	},
	{
		name: "Invalid cron expression #1",
		etcdName: "etcd-invalid-1",
		value: "5 24 * * *", // hours >23
		expectErr: true,
	},
	{
		name: "Invalid cron expression #2",
		etcdName: "etcd-invalid-2",
		value: "3 */24 12 *", // missing field
		expectErr: true,
	},
}
var durationFieldTestCases = []struct {
	name string
	etcdName string
	value string
	expectErr bool
} {
	{
		name: "valid duration #1",
		etcdName: "etcd-valid-1",
		value: "1h2m3s",
		expectErr: false,
	},
	{
		name: "valid duration #2",
		etcdName: "etcd-valid-2",
    	value: "3m735s",
    	expectErr: false,
	},
	{
		name: "Invalid duration #1",
		value: "10h5m6m",
		expectErr: true,
	},
	{
		name: "invalid duration #2",
		etcdName: "etcd-invalid-1",
		value: "hms",
		expectErr: true,
	},
	{
		name: "invalid duration #3",
		etcdName: "etcd-invalid-2",
		value: "-10h2m3s",
		expectErr: true,
	},
}


// Takes in string as input and parses it to the type expected by the duration fields ie metav1.Duration
func parseDuration(value string, expectErr bool, t *testing.T) (*metav1.Duration, error) {
    if value == "" {
        return nil, nil
    }
    duration, err := time.ParseDuration(value)
    if err != nil {
        return nil, err
    }
    return &metav1.Duration{Duration: duration}, nil
}

// helper function to handle the parsing of the duration string value.
func validateDuration(t *testing.T, value string, expectErr bool) (*metav1.Duration, bool) {
	duration, err := parseDuration(value, expectErr, t)
	if expectErr {
		return nil, false
	} else if err != nil {
		return nil, false
	}
	return duration, true
}
// ------------------------------ validation tests ------------------------------

// tests for etcd.spec.etcd fields

// runs the validation on the etcd.spec.etcd.etcdDefragTimeout field.
func TestValidateSpecEtcdEtcdDefragTimeout(t *testing.T) {
	testNs, g := setupTestEnvironment(t)

	for _, test := range durationFieldTestCases {
		t.Run(test.name, func(t *testing.T){
			duration, shouldContinue := validateDuration(t, test.value, test.expectErr)
			if !shouldContinue {
			    return
			}

			etcd := &druidv1alpha1.Etcd{
				ObjectMeta: metav1.ObjectMeta{
					Name: test.etcdName,
					Namespace: testNs,
				},
				Spec: druidv1alpha1.EtcdSpec{
					Labels: map[string]string {
						"app":"etcd",
					},
					Backup: druidv1alpha1.BackupSpec{},
					Etcd : druidv1alpha1.EtcdConfig{
						EtcdDefragTimeout: duration,
					},
					Replicas: 3,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "etcd",
						},
					},
				},
				
			}
			validateEtcdCreation(t, g, etcd, test.expectErr)
		})
	}

}


// runs the validation on the etcd.spec.etcd.heartbeatDuration field.
func TestValidateSpecEtcdHeartbeatDuration(t *testing.T) {
	testNs, g := setupTestEnvironment(t)

	for _, test := range durationFieldTestCases {
		t.Run(test.name, func(t *testing.T){
			duration, shouldContinue := validateDuration(t, test.value, test.expectErr)
			if !shouldContinue {
			    return
			}

			etcd := &druidv1alpha1.Etcd{
				ObjectMeta: metav1.ObjectMeta{
					Name: test.etcdName,
					Namespace: testNs,
				},
				Spec: druidv1alpha1.EtcdSpec{
					Labels: map[string]string {
						"app":"etcd",
					},
					Backup: druidv1alpha1.BackupSpec{},
					Etcd : druidv1alpha1.EtcdConfig{
						HeartbeatDuration: duration,
					},
					Replicas: 3,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "etcd",
						},
					},
				},
				
			}
			validateEtcdCreation(t, g, etcd, test.expectErr)
		})
	}

}

// runs validation on the etcd.spec.etcd.metrics field where if the field exists, the value should be either "basic" or "extensive"
func TestValidateSpecEtcdMetrics(t *testing.T) {
	tests := []struct {
		name string
		etcdName string
		metricsValue string
		expectErr bool
	}{
		{
			name: "Valid metrics #1: basic",
			etcdName: "etcd-valid-1",
			metricsValue: "basic",
			expectErr: false,
		},
		{
			name: "Valid metrics #2: extensive",
			etcdName: "etcd-valid-2",
			metricsValue: "extensive",
			expectErr: false,
		},
		{
			name: "Invalid metrics #1: invalid value",
			etcdName: "etcd-invalid-1",
			metricsValue: "random",
			expectErr: true,
		},
		{
			name: "Invalid metrics #2: Empty string",
			etcdName: "etcd-invalid-2",
			metricsValue: "",
			expectErr: true,
		},

	}

	testNs, g := setupTestEnvironment(t)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T){
			etcd := &druidv1alpha1.Etcd{
				ObjectMeta: metav1.ObjectMeta{
					Name:      test.etcdName,
					Namespace: testNs,
				},
				Spec: druidv1alpha1.EtcdSpec{
					Backup: druidv1alpha1.BackupSpec{},
					Labels: map[string]string{
						"app":"etcd",
					},
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "etcd",
						},
					},
					Etcd: druidv1alpha1.EtcdConfig{
						Metrics: (*druidv1alpha1.MetricsLevel)(&test.metricsValue),
					},
					Replicas: 3,
				},
			}
			validateEtcdCreation(t, g, etcd, test.expectErr)
		})
	}


}

// runs validation on the cron expression passed to the field etcd.spec.etcd.defragmentationSchedule
func TestvalidateSpecEtcdDefragmentationSchedule(t * testing.T) {
	testNs, g := setupTestEnvironment(t)

	for _, test := range cronFieldTestCases {
		t.Run(test.name, func(t *testing.T){
			etcd := &druidv1alpha1.Etcd{
				ObjectMeta: metav1.ObjectMeta{
					Name:      test.etcdName,
					Namespace: testNs,
				},
				Spec: druidv1alpha1.EtcdSpec{
					Backup: druidv1alpha1.BackupSpec{},
					Labels: map[string]string{
						"app":"etcd",
					},
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "etcd",
						},
					},
					Etcd: druidv1alpha1.EtcdConfig{
						DefragmentationSchedule: &test.value,
					},
					Replicas: 3,
				},
			}

			validateEtcdCreation(t, g, etcd, test.expectErr)

		})
	}
}

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
			etcd := &druidv1alpha1.Etcd{
				ObjectMeta: metav1.ObjectMeta{
					Name:      test.etcdName,
					Namespace: testNs,
				},
				Spec: druidv1alpha1.EtcdSpec{
					Backup: druidv1alpha1.BackupSpec{
						GarbageCollectionPolicy: (*druidv1alpha1.GarbageCollectionPolicy)(&test.garbageCollectionPolicy),
					},
					Labels: map[string]string{
						"app":"etcd",
					},
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "etcd",
						},
					},
					Etcd: druidv1alpha1.EtcdConfig{},
					Replicas: 3,
				},
			}
			validateEtcdCreation(t, g, etcd, test.expectErr)
		})
	}
}

// runs validation on the field etcd.spec.backup.compression.policy. Accepted values: gzip, lzw, zlib
func TestValidateSpecBackupCompressionPolicy(t *testing.T) {
	tests := []struct {
		name string
		etcdName string
		policy string
		expectErr bool
	}{
		{
			name: "Valid compression Policy #1: gzip",
			etcdName: "etcd-valid-1",
			policy: "gzip",
			expectErr: false,
		},
		{
			name: "Valid compression Policy #2: lzw",
			etcdName: "etcd-valid-2",
			policy: "lzw",
			expectErr: false,
		},
		{
			name: "Valid compression Policy #3: zlib",
			etcdName: "etcd-valid-3",
			policy: "zlib",
			expectErr: false,
		},
		{
			name: "Invalid compression Policy #1: invalid value",
			etcdName: "etcd-invalid-1",
			policy: "7zip",
			expectErr: true,
		},
		{
			name: "Invalid compression Policy #2: empty string",
			etcdName: "etcd-invalid-2",
			policy: "",
			expectErr: true,
		},
		
	}

	testNs, g := setupTestEnvironment(t)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T){
			etcd := &druidv1alpha1.Etcd{
				ObjectMeta: metav1.ObjectMeta{
					Name:      test.etcdName,
					Namespace: testNs,
				},
				Spec: druidv1alpha1.EtcdSpec{
					Backup: druidv1alpha1.BackupSpec{
						SnapshotCompression: &druidv1alpha1.CompressionSpec{
							Policy: (*druidv1alpha1.CompressionPolicy)(&test.policy),
						},
					},
					Labels: map[string]string{
						"app":"etcd",
					},
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "etcd",
						},
					},
					Etcd: druidv1alpha1.EtcdConfig{},
					Replicas: 3,
				},
			}

			validateEtcdCreation(t, g, etcd, test.expectErr)
		})
	}
}
// validates the duration passed into the etcd.spec.backup.deltaSnapshotRetentionPeriod field.
func TestValidateSpecBackupDeltaSnapshotRetentionPeriod(t *testing.T){
	testNs, g := setupTestEnvironment(t)

	for _, test := range durationFieldTestCases {
		t.Run(test.name, func(t *testing.T){
			duration, shouldContinue := validateDuration(t, test.value, test.expectErr)
			if !shouldContinue {
			    return
			}
			etcd := &druidv1alpha1.Etcd{
				ObjectMeta: metav1.ObjectMeta{
					Name:      test.etcdName,
					Namespace: testNs,
				},
				Spec: druidv1alpha1.EtcdSpec{
					Backup: druidv1alpha1.BackupSpec{
						DeltaSnapshotRetentionPeriod: duration,
					},
					Labels: map[string]string{
						"app":"etcd",
					},
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "etcd",
						},
					},
					Etcd: druidv1alpha1.EtcdConfig{},
					Replicas: 3,
				},
			}

			validateEtcdCreation(t, g, etcd, test.expectErr)
		})
	}
}

// validates the duration passed into the etcd.spec.backup.etcdSnapshotTimeout field.
func TestValidateSpecBackupEtcdSnapshotTimeout(t *testing.T){
	testNs, g := setupTestEnvironment(t)

	for _, test := range durationFieldTestCases {
		t.Run(test.name, func(t *testing.T){
			duration, shouldContinue := validateDuration(t, test.value, test.expectErr)
			if !shouldContinue {
			    return
			}
			etcd := &druidv1alpha1.Etcd{
				ObjectMeta: metav1.ObjectMeta{
					Name:      test.etcdName,
					Namespace: testNs,
				},
				Spec: druidv1alpha1.EtcdSpec{
					Backup: druidv1alpha1.BackupSpec{
						EtcdSnapshotTimeout: duration,
					},
					Labels: map[string]string{
						"app":"etcd",
					},
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "etcd",
						},
					},
					Etcd: druidv1alpha1.EtcdConfig{},
					Replicas: 3,
				},
			}

			validateEtcdCreation(t, g, etcd, test.expectErr)
		})
	}
}

// validates the duration passed into the etcd.spec.backup.leaderElection.reelectionPeriod field.
func TestValidateSpecBackupLeaderElectionReelectionPeriod(t *testing.T){
	testNs, g := setupTestEnvironment(t)

	for _, test := range durationFieldTestCases {
		t.Run(test.name, func(t *testing.T){
			duration, shouldContinue := validateDuration(t, test.value, test.expectErr)
			if !shouldContinue {
			    return
			}
			etcd := &druidv1alpha1.Etcd{
				ObjectMeta: metav1.ObjectMeta{
					Name:      test.etcdName,
					Namespace: testNs,
				},
				Spec: druidv1alpha1.EtcdSpec{
					Backup: druidv1alpha1.BackupSpec{
						LeaderElection: &druidv1alpha1.LeaderElectionSpec{
							ReelectionPeriod: duration,
						},
					},
					Labels: map[string]string{
						"app":"etcd",
					},
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "etcd",
						},
					},
					Etcd: druidv1alpha1.EtcdConfig{},
					Replicas: 3,
				},
			}

			validateEtcdCreation(t, g, etcd, test.expectErr)
		})
	}
}

// validates the duration passed into the etcd.spec.backup.leaderElection.etcdConnectionTimeout field.
func TestValidateSpecBackupLeaderElectionEtcdConnectionTimeout(t *testing.T){
	testNs, g := setupTestEnvironment(t)

	for _, test := range durationFieldTestCases {
		t.Run(test.name, func(t *testing.T){
			duration, shouldContinue := validateDuration(t, test.value, test.expectErr)
			if !shouldContinue {
			    return
			}
			etcd := &druidv1alpha1.Etcd{
				ObjectMeta: metav1.ObjectMeta{
					Name:      test.etcdName,
					Namespace: testNs,
				},
				Spec: druidv1alpha1.EtcdSpec{
					Backup: druidv1alpha1.BackupSpec{
						LeaderElection: &druidv1alpha1.LeaderElectionSpec{
							EtcdConnectionTimeout: duration,
						},
					},
					Labels: map[string]string{
						"app":"etcd",
					},
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "etcd",
						},
					},
					Etcd: druidv1alpha1.EtcdConfig{},
					Replicas: 3,
				},
			}

			validateEtcdCreation(t, g, etcd, test.expectErr)
		})
	}
}

// validates the duration passed into the etcd.spec.backup.garbageCollectionPeriod field.
func TestValidateSpecBackupGarbageCollectionPeriod(t *testing.T){
	testNs, g := setupTestEnvironment(t)

	for _, test := range durationFieldTestCases {
		t.Run(test.name, func(t *testing.T){
			duration, shouldContinue := validateDuration(t, test.value, test.expectErr)
			if !shouldContinue {
			    return
			}
			etcd := &druidv1alpha1.Etcd{
				ObjectMeta: metav1.ObjectMeta{
					Name:      test.etcdName,
					Namespace: testNs,
				},
				Spec: druidv1alpha1.EtcdSpec{
					Backup: druidv1alpha1.BackupSpec{
						GarbageCollectionPeriod: duration,
					},
					Labels: map[string]string{
						"app":"etcd",
					},
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "etcd",
						},
					},
					Etcd: druidv1alpha1.EtcdConfig{},
					Replicas: 3,
				},
			}

			validateEtcdCreation(t, g, etcd, test.expectErr)
		})
	}
}

// validates the duration passed into the etcd.spec.backup.deltaSnapshotPeriod field.
func TestValidateSpecBackupDeltaSnapshotPeriod(t *testing.T){
	testNs, g := setupTestEnvironment(t)

	for _, test := range durationFieldTestCases {
		t.Run(test.name, func(t *testing.T){
			duration, shouldContinue := validateDuration(t, test.value, test.expectErr)
			if !shouldContinue {
			    return
			}
			etcd := &druidv1alpha1.Etcd{
				ObjectMeta: metav1.ObjectMeta{
					Name:      test.etcdName,
					Namespace: testNs,
				},
				Spec: druidv1alpha1.EtcdSpec{
					Backup: druidv1alpha1.BackupSpec{
						DeltaSnapshotPeriod: duration,
					},
					Labels: map[string]string{
						"app":"etcd",
					},
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "etcd",
						},
					},
					Etcd: druidv1alpha1.EtcdConfig{},
					Replicas: 3,
				},
			}

			validateEtcdCreation(t, g, etcd, test.expectErr)
		})
	}
}

// Checks for valid duration values passed to etcd.spec.backup.garbageCollectionPeriod and etcd.spec.backup.deltaSnapshotPeriod, the value of GarbageCollectionPolicy is greater than the DeltaSnapshotPeriod
func TestValidateSpecBackupGCDeltaSnapshotPeriodRelation(t *testing.T) {
	tests := []struct {
		name string
		etcdName string
		gcPeriod string
		deltaSnapshotPeriod string
		expectErr bool
	} {
		{
			name: "Valid durations passed; gcperiod > deltaSnapshotPeriod; valid",
			etcdName: "etcd-valid-1",
			gcPeriod: "10m5s",
			deltaSnapshotPeriod: "5m12s",
			expectErr: false,
		},
		{
			name: "Valid durations passed; gcperiod < deltaSnapshotPeriod; invalid",
			etcdName: "etcd-invalid-1",
			gcPeriod: "1m5s",
			deltaSnapshotPeriod: "5m12s",
			expectErr: true,
		},
		{
			name: "Invalid durations passed",
			etcdName: "etcd-invalid-2",
			gcPeriod: "10m5sddd",
			deltaSnapshotPeriod: "5m12s",
			expectErr: true,
		},
	}

	testNs, g := setupTestEnvironment(t)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T){
			garbageCollectionDuration, shouldContinue := validateDuration(t, test.gcPeriod, test.expectErr)
            if !shouldContinue {
                return
            }

            deltaSnapshotDuration, shouldContinue := validateDuration(t, test.deltaSnapshotPeriod, test.expectErr)
            if !shouldContinue {
                return
            }
			etcd := &druidv1alpha1.Etcd{
                ObjectMeta: metav1.ObjectMeta{
                    Name:  test.etcdName,
                    Namespace: testNs,
                },
                Spec: druidv1alpha1.EtcdSpec{
                    Backup: druidv1alpha1.BackupSpec{
                        GarbageCollectionPeriod: garbageCollectionDuration,
                        DeltaSnapshotPeriod:     deltaSnapshotDuration,
                    },
                    Labels: map[string]string{
                        "app": "etcd",
                    },
                    Selector: &metav1.LabelSelector{
                        MatchLabels: map[string]string{
                            "app": "etcd",
                        },
                    },
                    Etcd: druidv1alpha1.EtcdConfig{},
                    Replicas: 3,
                },
            }

            validateEtcdCreation(t, g, etcd, test.expectErr)

		})
	}
}


// validates the cron expression passed into the etcd.spec.backup.fullSnapshotSchedule field.
func TestValidateSpecBackupFullSnapshotSchedule(t *testing.T){
	testNs, g := setupTestEnvironment(t)

	for _, test := range cronFieldTestCases {
		t.Run(test.name, func(t *testing.T){
			etcd := &druidv1alpha1.Etcd{
				ObjectMeta: metav1.ObjectMeta{
					Name:      test.etcdName,
					Namespace: testNs,
				},
				Spec: druidv1alpha1.EtcdSpec{
					Backup: druidv1alpha1.BackupSpec{
						FullSnapshotSchedule: &test.value,
					},
					Labels: map[string]string{
						"app":"etcd",
					},
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "etcd",
						},
					},
					Etcd: druidv1alpha1.EtcdConfig{},
					Replicas: 3,
				},
			}

			validateEtcdCreation(t, g, etcd, test.expectErr)
		})
	}
}

// tests for etcd.spec.sharedConfig fields:

// validates whether the value passed to the etcd.spec.sharedConfig.autoCompactionMode is either set as "periodic" or "revision"
func TestValidateSpecSharedConfigAutoCompactionMode(t *testing.T){
	tests := []struct {
		name string
		etcdName string
		autoCompactionMode string
		expectErr bool
	} {
		{
			name: "Valid autoCompactionMode #1: periodic",
			etcdName: "etcd-valid-1",
			autoCompactionMode: "periodic",
			expectErr: false,
		},
		{
			name: "Valid autoCompactionMode #1: revision",
			etcdName: "etcd-valid-2",
			autoCompactionMode: "revision",
			expectErr: false,
		},
		{
			name: "Invalid autoCompactionMode #1: invalid value",
			etcdName: "etcd-invalid-1",
			autoCompactionMode: "frequent",
			expectErr: true,
		},
		{
			name: "Invalid autoCompactionMode #2: empty string",
			etcdName: "etcd-valid-1",
			autoCompactionMode: "",
			expectErr: true,
		},
	}

	testNs, g := setupTestEnvironment(t)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T){
			etcd := &druidv1alpha1.Etcd{
				ObjectMeta: metav1.ObjectMeta{
					Name:      test.etcdName,
					Namespace: testNs,
				},
				Spec: druidv1alpha1.EtcdSpec{
					Backup: druidv1alpha1.BackupSpec{},
					Labels: map[string]string{
						"app":"etcd",
					},
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "etcd",
						},
					},
					Etcd: druidv1alpha1.EtcdConfig{},
					Replicas: 3,
					Common: druidv1alpha1.SharedConfig{
						AutoCompactionMode: (*druidv1alpha1.CompactionMode)(&test.autoCompactionMode),
					},
				},
			}

			validateEtcdCreation(t, g, etcd, test.expectErr)
		})
	}
}

// checks that if the values for etcd.spec.storageCapacity and etcd.spec.etcd.quota are valid, then if backups are enabled, the value of storageCapacity must be > 3x value of quota. If backups are not enabled, value of storageCapacity must be > quota
func TestValidateSpecStorageCapacitySpecEtcdQuotaRelation(t *testing.T){
	testNs, g := setupTestEnvironment(t)
	tests := []struct {
		name string
		etcdName string
		storageCapacity resource.Quantity
		quota resource.Quantity
		backup bool
		expectErr bool
	} {
		{
			name: "Valid #1: backups enabled",
			etcdName: "etcd-valid-1",
			storageCapacity: resource.MustParse("27Gi"),
			quota: resource.MustParse("8Gi"),
			backup: true,
			expectErr: false,
		},
		{
			name: "Valid #2: backups disabled",
			etcdName: "etcd-valid-2",
			storageCapacity: resource.MustParse("12Gi"),
			quota: resource.MustParse("8Gi"),
			backup: false,
			expectErr: false,
		},
		{
			name: "Invalid #1: backups enabled",
			etcdName: "etcd-invalid-1",
			storageCapacity: resource.MustParse("15Gi"),
			quota: resource.MustParse("8Gi"),
			backup: true,
			expectErr: true,
		},
		{
			name: "Invalid #2: backups disabled",
			etcdName: "etcd-invalid-2",
			storageCapacity: resource.MustParse("9Gi"),
			quota: resource.MustParse("10Gi"),
			backup: false,
			expectErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T){
			etcd := &druidv1alpha1.Etcd{
				ObjectMeta: metav1.ObjectMeta{
					Name:      test.etcdName,
					Namespace: testNs,
				},
				Spec: druidv1alpha1.EtcdSpec{
					Labels: map[string]string{
						"app": "etcd",
					},
					StorageCapacity: &test.storageCapacity,
					Etcd: druidv1alpha1.EtcdConfig{
						Quota: &test.quota,
					},
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "etcd",
						},
					},
					Replicas: 3,
				},
			}
	
			if test.backup {
				container := "etcd-bucket"
				provider := "Provider"
				etcd.Spec.Backup.Store = &druidv1alpha1.StoreSpec{
					Container: &container,
					Provider: (*druidv1alpha1.StorageProvider)(&provider),
					Prefix: "etcd-test",
					SecretRef: &corev1.SecretReference{
						Name: "test-secret",
					},
				}
			}

			validateEtcdCreation(t,g,etcd,test.expectErr)
		})


	}
}


// tests for the Update Validations:

// etcd.spec.storageClass is immutable

func TestValidateSpecStorageClass(t *testing.T) {
	testNs, g := setupTestEnvironment(t)

	

	tests := []struct {
		name string
		etcdName string
		initalStorageClassName string
		updatedStorageClassName string
		expectErr bool
	}{
		{
			name: "Valid #1: Unchanged storageClass",
			etcdName: "etcd-valid-1",
			initalStorageClassName: "gardener.cloud-fast",
			updatedStorageClassName: "gardener.cloud-fast",
			expectErr: false,
		},
		{
			name: "Invalid #1: Updated storageClass",
			etcdName: "etcd-invalid-1",
			initalStorageClassName: "gardener.cloud-fast",
			updatedStorageClassName: "default",
			expectErr: true,
		},

	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T){
			etcdInitial := &druidv1alpha1.Etcd{
				ObjectMeta: metav1.ObjectMeta{
					Name:      test.etcdName,
					Namespace: testNs,
				},
				Spec: druidv1alpha1.EtcdSpec{
					Labels: map[string]string{
						"app": "etcd",
					},
					Etcd: druidv1alpha1.EtcdConfig{
					},
					StorageClass: &test.initalStorageClassName,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "etcd",
						},
					},
					Replicas: 3,
				},
			}
	
			// validateEtcdCreation(t, g, etcdInitial, test.expectErr)
			
			cl := itTestEnv.GetClient()
			ctx := context.Background()
			g.Expect(cl.Create(ctx, etcdInitial)).To(Succeed())

			etcdInitial.Spec.StorageClass = &test.updatedStorageClassName


            if test.expectErr {
                g.Expect(cl.Update(ctx, etcdInitial)).ToNot(Succeed())
            } else {
                g.Expect(cl.Update(ctx, etcdInitial)).To(Succeed())
            }
	
			
	
		})
	}
}

// checks the update on the etcd.spec.replicas field
func TestValidateSpecReplicas(t *testing.T) {
	tests := []struct {
		name string
		etcdName string
		initialReplicas int
		updatedReplicas int
		expectErr bool
	} {
		{
			name: "Valid updation of replicas #1",
			etcdName: "etcd-valid-inc", // change later
			initialReplicas: 3,
			updatedReplicas: 5,
			expectErr: false,
		},
		{
			name: "Valid updation of replicas #2",
			etcdName: "etcd-valid-zero", // change later
			initialReplicas: 3,
			updatedReplicas: 0,
			expectErr: false,
		},
		{
			name: "Inalid updation of replicas #1",
			etcdName: "etcd-invalid-dec", // change later
			initialReplicas: 5,
			updatedReplicas: 3,
			expectErr: true,
		},
	}

	testNs, g := setupTestEnvironment(t)

	for _, test := range tests {
		t.Run(test.name, func (t *testing.T){
			etcd := &druidv1alpha1.Etcd{
				ObjectMeta: metav1.ObjectMeta{
					Name:      test.etcdName,
					Namespace: testNs,
				},
				Spec: druidv1alpha1.EtcdSpec{
					Labels: map[string]string{
						"app": "etcd",
					},
					Etcd: druidv1alpha1.EtcdConfig{
					},
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "etcd",
						},
					},
					Replicas: int32(test.initialReplicas),
				},
			}

			cl := itTestEnv.GetClient()
			ctx := context.Background()
			g.Expect(cl.Create(ctx, etcd)).To(Succeed())

			etcd.Spec.Replicas = int32(test.updatedReplicas)
			time.Sleep(30)
			if test.expectErr {
                g.Expect(cl.Update(ctx, etcd)).ToNot(Succeed())
            } else {
                g.Expect(cl.Update(ctx, etcd)).To(Succeed())
            }
		})
	}
}


// checks the immutablility of the etcd.spec.StorageCapacity field
func TestValidateSpecStorageCapacity(t *testing.T) {
	testNs, g := setupTestEnvironment(t)
	tests := []struct {
		name string
		etcdName string
		initalStorageCapacity resource.Quantity
		updatedStorageCapacity resource.Quantity
		expectErr bool
	}{
		{
			name: "Valid #1: Unchanged storageCapacity",
			etcdName: "etcd-valid-1-storagecap",
			initalStorageCapacity: resource.MustParse("25Gi"),
			updatedStorageCapacity: resource.MustParse("25Gi"),
			expectErr: false,
		},
		{
			name: "Invalid #1: Updated storageCapacity",
			etcdName: "etcd-invalid-1-storagecap",
			initalStorageCapacity: resource.MustParse("15Gi"),
			updatedStorageCapacity: resource.MustParse("20Gi"),
			expectErr: true,
		},

	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T){
			etcdInitial := &druidv1alpha1.Etcd{
				ObjectMeta: metav1.ObjectMeta{
					Name:      test.etcdName,
					Namespace: testNs,
				},
				Spec: druidv1alpha1.EtcdSpec{
					Labels: map[string]string{
						"app": "etcd",
					},
					Etcd: druidv1alpha1.EtcdConfig{
					},
					StorageCapacity: &test.initalStorageCapacity,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "etcd",
						},
					},
					Replicas: 3,
				},
			}
	
			// validateEtcdCreation(t, g, etcdInitial, test.expectErr)
			
			cl := itTestEnv.GetClient()
			ctx := context.Background()
			g.Expect(cl.Create(ctx, etcdInitial)).To(Succeed())

			etcdInitial.Spec.StorageCapacity = &test.updatedStorageCapacity


            if test.expectErr {
                g.Expect(cl.Update(ctx, etcdInitial)).ToNot(Succeed())
            } else {
                g.Expect(cl.Update(ctx, etcdInitial)).To(Succeed())
            }
	
			
	
		})
	}
}


// check the immutability of the etcd.spec.VolumeClaimTemplate field
func TestValidateSpecVolumeClaimTemplate(t *testing.T) {
	testNs, g := setupTestEnvironment(t)
	tests := []struct {
		name string
		etcdName string
		initalVolClaimTemp string
		updatedVolClaimTemp string
		expectErr bool
	}{
		{
			name: "Valid #1: Unchanged volumeClaimTemplate",
			etcdName: "etcd-valid-1-volclaim",
			initalVolClaimTemp: "main-etcd",
			updatedVolClaimTemp: "main-etcd",
			expectErr: false,
		},
		{
			name: "Invalid #1: Updated storageCapacity",
			etcdName: "etcd-invalid-1-volclaim",
			initalVolClaimTemp: "main-etcd",
			updatedVolClaimTemp: "new-vol-temp",
			expectErr: true,
		},

	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T){
			etcdInitial := &druidv1alpha1.Etcd{
				ObjectMeta: metav1.ObjectMeta{
					Name:      test.etcdName,
					Namespace: testNs,
				},
				Spec: druidv1alpha1.EtcdSpec{
					Labels: map[string]string{
						"app": "etcd",
					},
					Etcd: druidv1alpha1.EtcdConfig{
					},
					VolumeClaimTemplate: &test.initalVolClaimTemp,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "etcd",
						},
					},
					Replicas: 3,
				},
			}
	
			// validateEtcdCreation(t, g, etcdInitial, test.expectErr)
			
			cl := itTestEnv.GetClient()
			ctx := context.Background()
			g.Expect(cl.Create(ctx, etcdInitial)).To(Succeed())

			etcdInitial.Spec.VolumeClaimTemplate = &test.updatedVolClaimTemp


            if test.expectErr {
                g.Expect(cl.Update(ctx, etcdInitial)).ToNot(Succeed())
            } else {
                g.Expect(cl.Update(ctx, etcdInitial)).To(Succeed())
            }
	
			
	
		})
	}
}