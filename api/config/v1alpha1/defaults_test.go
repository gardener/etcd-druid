// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	. "github.com/onsi/gomega"
)

func TestSetDefaults_LeaderElectionConfiguration(t *testing.T) {
	tests := []struct {
		name     string
		config   *LeaderElectionConfiguration
		expected *LeaderElectionConfiguration
	}{
		{
			name:     "should correctly set default values when leader election is disabled",
			config:   &LeaderElectionConfiguration{},
			expected: &LeaderElectionConfiguration{},
		},
		{
			name: "should correctly set default values when leader election is enabled",
			config: &LeaderElectionConfiguration{
				Enabled: true,
			},
			expected: &LeaderElectionConfiguration{
				Enabled:       true,
				ResourceLock:  "leases",
				ResourceName:  "druid-leader-election",
				LeaseDuration: metav1.Duration{Duration: 15 * time.Second},
				RenewDeadline: metav1.Duration{Duration: 10 * time.Second},
				RetryPeriod:   metav1.Duration{Duration: 2 * time.Second},
			},
		},
		{
			name: "should not overwrite already set values",
			config: &LeaderElectionConfiguration{
				Enabled:       true,
				ResourceLock:  "endpoints",
				ResourceName:  "custom-resource-name",
				LeaseDuration: metav1.Duration{Duration: 20 * time.Second},
			},
			expected: &LeaderElectionConfiguration{
				Enabled:       true,
				ResourceLock:  "endpoints",
				ResourceName:  "custom-resource-name",
				LeaseDuration: metav1.Duration{Duration: 20 * time.Second},
				RenewDeadline: metav1.Duration{Duration: 10 * time.Second},
				RetryPeriod:   metav1.Duration{Duration: 2 * time.Second},
			},
		},
	}

	g := NewWithT(t)
	t.Parallel()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			SetDefaults_LeaderElectionConfiguration(test.config)
			g.Expect(test.config).To(Equal(test.expected))
		})
	}
}

func TestSetDefaults_ClientConnectionConfiguration(t *testing.T) {
	tests := []struct {
		name     string
		config   *ClientConnectionConfiguration
		expected *ClientConnectionConfiguration
	}{
		{
			name:     "should correctly set default values",
			config:   &ClientConnectionConfiguration{},
			expected: &ClientConnectionConfiguration{QPS: 100.0, Burst: 150, ContentType: "", AcceptContentTypes: ""},
		},
		{
			name: "should not overwrite already set values",
			config: &ClientConnectionConfiguration{
				QPS:         50.0,
				Burst:       100,
				ContentType: "application/json",
			},
			expected: &ClientConnectionConfiguration{
				QPS:                50.0,
				Burst:              100,
				ContentType:        "application/json",
				AcceptContentTypes: "",
			},
		},
		{
			name: "should not default ContentType and AcceptContentTypes",
			config: &ClientConnectionConfiguration{
				QPS:   50.0,
				Burst: 100,
			},
			expected: &ClientConnectionConfiguration{
				QPS:                50.0,
				Burst:              100,
				ContentType:        "",
				AcceptContentTypes: "",
			},
		},
	}

	g := NewWithT(t)
	t.Parallel()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			SetDefaults_ClientConnectionConfiguration(test.config)
			g.Expect(test.config).To(Equal(test.expected))
		})
	}
}

func TestSetDefaults_ServerConfiguration(t *testing.T) {
	tests := []struct {
		name     string
		config   *ServerConfiguration
		expected *ServerConfiguration
	}{
		{
			name:   "should correctly set default values",
			config: &ServerConfiguration{},
			expected: &ServerConfiguration{
				Webhooks: &TLSServer{
					Server:        Server{BindAddress: "", Port: 9443},
					ServerCertDir: "/etc/webhook-server-tls",
				},
				Metrics: &Server{BindAddress: "", Port: 8080},
			},
		},
		{
			name: "should not overwrite already set values",
			config: &ServerConfiguration{
				Webhooks: &TLSServer{
					Server:        Server{BindAddress: "", Port: 2784},
					ServerCertDir: "/var/etc/ssl/webhook-server-tls",
				},
			},
			expected: &ServerConfiguration{
				Webhooks: &TLSServer{
					Server:        Server{BindAddress: "", Port: 2784},
					ServerCertDir: "/var/etc/ssl/webhook-server-tls",
				},
				Metrics: &Server{BindAddress: "", Port: 8080},
			},
		},
	}

	g := NewWithT(t)
	t.Parallel()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			SetDefaults_ServerConfiguration(test.config)
			g.Expect(test.config).To(Equal(test.expected))
		})
	}
}

func TestSetDefaults_EtcdControllerConfiguration(t *testing.T) {
	tests := []struct {
		name     string
		config   *EtcdControllerConfiguration
		expected *EtcdControllerConfiguration
	}{
		{
			name:   "should correctly set default values",
			config: &EtcdControllerConfiguration{},
			expected: &EtcdControllerConfiguration{
				ConcurrentSyncs:      ptr.To(3),
				EtcdStatusSyncPeriod: metav1.Duration{Duration: 15 * time.Second},
				EtcdMember: EtcdMemberConfiguration{
					NotReadyThreshold: metav1.Duration{Duration: 5 * time.Minute},
					UnknownThreshold:  metav1.Duration{Duration: 1 * time.Minute},
				},
			},
		},
		{
			name: "should not overwrite already set values",
			config: &EtcdControllerConfiguration{
				ConcurrentSyncs:      ptr.To(5),
				EtcdStatusSyncPeriod: metav1.Duration{Duration: 30 * time.Second},
				EtcdMember: EtcdMemberConfiguration{
					NotReadyThreshold: metav1.Duration{Duration: 10 * time.Minute},
				},
			},
			expected: &EtcdControllerConfiguration{
				ConcurrentSyncs:      ptr.To(5),
				EtcdStatusSyncPeriod: metav1.Duration{Duration: 30 * time.Second},
				EtcdMember: EtcdMemberConfiguration{
					NotReadyThreshold: metav1.Duration{Duration: 10 * time.Minute},
					UnknownThreshold:  metav1.Duration{Duration: 1 * time.Minute},
				},
			},
		},
	}

	g := NewWithT(t)
	t.Parallel()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			SetDefaults_EtcdControllerConfiguration(test.config)
			g.Expect(test.config).To(Equal(test.expected))
		})
	}
}

func TestSetDefaults_SecretControllerConfiguration(t *testing.T) {
	tests := []struct {
		name     string
		config   *SecretControllerConfiguration
		expected *SecretControllerConfiguration
	}{
		{
			name:   "should correctly set default values",
			config: &SecretControllerConfiguration{},
			expected: &SecretControllerConfiguration{
				ConcurrentSyncs: ptr.To(10),
			},
		},
		{
			name: "should not overwrite already set values",
			config: &SecretControllerConfiguration{
				ConcurrentSyncs: ptr.To(5),
			},
			expected: &SecretControllerConfiguration{
				ConcurrentSyncs: ptr.To(5),
			},
		},
	}

	g := NewWithT(t)
	t.Parallel()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			SetDefaults_SecretControllerConfiguration(test.config)
			g.Expect(test.config).To(Equal(test.expected))
		})
	}
}

func TestSetDefaults_CompactionControllerConfiguration(t *testing.T) {
	tests := []struct {
		name     string
		config   *CompactionControllerConfiguration
		expected *CompactionControllerConfiguration
	}{
		{
			name:     "should not set default values when not enabled",
			config:   &CompactionControllerConfiguration{},
			expected: &CompactionControllerConfiguration{},
		},
		{
			name: "should correctly set default values when enabled is true",
			config: &CompactionControllerConfiguration{
				Enabled: true,
			},
			expected: &CompactionControllerConfiguration{
				Enabled:                      true,
				ConcurrentSyncs:              ptr.To(3),
				EventsThreshold:              1000000,
				TriggerFullSnapshotThreshold: 3000000,
				ActiveDeadlineDuration:       metav1.Duration{Duration: 3 * time.Hour},
			},
		},
		{
			name: "should not overwrite already set values",
			config: &CompactionControllerConfiguration{
				Enabled:                      true,
				ConcurrentSyncs:              ptr.To(5),
				TriggerFullSnapshotThreshold: 2000000,
				ActiveDeadlineDuration:       metav1.Duration{Duration: 1 * time.Hour},
			},
			expected: &CompactionControllerConfiguration{
				Enabled:                      true,
				ConcurrentSyncs:              ptr.To(5),
				EventsThreshold:              1000000,
				TriggerFullSnapshotThreshold: 2000000,
				ActiveDeadlineDuration:       metav1.Duration{Duration: 1 * time.Hour},
			},
		},
	}

	g := NewWithT(t)
	t.Parallel()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			SetDefaults_CompactionControllerConfiguration(test.config)
			g.Expect(test.config).To(Equal(test.expected))
		})
	}
}

func TestSetDefaults_EtcdCopyBackupsTaskControllerConfiguration(t *testing.T) {
	tests := []struct {
		name     string
		config   *EtcdCopyBackupsTaskControllerConfiguration
		expected *EtcdCopyBackupsTaskControllerConfiguration
	}{
		{
			name:     "should not set default values when not enabled",
			config:   &EtcdCopyBackupsTaskControllerConfiguration{},
			expected: &EtcdCopyBackupsTaskControllerConfiguration{},
		},
		{
			name: "should correctly set default values when enabled is true",
			config: &EtcdCopyBackupsTaskControllerConfiguration{
				Enabled: true,
			},
			expected: &EtcdCopyBackupsTaskControllerConfiguration{
				Enabled:         true,
				ConcurrentSyncs: ptr.To(3),
			},
		},
		{
			name: "should not overwrite already set values",
			config: &EtcdCopyBackupsTaskControllerConfiguration{
				Enabled:         true,
				ConcurrentSyncs: ptr.To(5),
			},
			expected: &EtcdCopyBackupsTaskControllerConfiguration{
				Enabled:         true,
				ConcurrentSyncs: ptr.To(5),
			},
		},
	}

	g := NewWithT(t)
	t.Parallel()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			SetDefaults_EtcdCopyBackupsTaskControllerConfiguration(test.config)
			g.Expect(test.config).To(Equal(test.expected))
		})
	}
}

func TestSetDefaults_LogConfig(t *testing.T) {
	tests := []struct {
		name     string
		config   *LogConfiguration
		expected *LogConfiguration
	}{
		{
			name:     "should correctly set default values",
			config:   &LogConfiguration{},
			expected: &LogConfiguration{LogLevel: LogLevelInfo, LogFormat: LogFormatJSON},
		},
		{
			name: "should not overwrite already set values",
			config: &LogConfiguration{
				LogLevel:  LogLevelError,
				LogFormat: LogFormatText,
			},
			expected: &LogConfiguration{
				LogLevel:  LogLevelError,
				LogFormat: LogFormatText,
			},
		},
	}

	g := NewWithT(t)
	t.Parallel()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			SetDefaults_LogConfiguration(test.config)
			g.Expect(test.config).To(Equal(test.expected))
		})
	}
}
