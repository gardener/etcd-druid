// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package opts

import (
	"fmt"
	"strings"
	"time"

	druidconfigv1alpha1 "github.com/gardener/etcd-druid/api/config/v1alpha1"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"

	flag "github.com/spf13/pflag"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type deprecatedOperatorConfiguration struct {
	metricsBindAddress                              string
	metricsPort                                     int
	webhookServerBindAddress                        string
	webhookServerPort                               int
	webhookServerTLSCertDir                         string
	leaderElectionEnabled                           bool
	leaderElectionResourceName                      string
	leaderElectionResourceLock                      string
	disableLeaseCache                               bool
	etcdWorkers                                     int
	etcdSpecAutoReconcile                           bool
	etcdDisableServiceAccountAutomount              bool
	etcdStatusSyncPeriod                            time.Duration
	etcdMemberNotReadyThreshold                     time.Duration
	etcdMemberUnknownThreshold                      time.Duration
	compactionEnabled                               bool
	compactionWorkers                               int
	compactionEventsThreshold                       int64
	compactionActiveDeadlineDuration                time.Duration
	compactionMetricsScrapeWaitDuration             time.Duration
	etcdCopyBackupsTaskEnabled                      bool
	etcdCopyBackupsTaskWorkers                      int
	secretWorkers                                   int
	etcdOpsTaskEnabled                              bool
	etcdOpsTaskWorkers                              int
	etcdOpsTaskRequeueInterval                      time.Duration
	etcdComponentProtectionEnabled                  bool
	etcdComponentProtectionReconcilerServiceAccount string
	etcdComponentProtectionExemptServiceAccounts    []string
}

func (d *deprecatedOperatorConfiguration) addDeprecatedFlags(fs *flag.FlagSet) {
	d.addDeprecatedControllerManagerFlags(fs)
	d.addDeprecatedEtcdControllerFlags(fs)
	d.addDeprecatedCompactionControllerFlags(fs)
	d.addDeprecatedEtcdCopyBackupsTaskControllerFlags(fs)
	d.addDeprecatedEtcdOpsTaskControllerFlags(fs)
	d.addDeprecatedSecretControllerFlags(fs)
	d.addDeprecatedEtcdComponentProtectionWebhookFlags(fs)
}

func (d *deprecatedOperatorConfiguration) addDeprecatedControllerManagerFlags(fs *flag.FlagSet) {
	fs.StringVar(&d.metricsBindAddress, "metrics-bind-address", "", "The IP address that the metrics endpoint binds to.")
	fs.IntVar(&d.metricsPort, "metrics-port", druidconfigv1alpha1.DefaultMetricsServerPort, "The port used for the metrics endpoint.")
	fs.StringVar(&d.webhookServerBindAddress, "webhook-server-bind-address", "", "The IP address on which to listen for the HTTPS webhook server.")
	fs.IntVar(&d.webhookServerPort, "webhook-server-port", druidconfigv1alpha1.DefaultWebhooksServerPort, "The port on which to listen for the HTTPS webhook server.")
	fs.StringVar(&d.webhookServerTLSCertDir, "webhook-server-tls-server-cert-dir", druidconfigv1alpha1.DefaultWebhooksServerTLSServerCertDir, "The path to a directory containing the server's TLS certificate and key (the files must be named tls.crt and tls.key respectively).")
	fs.BoolVar(&d.leaderElectionEnabled, "enable-leader-election", false, "Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")
	fs.StringVar(&d.leaderElectionResourceName, "leader-election-id", druidconfigv1alpha1.DefaultLeaderElectionResourceName, "Name of the resource that leader election will use for holding the leader lock.")
	fs.StringVar(&d.leaderElectionResourceLock, "leader-election-resource-lock", druidconfigv1alpha1.DefaultLeaderElectionResourceLock, "Specifies which resource type to use for leader election. Supported options are 'endpoints', 'configmaps', 'leases', 'endpointsleases' and 'configmapsleases'. : will be removed in the future in favour of using only `leases` as the leader election resource lock for the controller manager.")
	fs.BoolVar(&d.disableLeaseCache, "disable-lease-cache", false, "Disable cache for lease.coordination.k8s.io resources.")
}

func (d *deprecatedOperatorConfiguration) addDeprecatedEtcdControllerFlags(fs *flag.FlagSet) {
	fs.IntVar(&d.etcdWorkers, "etcd-workers", druidconfigv1alpha1.DefaultEtcdControllerConcurrentSyncs, "Number of workers spawned for concurrent reconciles of etcd spec and status changes. If not specified then default of 3 is assumed.")
	fs.BoolVar(&d.etcdSpecAutoReconcile, "enable-etcd-spec-auto-reconcile", false, fmt.Sprintf("If true then automatically reconciles Etcd Spec. If false, waits for explicit annotation `%s: %s` to be placed on the Etcd resource to trigger reconcile.", druidv1alpha1.DruidOperationAnnotation, druidv1alpha1.DruidOperationReconcile))
	fs.BoolVar(&d.etcdDisableServiceAccountAutomount, "disable-etcd-serviceaccount-automount", false, "If true then .automountServiceAccountToken will be set to false for the ServiceAccount created for etcd StatefulSets.")
	fs.DurationVar(&d.etcdStatusSyncPeriod, "etcd-status-sync-period", druidconfigv1alpha1.DefaultEtcdStatusSyncPeriod, "Period after which an etcd status sync will be attempted.")
	fs.DurationVar(&d.etcdMemberNotReadyThreshold, "etcd-member-notready-threshold", druidconfigv1alpha1.DefaultEtcdNotReadyThreshold, "Threshold after which an etcd member is considered not ready if the status was unknown before.")
	fs.DurationVar(&d.etcdMemberUnknownThreshold, "etcd-member-unknown-threshold", druidconfigv1alpha1.DefaultEtcdUnknownThreshold, "Threshold after which an etcd member is considered unknown.")
}

func (d *deprecatedOperatorConfiguration) addDeprecatedCompactionControllerFlags(fs *flag.FlagSet) {
	fs.BoolVar(&d.compactionEnabled, "enable-backup-compaction", false, "Enable automatic compaction of etcd backups.")
	fs.IntVar(&d.compactionWorkers, "compaction-workers", druidconfigv1alpha1.DefaultCompactionConcurrentSyncs, "Number of worker threads of the CompactionJob controller. The controller creates a backup compaction job if a certain etcd event threshold is reached. If compaction is enabled, the value for this flag must be greater than zero.")
	fs.Int64Var(&d.compactionEventsThreshold, "etcd-events-threshold", druidconfigv1alpha1.DefaultCompactionEventsThreshold, "Total number of etcd events that can be allowed before a backup compaction job is triggered.")
	fs.DurationVar(&d.compactionActiveDeadlineDuration, "active-deadline-duration", druidconfigv1alpha1.DefaultCompactionActiveDeadlineDuration, "Duration after which a running backup compaction job will be terminated.")
	fs.DurationVar(&d.compactionMetricsScrapeWaitDuration, "metrics-scrape-wait-duration", 0, "Duration to wait for after compaction job is completed, to allow Prometheus metrics to be scraped.")
}

func (d *deprecatedOperatorConfiguration) addDeprecatedEtcdCopyBackupsTaskControllerFlags(fs *flag.FlagSet) {
	// For backward compatibility reasons the default chosen is true.
	fs.BoolVar(&d.etcdCopyBackupsTaskEnabled, "enable-etcd-copy-backups-task", true, "Enable the etcd-copy-backups-task controller.")
	fs.IntVar(&d.etcdCopyBackupsTaskWorkers, "etcd-copy-backups-task-workers", druidconfigv1alpha1.DefaultEtcdCopyBackupsTaskConcurrentSyncs, "Number of worker threads for the etcdcopybackupstask controller.")
}

func (d *deprecatedOperatorConfiguration) addDeprecatedEtcdOpsTaskControllerFlags(fs *flag.FlagSet) {
	fs.BoolVar(&d.etcdOpsTaskEnabled, "enable-etcd-ops-task", true, "Enable the etcd-ops-task controller.")
	fs.IntVar(&d.etcdOpsTaskWorkers, "etcd-ops-task-workers", druidconfigv1alpha1.DefaultEtcdOpsTaskControllerConcurrentSyncs, "Number of worker threads for the etcd-ops-task controller.")
	fs.DurationVar(&d.etcdOpsTaskRequeueInterval, "etcd-ops-task-requeue-interval", druidconfigv1alpha1.DefaultEtcdOpsRequeueInterval, "Requeue interval for the etcd-ops-task controller.")
}

func (d *deprecatedOperatorConfiguration) addDeprecatedSecretControllerFlags(fs *flag.FlagSet) {
	fs.IntVar(&d.secretWorkers, "secret-workers", druidconfigv1alpha1.DefaultSecretControllerConcurrentSyncs, "Number of worker threads for the secrets controller.")
}

func (d *deprecatedOperatorConfiguration) addDeprecatedEtcdComponentProtectionWebhookFlags(fs *flag.FlagSet) {
	fs.BoolVar(&d.etcdComponentProtectionEnabled, "enable-etcd-components-webhook", false, "Enable Etcd-Components-Webhook to prevent unintended changes to resources managed by etcd-druid.")
	fs.StringVar(&d.etcdComponentProtectionReconcilerServiceAccount, "reconciler-service-account", "", "The fully qualified name of the service account used by etcd-druid for reconciling etcd resources. Deprecated and will be removed in a future release.")
	fs.StringSliceVar(&d.etcdComponentProtectionExemptServiceAccounts, "etcd-components-webhook-exempt-service-accounts", []string{}, "The comma-separated list of fully qualified names of service accounts that are exempt from Etcd-Components-Webhook checks.")
}

func (d *deprecatedOperatorConfiguration) ToOperatorConfiguration() *druidconfigv1alpha1.OperatorConfiguration {
	config := &druidconfigv1alpha1.OperatorConfiguration{
		TypeMeta: metav1.TypeMeta{
			APIVersion: druidconfigv1alpha1.SchemeGroupVersion.String(),
			Kind:       "OperatorConfiguration",
		},
		Server: druidconfigv1alpha1.ServerConfiguration{
			Metrics:  &druidconfigv1alpha1.Server{},
			Webhooks: &druidconfigv1alpha1.TLSServer{},
		},
	}
	config.Server.Metrics.BindAddress = d.metricsBindAddress
	config.Server.Metrics.Port = d.metricsPort
	config.Server.Webhooks.BindAddress = d.webhookServerBindAddress
	config.Server.Webhooks.Port = d.webhookServerPort
	config.Server.Webhooks.ServerCertDir = d.webhookServerTLSCertDir
	config.LeaderElection.Enabled = d.leaderElectionEnabled
	config.LeaderElection.ResourceName = d.leaderElectionResourceName
	config.LeaderElection.ResourceLock = d.leaderElectionResourceLock
	config.Controllers.DisableLeaseCache = d.disableLeaseCache
	config.Controllers.Etcd.ConcurrentSyncs = &d.etcdWorkers
	config.Controllers.Etcd.EnableEtcdSpecAutoReconcile = d.etcdSpecAutoReconcile
	config.Controllers.Etcd.DisableEtcdServiceAccountAutomount = d.etcdDisableServiceAccountAutomount
	config.Controllers.Etcd.EtcdStatusSyncPeriod = metav1.Duration{Duration: d.etcdStatusSyncPeriod}
	config.Controllers.Etcd.EtcdMember.NotReadyThreshold = metav1.Duration{Duration: d.etcdMemberNotReadyThreshold}
	config.Controllers.Etcd.EtcdMember.UnknownThreshold = metav1.Duration{Duration: d.etcdMemberUnknownThreshold}
	config.Controllers.Compaction.Enabled = d.compactionEnabled
	config.Controllers.Compaction.ConcurrentSyncs = &d.compactionWorkers
	config.Controllers.Compaction.EventsThreshold = d.compactionEventsThreshold
	config.Controllers.Compaction.ActiveDeadlineDuration = metav1.Duration{Duration: d.compactionActiveDeadlineDuration}
	config.Controllers.Compaction.MetricsScrapeWaitDuration = metav1.Duration{Duration: d.compactionMetricsScrapeWaitDuration}
	config.Controllers.EtcdCopyBackupsTask.Enabled = d.etcdCopyBackupsTaskEnabled
	config.Controllers.EtcdCopyBackupsTask.ConcurrentSyncs = &d.etcdCopyBackupsTaskWorkers
	config.Controllers.EtcdOpsTask.ConcurrentSyncs = &d.etcdOpsTaskWorkers
	config.Controllers.EtcdOpsTask.RequeueInterval = &metav1.Duration{Duration: d.etcdOpsTaskRequeueInterval}
	config.Controllers.Secret.ConcurrentSyncs = &d.secretWorkers
	config.Webhooks.EtcdComponentProtection.Enabled = d.etcdComponentProtectionEnabled
	if len(strings.TrimSpace(d.etcdComponentProtectionReconcilerServiceAccount)) > 0 {
		config.Webhooks.EtcdComponentProtection.ReconcilerServiceAccountFQDN = &d.etcdComponentProtectionReconcilerServiceAccount
	}
	config.Webhooks.EtcdComponentProtection.ExemptServiceAccounts = d.etcdComponentProtectionExemptServiceAccounts
	return config
}
