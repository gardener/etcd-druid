# Controllers

etcd-druid is an operator to manage etcd clusters, and follows the [`Operator`](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/) pattern for Kubernetes.
It makes use of the [Kubebuilder](https://github.com/kubernetes-sigs/kubebuilder) framework which makes it quite easy to define Custom Resources (CRs) such as `Etcd`s and `EtcdCopyBackupTask`s through [*Custom Resource Definitions*](https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definitions/) (CRDs), and define controllers for these CRDs.
etcd-druid uses Kubebuilder to define the `Etcd` CR and its corresponding controllers.

All controllers that are a part of etcd-druid reside in package `internal/controller`, as sub-packages.

Etcd-druid currently consists of the following controllers, each having its own responsibility:

- *etcd* : responsible for the reconciliation of the `Etcd` CR spec, which allows users to run etcd clusters within the specified Kubernetes cluster, and also responsible for periodically updating the `Etcd` CR status with the up-to-date state of the managed etcd cluster.
- *compaction* : responsible for [snapshot compaction](../proposals/02-snapshot-compaction.md).
- *etcdcopybackupstask* : responsible for the reconciliation of the `EtcdCopyBackupsTask` CR, which helps perform the job of copying snapshot backups from one object store to another.
- *secret* : responsible in making sure `Secret`s being referenced by `Etcd` resources are not deleted while in use.

## Package Structure

The typical package structure for the controllers that are part of etcd-druid is shown with the *compaction controller*:

``` bash
internal/controller/compaction
├── config.go
├── reconciler.go
└── register.go
```

- `config.go`: contains all the logic for the configuration of the controller, including feature gate activations, CLI flag parsing and validations.
- `register.go`: contains the logic for registering the controller with the etcd-druid controller manager.
- `reconciler.go`: contains the controller reconciliation logic.

Each controller package also contains auxiliary files which are relevant to that specific controller.

## Controller Manager

A *manager* is first created for all controllers that are a part of etcd-druid.
The *controller manager* is responsible for all the controllers that are associated with CRDs.
Once the manager is `Start()`ed, all the controllers that are *registered* with it are started.  

Each controller is built using a controller builder, configured with details such as the type of object being reconciled, owned objects whose owner object is reconciled, event filters (predicates), etc. `Predicates` are filters which allow controllers to filter which type of events the controller should respond to and which ones to ignore.

The logic relevant to the controller manager like the creation of the controller manager and registering each of the controllers with the manager, is contained in [`internal/manager/manager.go`](../../internal/manager/manager.go).

## Etcd Controller

The *etcd controller* is responsible for the reconciliation of the `Etcd` resource spec and status. It handles the provisioning and management of the etcd cluster. Different components that are required for the functioning of the cluster like `Leases`, `ConfigMap`s, and the `Statefulset` for the etcd cluster are all deployed and managed by the *etcd controller*.

Additionally, *etcd controller* also periodically updates the `Etcd` resource status with the latest available information from the etcd cluster, as well as results and errors from the recent-most reconciliation of the `Etcd` resource spec.

The *etcd controller* is essential to the functioning of the etcd cluster and etcd-druid, thus the minimum number of worker threads is 1 (default being 3), controlled by the CLI flag `--etcd-workers`.

### `Etcd` Spec Reconciliation

While building the controller, an event filter is set such that the behavior of the controller, specifically for `Etcd` update operations, depends on the `gardener.cloud/operation: reconcile` *annotation*. This is controlled by the `--enable-etcd-spec-auto-reconcile` CLI flag, which, if set to `false`, tells the controller to perform reconciliation only when this annotation is present. If the flag is set to `true`, the controller will reconcile the etcd cluster anytime the `Etcd` spec, and thus `generation`, changes, and the next queued event for it is triggered.

> **Note:** Creation and deletion of `Etcd` resources are not affected by the above flag or annotation.

The reason this filter is present is that any disruption in the `Etcd` resource due to reconciliation (due to changes in the `Etcd` spec, for example) while workloads are being run would cause unwanted downtimes to the etcd cluster. Hence, any user who wishes to avoid such disruptions, can choose to set the `--enable-etcd-spec-auto-reconcile` CLI flag to `false`. An example of this is Gardener's [gardenlet](https://github.com/gardener/gardener/blob/676d1bd9e95d80b9f4bc9c56807806031da5d1ce/docs/concepts/gardenlet.md), which reconciles the `Etcd` resource only during a shoot cluster's [*maintenance window*](https://github.com/gardener/gardener/blob/676d1bd9e95d80b9f4bc9c56807806031da5d1ce/docs/usage/shoot/shoot_maintenance.md).

The controller adds a finalizer to the `Etcd` resource in order to ensure that it does not get deleted until all dependent resources managed by etcd-druid, aka managed components, are properly cleaned up. Only the *etcd controller* can delete a resource once it adds finalizers to it. This ensures that the proper deletion flow steps are followed while deleting the resource. During deletion flow, managed components are deleted in parallel.

### `Etcd` Status Updates

The `Etcd` resource status is updated periodically by `etcd controller`, the interval for which is determined by the CLI flag `--etcd-status-sync-period`.

Status fields of the `Etcd` resource such as `LastOperation`, `LastErrors` and `ObservedGeneration`, are updated to reflect the result of the recent reconciliation of the `Etcd` resource spec.

- `LastOperation` holds information about the last operation performed on the etcd cluster, indicated by fields `Type`, `State`, `Description` and `LastUpdateTime`. Additionally, a field `RunID` indicates the unique ID assigned to the specific reconciliation run, to allow for better debugging of issues.
- `LastErrors` is a slice of errors encountered by the last reconciliation run. Each error consists of fields `Code` to indicate the custom etcd-druid error code for the error, a human-readable `Description`, and the `ObservedAt` time when the error was seen.
- `ObservedGeneration` indicates the latest `generation` of the `Etcd` resource that etcd-druid has "observed" and consequently reconciled. It helps identify whether a change in the `Etcd` resource spec was acted upon by druid or not.

Status fields of the `Etcd` resource which correspond to the `StatefulSet` like `CurrentReplicas`, `ReadyReplicas` and `Replicas` are updated to reflect those of the `StatefulSet` by the controller.

Status fields related to the etcd cluster itself, such as `Members`, `PeerUrlTLSEnabled` and `Ready` are updated as follows:

- Cluster Membership: The controller updates the information about etcd cluster membership like `Role`, `Status`, `Reason`, `LastTransitionTime` and identifying information like the `Name` and `ID`. For the `Status` field, the member is checked for the *Ready* condition, where the member can be in `Ready`, `NotReady` and `Unknown` statuses.

`Etcd` resource conditions are indicated by status field `Conditions`.  The condition checks that are currently performed are:

- `AllMembersReady`: indicates readiness of all members of the etcd cluster.
- `Ready`: indicates overall readiness of the etcd cluster in serving traffic.
- `BackupReady`: indicates health of the etcd backups, i.e., whether etcd backups are being taken regularly as per schedule. This condition is applicable only when backups are enabled for the etcd cluster.
- `DataVolumesReady`: indicates health of the persistent volumes containing the etcd data.

## Compaction Controller

The *compaction controller* deploys the snapshot compaction job whenever required. To understand the rationale behind this controller, please read [snapshot-compaction.md](../proposals/02-snapshot-compaction.md).
The controller watches the number of events accumulated as part of delta snapshots in the etcd cluster's backups, and triggers a snapshot compaction when the number of delta events crosses the set threshold, which is configurable through the `--etcd-events-threshold` CLI flag (1M events by default).

The controller watches for changes in *snapshot* `Leases` associated with `Etcd` resources.
It checks the full and delta snapshot `Leases` and calculates the difference in events between the latest delta snapshot and the previous full snapshot, and initiates the compaction job if the event threshold is crossed.

The number of worker threads for the *compaction controller* needs to be greater than or equal to 0 (default 3), controlled by the CLI flag `--compaction-workers`.
This is unlike other controllers which need at least one worker thread for the proper functioning of etcd-druid as snapshot compaction is not a core functionality for the etcd clusters to be deployed.
The compaction controller should be explicitly enabled by the user, through the `--enable-backup-compaction` CLI flag.

## EtcdCopyBackupsTask Controller

The *etcdcopybackupstask controller* is responsible for deploying the [`etcdbrctl copy`](https://github.com/gardener/etcd-backup-restore/blob/master/cmd/copy.go) command as a job.
This controller reacts to create/update events arising from EtcdCopyBackupsTask resources, and deploys the `EtcdCopyBackupsTask` job with source and target backup storage providers as arguments, which are derived from source and target bucket secrets referenced by the `EtcdCopyBackupsTask` resource.

The number of worker threads for the *etcdcopybackupstask controller* needs to be greater than or equal to 0 (default being 3), controlled by the CLI flag `--etcd-copy-backups-task-workers`.
This is unlike other controllers who need at least one worker thread for the proper functioning of etcd-druid as `EtcdCopyBackupsTask` is not a core functionality for the etcd clusters to be deployed.

## Secret Controller

The *secret controller*'s primary responsibility is to add a finalizer on `Secret`s referenced by the `Etcd` resource.
The *secret controller* is registered for `Secret`s, and the controller keeps a watch on the `Etcd` CR.
This finalizer is added to ensure that `Secret`s which are referenced by the `Etcd` CR aren't deleted while still being used by the `Etcd` resource.

Events arising from the `Etcd` resource are mapped to a list of `Secret`s such as backup and TLS secrets that are referenced by the `Etcd` resource, and are enqueued into the request queue, which the reconciler then acts on.

The number of worker threads for the secret controller must be at least 1 (default being 10) for this core controller, controlled by the CLI flag `--secret-workers`, since the referenced TLS and infrastructure access secrets are essential to the proper functioning of the etcd cluster.
