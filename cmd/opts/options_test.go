// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package opts

import (
	configv1alpha1 "github.com/gardener/etcd-druid/api/config/v1alpha1"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	"github.com/spf13/pflag"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
	"time"
)

var zero = metav1.Duration{Duration: 0}

func TestCompleteWithDeprecatedFlags(t *testing.T) {
	g := NewWithT(t)
	fs := pflag.NewFlagSet("etcd-druid", pflag.ExitOnError)
	cliOpts := NewCLIOptions(fs)
	args := []string{
		"--metrics-port=8080",
		"--enable-leader-election=true",
		"--webhook-server-port=9443",
		"--disable-lease-cache=true",
		"--etcd-workers=5",
		"--etcd-status-sync-period=15s",
		"--enable-backup-compaction=true",
		"--compaction-workers=3",
		"--metrics-scrape-wait-duration=0s",
		"--secret-workers=15",
	}
	g.Expect(fs.Parse(args)).To(Succeed())
	g.Expect(cliOpts.Complete()).To(Succeed())
	cfg := cliOpts.Config
	g.Expect(cliOpts.configFile).To(BeEmpty())
	g.Expect(cfg).ToNot(BeNil())
	// assert explicitly configured flags have been set correctly
	g.Expect(cfg.LeaderElection.Enabled).To(BeTrue())
	g.Expect(cfg.Server.Metrics.Port).To(Equal(8080))
	g.Expect(cfg.Server.Webhooks.Port).To(Equal(9443))
	g.Expect(cfg.Controllers.Etcd.EtcdStatusSyncPeriod).To(Equal(metav1.Duration{Duration: 15 * time.Second}))
	g.Expect(cfg.Controllers.Compaction.Enabled).To(BeTrue())
	g.Expect(cfg.Controllers.DisableLeaseCache).To(BeTrue())
	g.Expect(cfg.Controllers.Compaction.MetricsScrapeWaitDuration).To(Equal(zero))
	// assert that defaulting functions are called and the defaults are set correctly
	g.Expect(cfg.LeaderElection.LeaseDuration).To(Equal(metav1.Duration{Duration: 15 * time.Second}))
	g.Expect(cfg.LeaderElection.RenewDeadline).To(Equal(metav1.Duration{Duration: 10 * time.Second}))
	g.Expect(cfg.LeaderElection.RetryPeriod).To(Equal(metav1.Duration{Duration: 2 * time.Second}))
	// assert deprecated worker flags have been set correctly to the concurrent syncs of the respective controllers
	g.Expect(cfg.Controllers.Etcd.ConcurrentSyncs).To(PointTo(Equal(5)))
	g.Expect(cfg.Controllers.Compaction.ConcurrentSyncs).To(PointTo(Equal(3)))
	g.Expect(cfg.Controllers.Secret.ConcurrentSyncs).To(PointTo(Equal(15)))
}

func TestCompleteWithConfigFlag(t *testing.T) {
	g := NewWithT(t)
	fs := pflag.NewFlagSet("etcd-druid", pflag.ExitOnError)
	cliOpts := NewCLIOptions(fs)
	args := []string{
		"--config=./testdata/operatorconfig.yaml",
	}
	g.Expect(fs.Parse(args)).To(Succeed())
	g.Expect(cliOpts.Complete()).To(Succeed())
	g.Expect(cliOpts.configFile).To(Equal("./testdata/operatorconfig.yaml"))
	g.Expect(cliOpts.Config).ToNot(BeNil())
	cfg := cliOpts.Config
	g.Expect(cfg.ClientConnection.QPS).To(Equal(float32(150)))
	g.Expect(cfg.ClientConnection.Burst).To(Equal(250))
	g.Expect(cfg.LeaderElection.Enabled).To(BeTrue())
	g.Expect(cfg.LeaderElection.LeaseDuration).To(Equal(metav1.Duration{Duration: 25 * time.Second}))
	g.Expect(cfg.LeaderElection.RenewDeadline).To(Equal(metav1.Duration{Duration: 15 * time.Second}))
	g.Expect(cfg.LeaderElection.RetryPeriod).To(Equal(metav1.Duration{Duration: 5 * time.Second}))
	g.Expect(cfg.Server.Webhooks.Port).To(Equal(2750))
	g.Expect(cfg.Server.Webhooks.ServerCertDir).To(Equal("/var/etcd/ssl/webhook-server-tls"))
	g.Expect(cfg.Server.Metrics.Port).To(Equal(2751))
	g.Expect(cfg.Controllers.Etcd.ConcurrentSyncs).To(PointTo(Equal(10)))
	g.Expect(cfg.Controllers.Etcd.DisableEtcdServiceAccountAutomount).To(BeTrue())
	g.Expect(cfg.Controllers.Etcd.EtcdStatusSyncPeriod).To(Equal(metav1.Duration{Duration: 20 * time.Second}))
	g.Expect(cfg.Controllers.Etcd.EtcdMember.NotReadyThreshold).To(Equal(metav1.Duration{Duration: 7 * time.Minute}))
	g.Expect(cfg.Controllers.Compaction.Enabled).To(BeTrue())
	g.Expect(cfg.Controllers.Compaction.ConcurrentSyncs).To(PointTo(Equal(3)))
	g.Expect(cfg.Controllers.Compaction.EventsThreshold).To(Equal(int64(1500000)))
	g.Expect(cfg.Controllers.EtcdCopyBackupsTask.Enabled).To(BeTrue())
	g.Expect(cfg.Controllers.EtcdCopyBackupsTask.ConcurrentSyncs).To(PointTo(Equal(2)))
	g.Expect(cfg.Controllers.Secret.ConcurrentSyncs).To(PointTo(Equal(5)))
	g.Expect(cfg.LogConfiguration.LogFormat).To(Equal(configv1alpha1.LogFormatText))
	// assert that defaulting functions are called and the defaults are set correctly for fields that are not set in the config file.
	g.Expect(cfg.LeaderElection.ResourceLock).To(Equal("leases"))
	g.Expect(cfg.LeaderElection.ResourceName).To(Equal("druid-leader-election"))
	g.Expect(cfg.Controllers.Etcd.EnableEtcdSpecAutoReconcile).To(BeFalse())
	g.Expect(cfg.Controllers.Etcd.EtcdMember.UnknownThreshold).To(Equal(metav1.Duration{Duration: 1 * time.Minute}))
	g.Expect(cfg.Controllers.Compaction.ActiveDeadlineDuration).To(Equal(metav1.Duration{Duration: 3 * time.Hour}))
	g.Expect(cfg.Controllers.Compaction.MetricsScrapeWaitDuration).To(Equal(zero))
	g.Expect(cfg.Webhooks.EtcdComponentProtection.Enabled).To(BeFalse())
	g.Expect(cfg.LogConfiguration.LogLevel).To(Equal(configv1alpha1.LogLevelInfo))
}

func TestCompleteWithConfigFlagAndDeprecatedFlags(t *testing.T) {
	g := NewWithT(t)
	fs := pflag.NewFlagSet("etcd-druid", pflag.ExitOnError)
	cliOpts := NewCLIOptions(fs)
	args := []string{
		"--config=./testdata/operatorconfig.yaml",
		"--etcd-workers=5",              // this is also defined with a different value in the config file
		"--active-deadline-duration=1h", // this is not defined in the config file. It should use the default value instead of 3h
	}

	g.Expect(fs.Parse(args)).To(Succeed())
	g.Expect(cliOpts.Complete()).To(Succeed())
	g.Expect(cliOpts.configFile).To(Equal("./testdata/operatorconfig.yaml"))
	g.Expect(cliOpts.Config).ToNot(BeNil())
	cfg := cliOpts.Config
	g.Expect(cfg.ClientConnection.QPS).To(Equal(float32(150)))
	g.Expect(cfg.ClientConnection.Burst).To(Equal(250))
	g.Expect(cfg.LeaderElection.Enabled).To(BeTrue())
	g.Expect(cfg.LeaderElection.LeaseDuration).To(Equal(metav1.Duration{Duration: 25 * time.Second}))
	g.Expect(cfg.LeaderElection.RenewDeadline).To(Equal(metav1.Duration{Duration: 15 * time.Second}))
	g.Expect(cfg.LeaderElection.RetryPeriod).To(Equal(metav1.Duration{Duration: 5 * time.Second}))
	g.Expect(cfg.Server.Webhooks.Port).To(Equal(2750))
	g.Expect(cfg.Server.Webhooks.ServerCertDir).To(Equal("/var/etcd/ssl/webhook-server-tls"))
	g.Expect(cfg.Server.Metrics.Port).To(Equal(2751))
	g.Expect(cfg.Controllers.Etcd.ConcurrentSyncs).To(PointTo(Equal(10)))
	g.Expect(cfg.Controllers.Etcd.DisableEtcdServiceAccountAutomount).To(BeTrue())
	g.Expect(cfg.Controllers.Etcd.EtcdStatusSyncPeriod).To(Equal(metav1.Duration{Duration: 20 * time.Second}))
	g.Expect(cfg.Controllers.Etcd.EtcdMember.NotReadyThreshold).To(Equal(metav1.Duration{Duration: 7 * time.Minute}))
	g.Expect(cfg.Controllers.Compaction.Enabled).To(BeTrue())
	g.Expect(cfg.Controllers.Compaction.ConcurrentSyncs).To(PointTo(Equal(3)))
	g.Expect(cfg.Controllers.Compaction.EventsThreshold).To(Equal(int64(1500000)))
	g.Expect(cfg.Controllers.EtcdCopyBackupsTask.Enabled).To(BeTrue())
	g.Expect(cfg.Controllers.EtcdCopyBackupsTask.ConcurrentSyncs).To(PointTo(Equal(2)))
	g.Expect(cfg.Controllers.Secret.ConcurrentSyncs).To(PointTo(Equal(5)))
	g.Expect(cfg.LogConfiguration.LogFormat).To(Equal(configv1alpha1.LogFormatText))
	// assert that defaulting functions are called and the defaults are set correctly for fields that are not set in the config file.
	g.Expect(cfg.LeaderElection.ResourceLock).To(Equal("leases"))
	g.Expect(cfg.LeaderElection.ResourceName).To(Equal("druid-leader-election"))
	g.Expect(cfg.Controllers.Etcd.EnableEtcdSpecAutoReconcile).To(BeFalse())
	g.Expect(cfg.Controllers.Etcd.EtcdMember.UnknownThreshold).To(Equal(metav1.Duration{Duration: 1 * time.Minute}))
	g.Expect(cfg.Controllers.Compaction.ActiveDeadlineDuration).To(Equal(metav1.Duration{Duration: 3 * time.Hour}))
	g.Expect(cfg.Controllers.Compaction.MetricsScrapeWaitDuration).To(Equal(zero))
	g.Expect(cfg.Webhooks.EtcdComponentProtection.Enabled).To(BeFalse())
	g.Expect(cfg.LogConfiguration.LogLevel).To(Equal(configv1alpha1.LogLevelInfo))
}
