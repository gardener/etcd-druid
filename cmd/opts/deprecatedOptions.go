package opts

import (
	"fmt"
	configv1alpha1 "github.com/gardener/etcd-druid/api/config/v1alpha1"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	flag "github.com/spf13/pflag"
)

func (o *CLIOptions) addDeprecatedFlags(fs *flag.FlagSet) {
	o.addDeprecatedControllerManagerFlags(fs)
	o.addDeprecatedEtcdControllerFlags(fs)
	o.addDeprecatedCompactionControllerFlags(fs)
	o.addDeprecatedEtcdCopyBackupsTaskControllerFlags(fs)
	o.addDeprecatedSecretControllerFlags(fs)
	o.addDeprecatedEtcdComponentProtectionWebhookFlags(fs)
}

func (o *CLIOptions) addDeprecatedControllerManagerFlags(fs *flag.FlagSet) {
	flag.StringVar(&o.Config.Server.Metrics.BindAddress, "metrics-bind-address", "", "The IP address that the metrics endpoint binds to.")
	flag.IntVar(&o.Config.Server.Metrics.Port, "metrics-port", configv1alpha1.DefaultMetricsServerPort, "The port used for the metrics endpoint.")
	flag.StringVar(&o.Config.Server.Webhook.Server.BindAddress, "webhook-server-bind-address", "", "The IP address on which to listen for the HTTPS webhook server.")
	flag.IntVar(&o.Config.Server.Webhook.Server.Port, "webhook-server-port", configv1alpha1.DefaultWebhookServerPort, "The port on which to listen for the HTTPS webhook server.")
	flag.StringVar(&o.Config.Server.Webhook.TLSConfig.ServerCertDir, "webhook-server-tls-server-cert-dir", configv1alpha1.DefaultWebhookServerTLSServerCertDir, "The path to a directory containing the server's TLS certificate and key (the files must be named tls.crt and tls.key respectively).")
	flag.BoolVar(&o.Config.LeaderElection.Enabled, "enable-leader-election", false, "Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&o.Config.LeaderElection.ResourceName, "leader-election-id", configv1alpha1.DefaultLeaderElectionResourceName, "Name of the resource that leader election will use for holding the leader lock.")
	flag.StringVar(&o.Config.LeaderElection.ResourceLock, "leader-election-resource-lock", configv1alpha1.DefaultLeaderElectionResourceLock, "Specifies which resource type to use for leader election. Supported options are 'endpoints', 'configmaps', 'leases', 'endpointsleases' and 'configmapsleases'. Deprecated: will be removed in the future in favour of using only `leases` as the leader election resource lock for the controller manager.")
	flag.BoolVar(&o.Config.Controllers.DisableLeaseCache, "disable-lease-cache", false, "Disable cache for lease.coordination.k8s.io resources.")
}

func (o *CLIOptions) addDeprecatedEtcdControllerFlags(fs *flag.FlagSet) {
	fs.IntVar(o.Config.Controllers.Etcd.ConcurrentSyncs, "etcd-workers", configv1alpha1.DefaultEtcdControllerConcurrentSyncs, "Number of workers spawned for concurrent reconciles of etcd spec and status changes. If not specified then default of 3 is assumed.")
	flag.BoolVar(&o.Config.Controllers.Etcd.EnableEtcdSpecAutoReconcile, "enable-etcd-spec-auto-reconcile", false, fmt.Sprintf("If true then automatically reconciles Etcd Spec. If false, waits for explicit annotation `%s: %s` to be placed on the Etcd resource to trigger reconcile.", druidv1alpha1.DruidOperationAnnotation, druidv1alpha1.DruidOperationReconcile))
	fs.BoolVar(&o.Config.Controllers.Etcd.DisableEtcdServiceAccountAutomount, "disable-etcd-serviceaccount-automount", false, "If true then .automountServiceAccountToken will be set to false for the ServiceAccount created for etcd StatefulSets.")
	fs.DurationVar(&o.Config.Controllers.Etcd.EtcdStatusSyncPeriod.Duration, "etcd-status-sync-period", configv1alpha1.DefaultEtcdStatusSyncPeriod, "Period after which an etcd status sync will be attempted.")
	fs.DurationVar(&o.Config.Controllers.Etcd.EtcdMemberConfig.NotReadyThreshold.Duration, "etcd-member-notready-threshold", configv1alpha1.DefaultEtcdNotReadyThreshold, "Threshold after which an etcd member is considered not ready if the status was unknown before.")
	fs.DurationVar(&o.Config.Controllers.Etcd.EtcdMemberConfig.UnknownThreshold.Duration, "etcd-member-unknown-threshold", configv1alpha1.DefaultEtcdUnknownThreshold, "Threshold after which an etcd member is considered unknown.")
}

func (o *CLIOptions) addDeprecatedCompactionControllerFlags(fs *flag.FlagSet) {
	fs.BoolVar(&o.Config.Controllers.Compaction.Enabled, "enable-backup-compaction", false, "Enable automatic compaction of etcd backups.")
	fs.IntVar(o.Config.Controllers.Compaction.ConcurrentSyncs, "compaction-workers", configv1alpha1.DefaultCompactionConcurrentSyncs, "Number of worker threads of the CompactionJob controller. The controller creates a backup compaction job if a certain etcd event threshold is reached. If compaction is enabled, the value for this flag must be greater than zero.")
	fs.Int64Var(&o.Config.Controllers.Compaction.EventsThreshold, "etcd-events-threshold", configv1alpha1.DefaultCompactionEventsThreshold, "Total number of etcd events that can be allowed before a backup compaction job is triggered.")
	fs.DurationVar(&o.Config.Controllers.Compaction.ActiveDeadlineDuration.Duration, "active-deadline-duration", configv1alpha1.DefaultCompactionActiveDeadlineDuration, "Duration after which a running backup compaction job will be terminated.")
	fs.DurationVar(&o.Config.Controllers.Compaction.MetricsScrapeWaitDuration.Duration, "metrics-scrape-wait-duration", 0, "Duration to wait for after compaction job is completed, to allow Prometheus metrics to be scraped.")
}

func (o *CLIOptions) addDeprecatedEtcdCopyBackupsTaskControllerFlags(fs *flag.FlagSet) {
	fs.IntVar(o.Config.Controllers.EtcdCopyBackupsTask.ConcurrentSyncs, "etcd-copy-backups-task-workers", configv1alpha1.DefaultEtcdCopyBackupsTaskConcurrentSyncs, "Number of worker threads for the etcdcopybackupstask controller.")
}

func (o *CLIOptions) addDeprecatedSecretControllerFlags(fs *flag.FlagSet) {
	fs.IntVar(o.Config.Controllers.Secret.ConcurrentSyncs, "secret-workers", configv1alpha1.DefaultSecretControllerConcurrentSyncs, "Number of worker threads for the secrets controller.")
}

func (o *CLIOptions) addDeprecatedEtcdComponentProtectionWebhookFlags(fs *flag.FlagSet) {
	fs.BoolVar(&o.Config.Webhooks.EtcdComponentProtection.Enabled, "enable-etcd-components-webhook", false, "Enable Etcd-Components-Webhook to prevent unintended changes to resources managed by etcd-druid.")
	fs.StringVar(&o.Config.Webhooks.EtcdComponentProtection.ReconcilerServiceAccount, "reconciler-service-account", "", "The fully qualified name of the service account used by etcd-druid for reconciling etcd resources.")
	fs.StringSliceVar(&o.Config.Webhooks.EtcdComponentProtection.ExemptServiceAccounts, "etcd-components-webhook-exempt-service-accounts", []string{}, "The comma-separated list of fully qualified names of service accounts that are exempt from Etcd-Components-Webhook checks.")
}
