// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"testing"
	"time"

	druidconfigv1alpha1 "github.com/gardener/etcd-druid/api/config/v1alpha1"

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
		enabled               *bool
		overrideLeaseDuration *time.Duration
		overrideRenewDeadline *time.Duration
		overrideRetryPeriod   *time.Duration
		overrideResourceLock  *string
		overrideResourceName  *string
		expectedErrors        int
		matcher               gomegatypes.GomegaMatcher
	}{
		{
			name:           "should allow to unset leader election",
			enabled:        nil,
			expectedErrors: 0,
			matcher:        nil,
		},
		{
			name:           "should allow to disable leader election",
			enabled:        ptr.To(false),
			expectedErrors: 0,
			matcher:        nil,
		},
		{
			name:           "should allow default leader election configuration",
			enabled:        ptr.To(true),
			expectedErrors: 0,
			matcher:        nil,
		},
		{
			name:                 "should forbid empty resource lock",
			enabled:              ptr.To(true),
			overrideResourceLock: ptr.To(""),
			expectedErrors:       1,
			matcher:              ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeRequired), "Field": Equal("leaderElection.resourceLock")}))),
		},
		{
			name:                 "should forbid empty resource name",
			enabled:              ptr.To(true),
			overrideResourceName: ptr.To(""),
			expectedErrors:       1,
			matcher:              ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeRequired), "Field": Equal("leaderElection.resourceName")}))),
		},
		{
			name:                  "should forbid zero lease duration",
			enabled:               ptr.To(true),
			overrideLeaseDuration: ptr.To(zero),
			expectedErrors:        2,
			matcher: ConsistOf(
				PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeInvalid), "Field": Equal("leaderElection.leaseDuration")})),
				PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeInvalid), "Field": Equal("leaderElection.leaseDuration")})),
			),
		},
		{
			name:                  "should forbid zero renew deadline",
			enabled:               ptr.To(true),
			overrideRenewDeadline: ptr.To(zero),
			expectedErrors:        1,
			matcher:               ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeInvalid), "Field": Equal("leaderElection.renewDeadline")}))),
		},
		{
			name:                "should forbid zero retry period",
			enabled:             ptr.To(true),
			overrideRetryPeriod: ptr.To(zero),
			expectedErrors:      1,
			matcher:             ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeInvalid), "Field": Equal("leaderElection.retryPeriod")}))),
		},
		{
			name:                  "should forbid renew deadline greater than lease duration",
			enabled:               ptr.To(true),
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
			leaderElectionConfig := &druidconfigv1alpha1.LeaderElectionConfiguration{
				Enabled: test.enabled,
			}
			druidconfigv1alpha1.SetDefaults_LeaderElectionConfiguration(leaderElectionConfig)
			if test.enabled != nil && *test.enabled {
				updateLeaderElectionConfig(leaderElectionConfig, test.overrideLeaseDuration, test.overrideRenewDeadline, test.overrideRetryPeriod, test.overrideResourceLock, test.overrideResourceName)
			}
			actualErrList := validateLeaderElectionConfiguration(*leaderElectionConfig, fldPath)
			g.Expect(len(actualErrList)).To(Equal(test.expectedErrors))
			if test.matcher != nil {
				g.Expect(actualErrList).To(test.matcher)
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
			clientConnConfig := &druidconfigv1alpha1.ClientConnectionConfiguration{}
			druidconfigv1alpha1.SetDefaults_ClientConnectionConfiguration(clientConnConfig)
			if test.overrideBurst != nil {
				clientConnConfig.Burst = *test.overrideBurst
			}
			actualErrList := validateClientConnectionConfiguration(*clientConnConfig, fldPath)
			g.Expect(len(actualErrList)).To(Equal(test.expectedErrors))
		})
	}
}

func TestValidateLogConfiguration(t *testing.T) {
	tests := []struct {
		name           string
		logLevel       *druidconfigv1alpha1.LogLevel
		logFormat      *druidconfigv1alpha1.LogFormat
		expectedErrors int
		matcher        gomegatypes.GomegaMatcher
	}{
		{
			name:           "should allow debug log configuration",
			logLevel:       ptr.To(druidconfigv1alpha1.LogLevelDebug),
			expectedErrors: 0,
			matcher:        nil,
		},
		{
			name:           "should allow info log configuration",
			logLevel:       ptr.To(druidconfigv1alpha1.LogLevelInfo),
			expectedErrors: 0,
			matcher:        nil,
		},
		{
			name:           "should allow error log configuration",
			logLevel:       ptr.To(druidconfigv1alpha1.LogLevelError),
			expectedErrors: 0,
			matcher:        nil,
		},
		{
			name:           "should allow text log format",
			logFormat:      ptr.To(druidconfigv1alpha1.LogFormatText),
			expectedErrors: 0,
			matcher:        nil,
		},
		{
			name:           "should forbid invalid log level",
			logLevel:       ptr.To(druidconfigv1alpha1.LogLevel("invalid-level")),
			expectedErrors: 1,
			matcher:        ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeNotSupported), "Field": Equal("log.logLevel")}))),
		},
		{
			name:           "should forbid invalid log format",
			logFormat:      ptr.To(druidconfigv1alpha1.LogFormat("invalid-log-format")),
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
			logConfig := &druidconfigv1alpha1.LogConfiguration{}
			druidconfigv1alpha1.SetDefaults_LogConfiguration(logConfig)
			if test.logLevel != nil {
				logConfig.LogLevel = *test.logLevel
			}
			if test.logFormat != nil {
				logConfig.LogFormat = *test.logFormat
			}
			actualErrList := validateLogConfiguration(*logConfig, fldPath)
			g.Expect(len(actualErrList)).To(Equal(test.expectedErrors))
			if test.matcher != nil {
				g.Expect(actualErrList).To(test.matcher)
			}
		})
	}
}

func TestValidateEtcdControllerConfiguration(t *testing.T) {
	tests := []struct {
		name                 string
		concurrentSync       *int
		etcdStatusSyncPeriod *metav1.Duration
		notReadyThreshold    *metav1.Duration
		unknownThreshold     *metav1.Duration
		expectedErrors       int
		matcher              gomegatypes.GomegaMatcher
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
		{
			name:                 "should forbid etcdStatusSyncPeriod less than zero",
			etcdStatusSyncPeriod: ptr.To(metav1.Duration{Duration: -time.Second}),
			expectedErrors:       1,
			matcher:              ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeInvalid), "Field": Equal("controllers.etcd.etcdStatusSyncPeriod")}))),
		},
		{
			name:              "should forbid notReadyThreshold less than zero",
			notReadyThreshold: ptr.To(metav1.Duration{Duration: -time.Second}),
			expectedErrors:    1,
			matcher:           ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeInvalid), "Field": Equal("controllers.etcd.etcdMember.notReadyThreshold")}))),
		},
		{
			name:             "should forbid unknownThreshold less than zero",
			unknownThreshold: ptr.To(metav1.Duration{Duration: -time.Second}),
			expectedErrors:   1,
			matcher:          ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeInvalid), "Field": Equal("controllers.etcd.etcdMember.unknownThreshold")}))),
		},
	}

	fldPath := field.NewPath("controllers.etcd")
	g := NewWithT(t)
	t.Parallel()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			etcdConfig := &druidconfigv1alpha1.EtcdControllerConfiguration{}
			druidconfigv1alpha1.SetDefaults_EtcdControllerConfiguration(etcdConfig)
			if test.concurrentSync != nil {
				etcdConfig.ConcurrentSyncs = test.concurrentSync
			}
			if test.etcdStatusSyncPeriod != nil {
				etcdConfig.EtcdStatusSyncPeriod = *test.etcdStatusSyncPeriod
			}
			if test.notReadyThreshold != nil {
				etcdConfig.EtcdMember.NotReadyThreshold = *test.notReadyThreshold
			}
			if test.unknownThreshold != nil {
				etcdConfig.EtcdMember.UnknownThreshold = *test.unknownThreshold
			}
			actualErrList := validateEtcdControllerConfiguration(*etcdConfig, fldPath)
			g.Expect(len(actualErrList)).To(Equal(test.expectedErrors))
			if test.matcher != nil {
				g.Expect(actualErrList).To(test.matcher)
			}
		})
	}
}

func TestValidateCompactionControllerConfiguration(t *testing.T) {
	tests := []struct {
		name                         string
		enabled                      *bool
		concurrentSync               *int
		eventsThreshold              *int64
		triggerFullSnapshotThreshold *int64
		activeDeadlineDuration       *metav1.Duration
		metricsScrapeWaitDuration    *metav1.Duration
		expectedErrors               int
		matcher                      gomegatypes.GomegaMatcher
	}{
		{
			name:           "should allow default compaction controller configuration when it is enabled",
			enabled:        ptr.To(true),
			expectedErrors: 0,
			matcher:        nil,
		},
		{
			name:           "should allow empty compaction controller configuration when it is disabled",
			enabled:        ptr.To(false),
			expectedErrors: 0,
			matcher:        nil,
		},
		{
			name:           "should allow empty compaction controller configuration when enabled is unset",
			enabled:        nil,
			expectedErrors: 0,
			matcher:        nil,
		},
		{
			name:           "should allow concurrent syncs greater than zero",
			enabled:        ptr.To(true),
			concurrentSync: ptr.To(1),
			expectedErrors: 0,
		},
		{
			name:           "should forbid concurrent syncs equal to zero",
			enabled:        ptr.To(true),
			concurrentSync: ptr.To(0),
			expectedErrors: 1,
			matcher:        ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeInvalid), "Field": Equal("controllers.compaction.concurrentSyncs")}))),
		},
		{
			name:           "should forbid concurrent syncs less than zero",
			enabled:        ptr.To(true),
			concurrentSync: ptr.To(-1),
			expectedErrors: 1,
			matcher:        ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeInvalid), "Field": Equal("controllers.compaction.concurrentSyncs")}))),
		},
		{
			name:            "should forbid events threshold equal to zero",
			enabled:         ptr.To(true),
			eventsThreshold: ptr.To(int64(0)),
			expectedErrors:  1,
			matcher:         ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeInvalid), "Field": Equal("controllers.compaction.eventsThreshold")}))),
		},
		{
			name:            "should forbid events threshold less than zero",
			enabled:         ptr.To(true),
			eventsThreshold: ptr.To(int64(-1)),
			expectedErrors:  1,
			matcher:         ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeInvalid), "Field": Equal("controllers.compaction.eventsThreshold")}))),
		},
		{
			name:                         "should forbid trigger full snapshot threshold equal to zero",
			enabled:                      ptr.To(true),
			triggerFullSnapshotThreshold: ptr.To(int64(0)),
			expectedErrors:               1,
			matcher:                      ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeInvalid), "Field": Equal("controllers.compaction.triggerFullSnapshotThreshold")}))),
		},
		{
			name:                         "should forbid trigger full snapshot threshold less than zero",
			enabled:                      ptr.To(true),
			triggerFullSnapshotThreshold: ptr.To(int64(-1)),
			expectedErrors:               1,
			matcher:                      ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeInvalid), "Field": Equal("controllers.compaction.triggerFullSnapshotThreshold")}))),
		},
		{
			name:                   "should forbid active deadline duration less than zero",
			enabled:                ptr.To(true),
			activeDeadlineDuration: &metav1.Duration{Duration: -time.Second},
			expectedErrors:         1,
			matcher:                ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeInvalid), "Field": Equal("controllers.compaction.activeDeadlineDuration")}))),
		},
		{
			name:                      "should forbid metrics scrape wait duration less than zero",
			enabled:                   ptr.To(true),
			metricsScrapeWaitDuration: &metav1.Duration{Duration: -time.Second},
			expectedErrors:            1,
			matcher:                   ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeInvalid), "Field": Equal("controllers.compaction.metricsScrapeWaitDuration")}))),
		},
	}

	fldPath := field.NewPath("controllers.compaction")
	g := NewWithT(t)
	t.Parallel()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			controllerConfig := &druidconfigv1alpha1.CompactionControllerConfiguration{}
			controllerConfig.Enabled = test.enabled
			druidconfigv1alpha1.SetDefaults_CompactionControllerConfiguration(controllerConfig)
			if test.concurrentSync != nil {
				controllerConfig.ConcurrentSyncs = test.concurrentSync
			}
			if test.eventsThreshold != nil {
				controllerConfig.EventsThreshold = *test.eventsThreshold
			}
			if test.triggerFullSnapshotThreshold != nil {
				controllerConfig.TriggerFullSnapshotThreshold = *test.triggerFullSnapshotThreshold
			}
			if test.activeDeadlineDuration != nil {
				controllerConfig.ActiveDeadlineDuration = *test.activeDeadlineDuration
			}
			if test.metricsScrapeWaitDuration != nil {
				controllerConfig.MetricsScrapeWaitDuration = *test.metricsScrapeWaitDuration
			}
			actualErrList := validateCompactionControllerConfiguration(*controllerConfig, fldPath)
			g.Expect(len(actualErrList)).To(Equal(test.expectedErrors))
			if test.matcher != nil {
				g.Expect(actualErrList).To(test.matcher)
			}
		})
	}
}

func TestValidateEtcdCopyBackupsTaskControllerConfiguration(t *testing.T) {
	tests := []struct {
		name           string
		enabled        *bool
		concurrentSync *int
		expectedErrors int
		matcher        gomegatypes.GomegaMatcher
	}{
		{
			name:           "should allow default etcdCopyBackupsTask controller configuration when it is enabled",
			enabled:        ptr.To(true),
			expectedErrors: 0,
			matcher:        nil,
		},
		{
			name:           "should allow empty etcdCopyBackupsTask controller configuration when it is disabled",
			enabled:        ptr.To(false),
			expectedErrors: 0,
		},
		{
			name:           "should allow empty etcdCopyBackupsTask controller configuration when enabled is unset",
			enabled:        nil,
			expectedErrors: 0,
		},
		{
			name:           "should allow concurrent syncs greater than zero",
			enabled:        ptr.To(true),
			concurrentSync: ptr.To(1),
			expectedErrors: 0,
		},
		{
			name:           "should forbid concurrent syncs equal to zero",
			enabled:        ptr.To(true),
			concurrentSync: ptr.To(0),
			expectedErrors: 1,
			matcher:        ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeInvalid), "Field": Equal("controllers.etcdCopyBackupsTask.concurrentSyncs")}))),
		},
		{
			name:           "should forbid concurrent syncs less than zero",
			enabled:        ptr.To(true),
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
			controllerConfig := &druidconfigv1alpha1.EtcdCopyBackupsTaskControllerConfiguration{}
			controllerConfig.Enabled = test.enabled
			druidconfigv1alpha1.SetDefaults_EtcdCopyBackupsTaskControllerConfiguration(controllerConfig)
			if test.concurrentSync != nil {
				controllerConfig.ConcurrentSyncs = test.concurrentSync
			}
			actualErrList := validateEtcdCopyBackupsTaskControllerConfiguration(*controllerConfig, fldPath)
			g.Expect(len(actualErrList)).To(Equal(test.expectedErrors))
			if test.matcher != nil {
				g.Expect(actualErrList).To(test.matcher)
			}
		})
	}
}

func TestValidateEtcdOpsTaskControllerConfiguration(t *testing.T) {
	tests := []struct {
		name              string
		config            *druidconfigv1alpha1.EtcdOpsTaskControllerConfiguration
		numExpectedErrors int
		matcher           gomegatypes.GomegaMatcher
	}{
		{
			name:              "should allow default etcdOpsTask controller configuration",
			config:            &druidconfigv1alpha1.EtcdOpsTaskControllerConfiguration{},
			numExpectedErrors: 0,
			matcher:           nil,
		},
		{
			name: "should allow concurrent syncs greater than zero",
			config: &druidconfigv1alpha1.EtcdOpsTaskControllerConfiguration{
				ConcurrentSyncs: ptr.To(1),
			},
			numExpectedErrors: 0,
		},
		{
			name: "should forbid concurrent syncs equal to zero",
			config: &druidconfigv1alpha1.EtcdOpsTaskControllerConfiguration{
				ConcurrentSyncs: ptr.To(0),
			},
			numExpectedErrors: 1,
			matcher:           ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeInvalid), "Field": Equal("controllers.etcdOpsTask.concurrentSyncs")}))),
		},
		{
			name: "should forbid concurrent syncs less than zero",
			config: &druidconfigv1alpha1.EtcdOpsTaskControllerConfiguration{
				ConcurrentSyncs: ptr.To(-1),
			},
			numExpectedErrors: 1,
			matcher:           ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeInvalid), "Field": Equal("controllers.etcdOpsTask.concurrentSyncs")}))),
		},
		{
			name: "should allow requeue interval greater than zero",
			config: &druidconfigv1alpha1.EtcdOpsTaskControllerConfiguration{
				RequeueInterval: &metav1.Duration{Duration: 2 * time.Second},
			},
			numExpectedErrors: 0,
		},
		{
			name: "should forbid requeue interval equal to zero",
			config: &druidconfigv1alpha1.EtcdOpsTaskControllerConfiguration{
				RequeueInterval: &metav1.Duration{Duration: 0},
			},
			numExpectedErrors: 1,
			matcher:           ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeInvalid), "Field": Equal("controllers.etcdOpsTask.requeueInterval")}))),
		},
		{
			name: "should forbid requeue interval less than zero",
			config: &druidconfigv1alpha1.EtcdOpsTaskControllerConfiguration{
				RequeueInterval: &metav1.Duration{Duration: -time.Second},
			},
			numExpectedErrors: 1,
			matcher:           ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeInvalid), "Field": Equal("controllers.etcdOpsTask.requeueInterval")}))),
		},
		{
			name: "should forbid both concurrent syncs and requeue interval equal to zero",
			config: &druidconfigv1alpha1.EtcdOpsTaskControllerConfiguration{
				ConcurrentSyncs: ptr.To(0),
				RequeueInterval: &metav1.Duration{Duration: 0},
			},
			numExpectedErrors: 2,
			matcher: ConsistOf(
				PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeInvalid), "Field": Equal("controllers.etcdOpsTask.concurrentSyncs")})),
				PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeInvalid), "Field": Equal("controllers.etcdOpsTask.requeueInterval")})),
			),
		},
	}

	g := NewWithT(t)
	t.Parallel()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			druidconfigv1alpha1.SetDefaults_EtcdOpsTaskControllerConfiguration(test.config)
			actualErrList := validateEtcdOpsTaskControllerConfiguration(*test.config, field.NewPath("controllers.etcdOpsTask"))
			g.Expect(len(actualErrList)).To(Equal(test.numExpectedErrors))
			if test.matcher != nil {
				g.Expect(actualErrList).To(test.matcher)
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
			controllerConfig := &druidconfigv1alpha1.SecretControllerConfiguration{}
			druidconfigv1alpha1.SetDefaults_SecretControllerConfiguration(controllerConfig)
			if test.concurrentSync != nil {
				controllerConfig.ConcurrentSyncs = test.concurrentSync
			}
			actualErrList := validateSecretControllerConfiguration(*controllerConfig, fldPath)
			g.Expect(len(actualErrList)).To(Equal(test.expectedErrors))
			if test.matcher != nil {
				g.Expect(actualErrList).To(test.matcher)
			}
		})
	}
}

func TestValidateEtcdComponentProtectionWebhookConfiguration(t *testing.T) {
	tests := []struct {
		name                                 string
		enabled                              *bool
		overrideReconcilerServiceAccountFQDN *string
		serviceAccountInfo                   *druidconfigv1alpha1.ServiceAccountInfo
		overrideExemptServiceAccounts        []string
		expectedErrors                       int
		matcher                              gomegatypes.GomegaMatcher
	}{
		{
			name:           "should allow empty configuration when it is disabled",
			enabled:        ptr.To(false),
			expectedErrors: 0,
			matcher:        nil,
		},
		{
			name:           "should allow empty configuration when enabled is unset",
			enabled:        nil,
			expectedErrors: 0,
			matcher:        nil,
		},
		{
			name:                                 "when enabled, should allow valid configuration using deprecated reconciler service account FQDN",
			enabled:                              ptr.To(true),
			overrideReconcilerServiceAccountFQDN: ptr.To(testServiceAccountFQDN),
			overrideExemptServiceAccounts:        []string{"garbage-collector-sa"},
			expectedErrors:                       0,
			matcher:                              nil,
		},
		{
			name:    "when enabled, should allow valid configuration using a valid service account info",
			enabled: ptr.To(true),
			serviceAccountInfo: &druidconfigv1alpha1.ServiceAccountInfo{
				Name:      testServiceAccountName,
				Namespace: testNs,
			},
			expectedErrors: 0,
			matcher:        nil,
		},
		{
			name:           "when enabled, should forbid nil values for both reconcilerServiceAccountFQDN and serviceAccountInfo",
			enabled:        ptr.To(true),
			expectedErrors: 1,
			matcher:        ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeRequired), "Field": Equal("webhooks.etcdComponentProtection")}))),
		},
		{
			name:    "when enabled, should forbid setting non-nil and non-empty values for both reconcilerServiceAccountFQDN and serviceAccountInfo",
			enabled: ptr.To(true),
			serviceAccountInfo: &druidconfigv1alpha1.ServiceAccountInfo{
				Name:      testServiceAccountName,
				Namespace: testNs,
			},
			overrideReconcilerServiceAccountFQDN: ptr.To(testServiceAccountFQDN),
			expectedErrors:                       1,
			matcher:                              ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeInvalid), "Field": Equal("webhooks.etcdComponentProtection")}))),
		},
		{
			name:                          "when enabled with non-nil serviceAccountInfo, should forbid empty values for namePath and namespacePath",
			enabled:                       ptr.To(true),
			overrideExemptServiceAccounts: []string{"garbage-collector-sa"},
			serviceAccountInfo:            &druidconfigv1alpha1.ServiceAccountInfo{},
			expectedErrors:                2,
			matcher: ConsistOf(
				PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeRequired), "Field": Equal("webhooks.etcdComponentProtection.serviceAccountInfo.name")})),
				PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeRequired), "Field": Equal("webhooks.etcdComponentProtection.serviceAccountInfo.namespace")})),
			),
		},
		{
			name:                                 "when enabled, with non-nil reconcilerServiceAccountFQDN and nil serviceAccountInfo, should forbid empty value for reconcilerServiceAccountFQDN",
			enabled:                              ptr.To(true),
			overrideReconcilerServiceAccountFQDN: ptr.To(""),
			expectedErrors:                       1,
			matcher:                              ConsistOf(PointTo(MatchFields(IgnoreExtras, Fields{"Type": Equal(field.ErrorTypeRequired), "Field": Equal("webhooks.etcdComponentProtection.reconcilerServiceAccountFQDN")}))),
		},
	}

	fldPath := field.NewPath("webhooks.etcdComponentProtection")
	g := NewWithT(t)
	t.Parallel()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			webhookConfig := &druidconfigv1alpha1.EtcdComponentProtectionWebhookConfiguration{}
			webhookConfig.Enabled = test.enabled
			if test.overrideExemptServiceAccounts != nil {
				webhookConfig.ExemptServiceAccounts = test.overrideExemptServiceAccounts
			}
			webhookConfig.ReconcilerServiceAccountFQDN = test.overrideReconcilerServiceAccountFQDN
			webhookConfig.ServiceAccountInfo = test.serviceAccountInfo
			actualErrList := validateEtcdComponentProtectionWebhookConfiguration(*webhookConfig, fldPath)
			g.Expect(len(actualErrList)).To(Equal(test.expectedErrors))
			if test.matcher != nil {
				g.Expect(actualErrList).To(test.matcher)
			}
		})
	}
}

func updateLeaderElectionConfig(config *druidconfigv1alpha1.LeaderElectionConfiguration,
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
