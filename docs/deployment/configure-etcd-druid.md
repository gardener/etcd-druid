# etcd-druid CLI Flags

`etcd-druid` process can be started with the following command line flags.

## Command line flags

!!! note "Deprecation Notice"

​	All existing command line flags have been marked as deprecated. To configure `etcd-druid` the recommended way is to define a single `--config` CLI flag which points to a mounted `ConfigMap` containing `etcd-druid` [OperatorConfiguration](https://github.com/gardener/etcd-druid/blob/opconfig/api/config/v1alpha1/types.go#L14). For backward compatibility both new `--config` CLI flag and now deprecated existing `CLI flags` are supported.

### Operator Configuration

The recommended way to configure `etcd-druid` is via [OperatorConfiguration](https://github.com/gardener/etcd-druid/blob/opconfig/api/config/v1alpha1/types.go#L14).  It is assumed that you have done the following:

* Created a `ConfigMap` which contains the serialized operator configuration.
* Mount the `ConfigMap` to etcd-druid `Deployment`.

| Flag   | Description                                 | Default |
| ------ | ------------------------------------------- | ------- |
| config | Path to the mounted operator configuration. | NA      |

You can see the default values for `OperatorConfiguration` at [api/config/v1alpha1/defaults.go](https://github.com/gardener/etcd-druid/blob/opconfig/api/config/v1alpha1/defaults.go).

!!! note

​	It is required to define the `--config` CLI flag and point to the `ConfigMap` mount path.

### Leader election *(Deprecated)*

If you wish to setup `etcd-druid` in high-availability mode then leader election needs to be enabled to ensure that at a time only one replica services the incoming events and does the reconciliation.

| Flag                          | Description                                                  | Default                 |
| ----------------------------- | ------------------------------------------------------------ | ----------------------- |
| enable-leader-election        | Leader election provides the capability to select one replica as a leader where active reconciliation will happen. The other replicas will keep waiting for leadership change and not do active reconciliations. | false                   |
| leader-election-id            | Name of the k8s lease object that leader election will use for holding the leader lock. By default etcd-druid will use lease resource lock for leader election which is also a [natural usecase](https://kubernetes.io/docs/concepts/architecture/leases/#leader-election) for leases and is also recommended by k8s. | "druid-leader-election" |
| leader-election-resource-lock | ***Deprecated***: This flag will be removed in later version of druid. By default `lease.coordination.k8s.io` resources will be used for leader election resource locking for the controller manager. | "leases"                |

### Metrics *(Deprecated)*

`etcd-druid` exposes a `/metrics` endpoint which can be scrapped by tools like [Prometheus](https://prometheus.io/). If the default metrics endpoint configuration is not suitable then consumers can change it via the following options.

| Flag                 | Description                                                  | Default |
| -------------------- | ------------------------------------------------------------ | ------- |
| metrics-bind-address | The IP address that the metrics endpoint binds to            | ""      |
| metrics-port         | The port used for the metrics endpoint                       | 8080    |
| metrics-addr         | Duration to wait for after compaction job is completed, to allow Prometheus metrics to be scraped.<br />**Deprecated:** Please use `--metrics-bind-address` and `--metrics-port` instead | ":8080" |

Metrics bind-address is computed by joining the host and port. By default its value is computed as `:8080`.

!!! tip
    Ensure that the `metrics-port` is also reflected in the `etcd-druid` deployment specification.

### Webhook Server *(Deprecated)*

etcd-druid provides the following CLI flags to configure webhook server. These CLI flags are used to construct a new [webhook.Server](https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/webhook#Server) by configuring [Options](https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/webhook#Options).

| Flag                               | Description                                                  | Default                 |
| ---------------------------------- | ------------------------------------------------------------ | ----------------------- |
| webhook-server-bind-address        | It is the address that the webhook server will listen on.    | ""                      |
| webhook-server-port                | Port is the port number that the webhook server will serve.  | 9443                    |
| webhook-server-tls-server-cert-dir | The path to a directory containing the server's TLS certificate and key (the files must be named tls.crt and tls.key respectively). | /etc/webhook-server-tls |

### Etcd-Components Webhook *(Deprecated)*

etcd-druid provisions and manages several Kubernetes resources which we call [`Etcd`cluster components](../concepts/etcd-cluster-components.md). To ensure that there is no accidental changes done to these managed resources, a webhook is put in place to check manual changes done to any managed etcd-cluster Kubernetes resource. It rejects most of these changes except a few. The details on how to enable the `etcd-components` webhook, which resources are protected and in which scenarios is the change allowed is documented [here](../concepts/etcd-cluster-resource-protection.md).

Following CLI flags are provided to configure the `etcd-components` webhook:

| Flag                                    | Description                                                  | Default                    |
| --------------------------------------- | ------------------------------------------------------------ | -------------------------- |
| enable-etcd-components-webhook          | Enable EtcdComponents Webhook to prevent unintended changes to resources managed by etcd-druid. | false                      |
| reconciler-service-account              | The fully qualified name of the service account used by etcd-druid for reconciling etcd resources. If unspecified, the default service account mounted for etcd-druid will be used | etcd-druid-service-account |
| etcd-components-exempt-service-accounts | In case there is a need to allow changes to `Etcd` resources from external controllers like `vertical-pod-autoscaler` then one must list the `ServiceAaccount` that is used by each such controller. | ""                         |

### Reconcilers *(Deprecated)*

Following set of flags configures the reconcilers running within etcd-druid. To know more about different reconcilers read [this](../development/controllers.md) document.

#### Etcd Reconciler *(Deprecated)*

| Flag                                  | Description                                                  | Default |
| ------------------------------------- | ------------------------------------------------------------ | ------- |
| etcd-workers                          | Number of workers spawned for concurrent reconciles of `Etcd` resources. | 3       |
| enable-etcd-spec-auto-reconcile       | If true then automatically reconciles Etcd Spec. If false, waits for explicit annotation `gardener.cloud/operation: reconcile` to be placed on the Etcd resource to trigger reconcile. | false   |
| disable-etcd-serviceaccount-automount | For each `Etcd` cluster a `ServiceAccount` is created which is used by the `StatefulSet` pods and tied to `Role` via `RoleBinding`. If `false` then pods running as this `ServiceAccount` will have the API token automatically mounted. You can consider disabling it if you wish to use [Projected Volumes](https://kubernetes.io/docs/concepts/storage/projected-volumes/#serviceaccounttoken) allowing one to set an `expirationSeconds` on the mounted token for better security. To use projected volumes ensure that you have set relevant  [kube-apiserver flags](https://kubernetes.io/docs/tasks/configure-pod-container/configure-service-account/#serviceaccount-token-volume-projection).<br />**Note:** With Kubernetes version >=1.24 projected service account token is the default. This means that we no longer need this flag. [Issue #872](https://github.com/gardener/etcd-druid/issues/872) has been raised to address this. | false   |
| etcd-status-sync-period               | `Etcd.Status` is periodically updated. This interval defines the status sync frequency. | 15s     |
| etcd-member-notready-threshold        | Threshold after which an etcd member is considered not ready if the status was unknown before. This is currently used to update [EtcdMemberConditionStatus](https://github.com/gardener/etcd-druid/blob/55efca1c8f6c852b0a4e97f08488ffec2eed0e68/api/v1alpha1/etcd.go#L360). | 5m      |
| etcd-member-unknown-threshold         | Threshold after which an etcd member is considered unknown. This is currently used to update [EtcdMemberConditionStatus](https://github.com/gardener/etcd-druid/blob/55efca1c8f6c852b0a4e97f08488ffec2eed0e68/api/v1alpha1/etcd.go#L360). | 1m      |

#### Compaction Reconciler *(Deprecated)*

| Flag                         | Description                                                  | Default |
| ---------------------------- | ------------------------------------------------------------ | ------- |
| enable-backup-compaction     | Enable automatic compaction of etcd backups                  | false   |
| compaction-workers           | Number of workers that can be spawned for concurrent reconciles for compaction Jobs. The controller creates a backup compaction job if a certain etcd event threshold is reached. If compaction is enabled, the value for this flag must be greater than zero. | 3       |
| etcd-events-threshold        | Defines the threshold in terms of total number of etcd events before a backup compaction job is triggered. | 1000000 |
| active-deadline-duration     | Duration after which a running backup compaction job will be terminated. | 3h      |
| metrics-scrape-wait-duration | Duration to wait for after compaction job is completed, to allow Prometheus metrics to be scraped. | 0s      |

#### Etcd Copy-Backup Task & Secret Reconcilers *(Deprecated)*

| Flag                           | Description                                                  | Default |
| ------------------------------ | ------------------------------------------------------------ | ------- |
| etcd-copy-backups-task-workers | Number of workers spawned for concurrent reconciles for `EtcdCopyBackupTask` resources. | 3       |
| secret-workers                 | Number of workers spawned for concurrent reconciles for secrets. | 10      |

#### Miscellaneous *(Deprecated)*

| Flag                | Description                                                  | Default |
| ------------------- | ------------------------------------------------------------ | ------- |
| feature-gates       | A set of key=value pairs that describe feature gates for alpha/experimental features. Please check [feature-gates](feature-gates.md) for more information. | ""      |
| disable-lease-cache | Disable cache for lease.coordination.k8s.io resources.       | false   |
