// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"testing"
	"time"

	configv1alpha1 "github.com/gardener/etcd-druid/api/config/v1alpha1"

	gomegatypes "github.com/onsi/gomega/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/ptr"

	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

var zero = time.Duration(0)

const (
	testNs                 = "test-ns"
	testServiceAccountName = "test-sa"
	testServiceAccountFQDN = "system:serviceaccount:test-ns:test-sa"
)

func TestValidateLeaderElectionConfiguration(t *testing.T) {
	tests := []struct {
		name                  string
		enabled               bool
		overrideLeaseDuration *time.Duration
		overrideRenewDeadline *time.Duration
		overrideRetryPeriod   *time.Duration
		overrideResourceLock  *string
		overrideResourceName  *string
		expectedErrors        int
		matcher               gomegatypes.GomegaMatcher
	}{
		{
			name:           "should allow to disable leader election",
			enabled:        false,
			expectedErrors: 0,
			matcher:        nil,
		},
		{
			name:           "should allow default leader election configuration",
			enabled:        true,
			expectedErrors: 0,
			matcher:        nil,
		},
		{
			name:                 "should forbid empty resource lock",
			enabled:              true,
			overrideResourceLock: ptr.To(""),
			expectedErrors:       1,
			matcher:              ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeRequired), "Field": Equal("leaderElection.resourceLock")}))),
		},
		{
			name:                 "should forbid empty resource name",
			enabled:              true,
			overrideResourceName: ptr.To(""),
			expectedErrors:       1,
			matcher:              ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeRequired), "Field": Equal("leaderElection.resourceName")}))),
		},
		{
			name:                  "should forbid zero lease duration",
			enabled:               true,
			overrideLeaseDuration: ptr.To(zero),
			expectedErrors:        2,
			matcher: ConsistOf(
				PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeInvalid), "Field": Equal("leaderElection.leaseDuration")})),
				PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeInvalid), "Field": Equal("leaderElection.leaseDuration")})),
			),
		},
		{
			name:                  "should forbid zero renew deadline",
			enabled:               true,
			overrideRenewDeadline: ptr.To(zero),
			expectedErrors:        1,
			matcher:               ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeInvalid), "Field": Equal("leaderElection.renewDeadline")}))),
		},
		{
			name:                "should forbid zero retry period",
			enabled:             true,
			overrideRetryPeriod: ptr.To(zero),
			expectedErrors:      1,
			matcher:             ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeInvalid), "Field": Equal("leaderElection.retryPeriod")}))),
		},
		{
			name:                  "should forbid renew deadline greater than lease duration",
			enabled:               true,
			overrideLeaseDuration: ptr.To(time.Second),
			overrideRenewDeadline: ptr.To(2 * time.Second),
			expectedErrors:        1,
			matcher:               ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeInvalid), "Field": Equal("leaderElection.leaseDuration")}))),
		},
	}

	fldPath := field.NewPath("leaderElection")

	g := NewWithT(t)
	t.Parallel()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			leaderElectionConfig := &configv1alpha1.LeaderElectionConfiguration{
				Enabled: test.enabled,
			}
			configv1alpha1.SetDefaults_LeaderElectionConfiguration(leaderElectionConfig)
			if test.enabled {
				updateLeaderElectionConfig(leaderElectionConfig, test.overrideLeaseDuration, test.overrideRenewDeadline, test.overrideRetryPeriod, test.overrideResourceLock, test.overrideResourceName)
			}
			actualErr := validateLeaderElectionConfiguration(*leaderElectionConfig, fldPath)
			g.Expect(len(actualErr)).To(Equal(test.expectedErrors))
			if test.matcher != nil {
				g.Expect(actualErr).To(test.matcher)
			}
		})
	}
}

func TestUpdateClientConnectionConfiguration(t *testing.T) {
	tests := []struct {
		name           string
		overrideBurst  *int
		expectedErrors int
		matcher        gomegatypes.GomegaMatcher
	}{
		{
			name:           "should allow default client connection configuration",
			expectedErrors: 0,
			matcher:        nil,
		},
		{
			name:           "should forbid bust to be less than zero",
			overrideBurst:  ptr.To(-1),
			expectedErrors: 1,
			matcher:        ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeInvalid), "Field": Equal("clientConnection.burst")}))),
		},
	}

	fldPath := field.NewPath("clientConnection")

	g := NewWithT(t)
	t.Parallel()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			clientConnConfig := &configv1alpha1.ClientConnectionConfiguration{}
			configv1alpha1.SetDefaults_ClientConnectionConfiguration(clientConnConfig)
			if test.overrideBurst != nil {
				clientConnConfig.Burst = *test.overrideBurst
			}
			actualErr := validateClientConnectionConfiguration(*clientConnConfig, fldPath)
			g.Expect(len(actualErr)).To(Equal(test.expectedErrors))
		})
	}
}

func TestValidateLogConfiguration(t *testing.T) {
	tests := []struct {
		name           string
		logLevel       *configv1alpha1.LogLevel
		logFormat      *configv1alpha1.LogFormat
		expectedErrors int
		matcher        gomegatypes.GomegaMatcher
	}{
		{
			name:           "should allow debug log configuration",
			logLevel:       ptr.To(configv1alpha1.LogLevelDebug),
			expectedErrors: 0,
			matcher:        nil,
		},
		{
			name:           "should allow info log configuration",
			logLevel:       ptr.To(configv1alpha1.LogLevelInfo),
			expectedErrors: 0,
			matcher:        nil,
		},
		{
			name:           "should allow error log configuration",
			logLevel:       ptr.To(configv1alpha1.LogLevelError),
			expectedErrors: 0,
			matcher:        nil,
		},
		{
			name:           "should allow text log format",
			logFormat:      ptr.To(configv1alpha1.LogFormatText),
			expectedErrors: 0,
			matcher:        nil,
		},
		{
			name:           "should forbid invalid log level",
			logLevel:       ptr.To(configv1alpha1.LogLevel("invalid-level")),
			expectedErrors: 1,
			matcher:        ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeNotSupported), "Field": Equal("log.logLevel")}))),
		},
		{
			name:           "should forbid invalid log format",
			logFormat:      ptr.To(configv1alpha1.LogFormat("invalid-log-format")),
			expectedErrors: 1,
			matcher:        ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeNotSupported), "Field": Equal("log.logFormat")}))),
		},
	}

	fldPath := field.NewPath("log")

	g := NewWithT(t)
	t.Parallel()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			logConfig := &configv1alpha1.LogConfiguration{}
			configv1alpha1.SetDefaults_LogConfiguration(logConfig)
			if test.logLevel != nil {
				logConfig.LogLevel = *test.logLevel
			}
			if test.logFormat != nil {
				logConfig.LogFormat = *test.logFormat
			}
			actualErr := validateLogConfiguration(*logConfig, fldPath)
			g.Expect(len(actualErr)).To(Equal(test.expectedErrors))
			if test.matcher != nil {
				g.Expect(actualErr).To(test.matcher)
			}
		})
	}
}

func TestValidateEtcdControllerConfiguration(t *testing.T) {
	tests := []struct {
		name           string
		concurrentSync *int
		expectedErrors int
		matcher        gomegatypes.GomegaMatcher
	}{
		{
			name:           "should allow default etcd controller configuration",
			expectedErrors: 0,
			matcher:        nil,
		},
		{
			name:           "should allow concurrent syncs greater than zero",
			concurrentSync: ptr.To(1),
			expectedErrors: 0,
		},
		{
			name:           "should forbid concurrent syncs equal to zero",
			concurrentSync: ptr.To(0),
			expectedErrors: 1,
			matcher:        ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeInvalid), "Field": Equal("controllers.etcd.concurrentSyncs")}))),
		},
		{
			name:           "should forbid concurrent syncs less than zero",
			concurrentSync: ptr.To(-1),
			expectedErrors: 1,
			matcher:        ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeInvalid), "Field": Equal("controllers.etcd.concurrentSyncs")}))),
		},
	}

	fldPath := field.NewPath("controllers.etcd")
	g := NewWithT(t)
	t.Parallel()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			etcdConfig := &configv1alpha1.EtcdControllerConfiguration{}
			configv1alpha1.SetDefaults_EtcdControllerConfiguration(etcdConfig)
			if test.concurrentSync != nil {
				etcdConfig.ConcurrentSyncs = test.concurrentSync
			}
			actualErr := validateEtcdControllerConfiguration(*etcdConfig, fldPath)
			g.Expect(len(actualErr)).To(Equal(test.expectedErrors))
			if test.matcher != nil {
				g.Expect(actualErr).To(test.matcher)
			}
		})
	}
}

func TestValidateCompactionControllerConfiguration(t *testing.T) {
	tests := []struct {
		name           string
		enabled        bool
		concurrentSync *int
		expectedErrors int
		matcher        gomegatypes.GomegaMatcher
	}{
		{
			name:           "should allow default compaction controller configuration when it is enabled",
			enabled:        true,
			expectedErrors: 0,
			matcher:        nil,
		},
		{
			name:           "should allow empty compaction controller configuration when it is disabled",
			enabled:        false,
			expectedErrors: 0,
			matcher:        nil,
		},
		{
			name:           "should allow concurrent syncs greater than zero",
			enabled:        true,
			concurrentSync: ptr.To(1),
			expectedErrors: 0,
		},
		{
			name:           "should forbid concurrent syncs equal to zero",
			enabled:        true,
			concurrentSync: ptr.To(0),
			expectedErrors: 1,
			matcher:        ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeInvalid), "Field": Equal("controllers.compaction.concurrentSyncs")}))),
		},
		{
			name:           "should forbid concurrent syncs less than zero",
			enabled:        true,
			concurrentSync: ptr.To(-1),
			expectedErrors: 1,
			matcher:        ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeInvalid), "Field": Equal("controllers.compaction.concurrentSyncs")}))),
		},
	}

	fldPath := field.NewPath("controllers.compaction")
	g := NewWithT(t)
	t.Parallel()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			controllerConfig := &configv1alpha1.CompactionControllerConfiguration{}
			controllerConfig.Enabled = test.enabled
			configv1alpha1.SetDefaults_CompactionControllerConfiguration(controllerConfig)
			if test.concurrentSync != nil {
				controllerConfig.ConcurrentSyncs = test.concurrentSync
			}
			actualErr := validateCompactionControllerConfiguration(*controllerConfig, fldPath)
			g.Expect(len(actualErr)).To(Equal(test.expectedErrors))
			if test.matcher != nil {
				g.Expect(actualErr).To(test.matcher)
			}
		})
	}
}

func TestValidateEtcdCopyBackupsTaskControllerConfiguration(t *testing.T) {
	tests := []struct {
		name           string
		enabled        bool
		concurrentSync *int
		expectedErrors int
		matcher        gomegatypes.GomegaMatcher
	}{
		{
			name:           "should allow default etcdCopyBackupsTask controller configuration when it is enabled",
			enabled:        true,
			expectedErrors: 0,
			matcher:        nil,
		},
		{
			name:           "should allow empty etcdCopyBackupsTask controller configuration when it is disabled",
			enabled:        false,
			expectedErrors: 0,
		},
		{
			name:           "should allow concurrent syncs greater than zero",
			enabled:        true,
			concurrentSync: ptr.To(1),
			expectedErrors: 0,
		},
		{
			name:           "should forbid concurrent syncs equal to zero",
			enabled:        true,
			concurrentSync: ptr.To(0),
			expectedErrors: 1,
			matcher:        ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeInvalid), "Field": Equal("controllers.etcdCopyBackupsTask.concurrentSyncs")}))),
		},
		{
			name:           "should forbid concurrent syncs less than zero",
			enabled:        true,
			concurrentSync: ptr.To(-1),
			expectedErrors: 1,
			matcher:        ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeInvalid), "Field": Equal("controllers.etcdCopyBackupsTask.concurrentSyncs")}))),
		},
	}

	fldPath := field.NewPath("controllers.etcdCopyBackupsTask")
	g := NewWithT(t)
	t.Parallel()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			controllerConfig := &configv1alpha1.EtcdCopyBackupsTaskControllerConfiguration{}
			controllerConfig.Enabled = test.enabled
			configv1alpha1.SetDefaults_EtcdCopyBackupsTaskControllerConfiguration(controllerConfig)
			if test.concurrentSync != nil {
				controllerConfig.ConcurrentSyncs = test.concurrentSync
			}
			actualErr := validateEtcdCopyBackupsTaskControllerConfiguration(*controllerConfig, fldPath)
			g.Expect(len(actualErr)).To(Equal(test.expectedErrors))
			if test.matcher != nil {
				g.Expect(actualErr).To(test.matcher)
			}
		})
	}
}

func TestValidateSecretControllerConfiguration(t *testing.T) {
	tests := []struct {
		name           string
		concurrentSync *int
		expectedErrors int
		matcher        gomegatypes.GomegaMatcher
	}{
		{
			name:           "should allow default secret controller configuration",
			expectedErrors: 0,
			matcher:        nil,
		},
		{
			name:           "should allow concurrent syncs greater than zero",
			concurrentSync: ptr.To(1),
			expectedErrors: 0,
		},
		{
			name:           "should forbid concurrent syncs equal to zero",
			concurrentSync: ptr.To(0),
			expectedErrors: 1,
			matcher:        ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeInvalid), "Field": Equal("controllers.secret.concurrentSyncs")}))),
		},
		{
			name:           "should forbid concurrent syncs less than zero",
			concurrentSync: ptr.To(-1),
			expectedErrors: 1,
			matcher:        ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeInvalid), "Field": Equal("controllers.secret.concurrentSyncs")}))),
		},
	}

	fldPath := field.NewPath("controllers.secret")
	g := NewWithT(t)
	t.Parallel()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			controllerConfig := &configv1alpha1.SecretControllerConfiguration{}
			configv1alpha1.SetDefaults_SecretControllerConfiguration(controllerConfig)
			if test.concurrentSync != nil {
				controllerConfig.ConcurrentSyncs = test.concurrentSync
			}
			actualErr := validateSecretControllerConfiguration(*controllerConfig, fldPath)
			g.Expect(len(actualErr)).To(Equal(test.expectedErrors))
			if test.matcher != nil {
				g.Expect(actualErr).To(test.matcher)
			}
		})
	}
}

func TestValidateEtcdComponentProtectionWebhookConfiguration(t *testing.T) {
	tests := []struct {
		name                                 string
		enabled                              bool
		overrideReconcilerServiceAccountFQDN *string
		serviceAccountInfo                   *configv1alpha1.ServiceAccountInfo
		overrideExemptServiceAccounts        []string
		expectedErrors                       int
		matcher                              gomegatypes.GomegaMatcher
	}{
		{
			name:           "should allow empty configuration when it is disabled",
			enabled:        false,
			expectedErrors: 0,
			matcher:        nil,
		},
		{
			name:                                 "when enabled, should allow valid configuration using deprecated reconciler service account FQDN",
			enabled:                              true,
			overrideReconcilerServiceAccountFQDN: ptr.To(testServiceAccountFQDN),
			overrideExemptServiceAccounts:        []string{"garbage-collector-sa"},
			expectedErrors:                       0,
			matcher:                              nil,
		},
		{
			name:    "when enabled, should allow valid configuration using a valid service account info",
			enabled: true,
			serviceAccountInfo: &configv1alpha1.ServiceAccountInfo{
				Name:      testServiceAccountName,
				Namespace: testNs,
			},
			expectedErrors: 0,
			matcher:        nil,
		},
		{
			name:           "when enabled, should forbid nil values for both reconcilerServiceAccountFQDN and serviceAccountInfo",
			enabled:        true,
			expectedErrors: 1,
			matcher:        ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeRequired), "Field": Equal("webhooks.etcdComponentProtection")}))),
		},
		{
			name:                          "when enabled with non-nil serviceAccountInfo, should forbid empty values for namePath and namespacePath",
			enabled:                       true,
			overrideExemptServiceAccounts: []string{"garbage-collector-sa"},
			serviceAccountInfo:            &configv1alpha1.ServiceAccountInfo{},
			expectedErrors:                2,
			matcher: ConsistOf(
				PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeRequired), "Field": Equal("webhooks.etcdComponentProtection.serviceAccountInfo.name")})),
				PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeRequired), "Field": Equal("webhooks.etcdComponentProtection.serviceAccountInfo.namespace")})),
			),
		},
	}

	fldPath := field.NewPath("webhooks.etcdComponentProtection")
	g := NewWithT(t)
	t.Parallel()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			webhookConfig := &configv1alpha1.EtcdComponentProtectionWebhookConfiguration{}
			webhookConfig.Enabled = test.enabled
			if test.overrideExemptServiceAccounts != nil {
				webhookConfig.ExemptServiceAccounts = test.overrideExemptServiceAccounts
			}
			webhookConfig.ReconcilerServiceAccountFQDN = test.overrideReconcilerServiceAccountFQDN
			webhookConfig.ServiceAccountInfo = test.serviceAccountInfo
			actualErr := validateEtcdComponentProtectionWebhookConfiguration(*webhookConfig, fldPath)
			g.Expect(len(actualErr)).To(Equal(test.expectedErrors))
			if test.matcher != nil {
				g.Expect(actualErr).To(test.matcher)
			}
		})
	}
}

func updateLeaderElectionConfig(config *configv1alpha1.LeaderElectionConfiguration,
	overrideLeaseDuration *time.Duration,
	overrideRenewDeadline *time.Duration,
	overrideRetryPeriod *time.Duration,
	overrideResourceLock *string,
	overrideResourceName *string) {

	if overrideLeaseDuration != nil {
		config.LeaseDuration = metav1.Duration{Duration: *overrideLeaseDuration}
	}
	if overrideRenewDeadline != nil {
		config.RenewDeadline = metav1.Duration{Duration: *overrideRenewDeadline}
	}
	if overrideRetryPeriod != nil {
		config.RetryPeriod = metav1.Duration{Duration: *overrideRetryPeriod}
	}
	if overrideResourceLock != nil {
		config.ResourceLock = *overrideResourceLock
	}
	if overrideResourceName != nil {
		config.ResourceName = *overrideResourceName
	}
}
