# Controllers

etcd-druid is an operator to manage etcd clusters, and follows the [`Operator`](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/) pattern for Kubernetes.
It makes use of the [Kubebuilder](https://github.com/kubernetes-sigs/kubebuilder) framework which makes it quite easy to define Custom Resources (CRs) such as `Etcd`s and `EtcdCopyBackupTask`s through [*Custom Resource Definitions*](https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definitions/) (CRDs), and define controllers for these CRDs.
etcd-druid uses Kubebuilder to define the `Etcd` CR and its corresponding controllers.

All controllers that are a part of etcd-druid reside in package `./controllers`, as sub-packages.

etcd-druid currently consists of 5 controllers, each having its own responsibility:

- *etcd* : responsible for the reconciliation of the `Etcd` CR, which allows users to run etcd clusters within the specified Kubernetes cluster.
- *custodian* : responsible for the updation of the status of the `Etcd` CR.
- *compaction* : responsible for [snapshot compaction](/docs/proposals/02-snapshot-compaction.md).
- *etcdcopybackupstask* : responsible for the reconciliation of the `EtcdCopyBackupsTask` CR, which helps perform the job of copying snapshot backups from one object store to another.
- *secret* : responsible in making sure `Secret`s being referenced by `Etcd` resources are not deleted while in use.

## Package Structure

The typical package structure for the controllers that are part of etcd-druid is shown with the *custodian controller*:

``` bash
controllers/custodian
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

The logic relevant to the controller manager like the creation of the controller manager and registering each of the controllers with the manager, is contained in [`controllers/manager.go`](/controllers/manager.go).

## Etcd Controller

The *etcd controller* is responsible for the reconciliation of the `Etcd` resource.
It handles the provisioning and management of the etcd cluster. Different components that are required for the functioning of the cluster like `Leases`, `ConfigMap`s, and the `Statefulset` for the etcd cluster are all deployed and managed by the *etcd controller*.

While building the controller, an event filter is set such that the behavior of the controller depends on the `gardener.cloud/operation: reconcile` *annotation*. This is controlled by the `--ignore-operation-annotation` CLI flag, which, if set to `false`, tells the controller to perform reconciliation only when this annotation is present. If the flag is set to `true`, the controller will trigger reconciliation anytime the `Etcd` spec, and thus `generation`, changes.  

The reason this filter is present is that any disruption in the `Etcd` resource due to reconciliation (due to changes in the `Etcd` spec, for example) while workloads are being run would be disastrous.
Hence, any user who wishes to avoid such disruptions, can choose to set the `--ignore-operation-annotation` CLI flag to `false`. An example of this is Gardener's [gardenlet](https://github.com/gardener/gardener/blob/master/docs/concepts/gardenlet.md), which reconciles the `Etcd` resource only during a shoot cluster's [*maintenance window*](https://github.com/gardener/gardener/blob/master/docs/usage/shoot_maintenance.md).

The controller adds a finalizer to the `Etcd` resource in order to ensure that the `Etcd` instance does not get deleted while the system is still dependent on the existence of the `Etcd` resource.
Only the *etcd controller* can delete a resource once it adds finalizers to it. This ensures that the proper deletion flow steps are followed while deleting the resource. When the *etcd controller* enters the deletion flow, components are deleted in the reverse order that they were deployed in.

The *etcd controller* is essential to the functioning of the etcd cluster and etcd-druid, thus the minimum number of worker threads is 1 (default being 3).

## Custodian Controller

The *custodian controller* acts on the `Etcd` resource.
The primary purpose of the *custodian controller* is to update the status of the `Etcd` resource.

It watches for changes in the status of the `Statefulset`s associated with the `Etcd` resources.
Even though the `Etcd` resource owns the `Statefulset`, it is not necessary that the *etcd controller* reconciles whenever there are changes in the statuses of the objects that the `Etcd` resource owns.

Status fields of the `Etcd` resource which correspond to the `StatefulSet` like `CurrentReplicas`, `ReadyReplicas`, `Replicas` and `Ready` are updated to reflect those of the `StatefulSet` by the controller. Cluster membership (`EtcdMemberStatus`) and `Conditions` are updated as follows:

- Cluster Membership: The controller updates the information about etcd cluster membership like `Role`, `Status`, `Reason`, `LastTransitionTime` and identifying information like the `Name` and `ID`. For the `Status` field, the member is checked for the *Ready* condition, where the member can be in `Ready`, `NotReady` and `Unknown` statuses.

- Condition: The controller updates the `Conditions` field which holds the latest information of the `Etcd`'s state. The condition checks that are performed are `AllMembersCheck`, `ReadyCheck` and `BackupReadyCheck`. The first two of these checks make use of the `EtcdMemberStatus` to update `Conditions` with information about the members, and the last corresponds to the status of the backup.

To reflect changes that occur in the `Statefulset` status in the `Etcd` resource, the *custodian controller* keeps a watch on the `Statefulset`.

The *custodian controller* reconciles periodically, which can be set through the `--custodian-sync-period` CLI flag (default being 30 seconds). It also reconciles whenever there are changes to the `Statefulset` status.

The *custodian controller* is essential to the functioning of etcd-druid, thus the minimum number of worker threads is 1, (default being 3).

## Compaction Controller

The *compaction controller* deploys the snapshot compaction job whenever required.
The controller watches the number of events accumulated as part of delta snapshots in the etcd cluster's backups, and triggers a snapshot compaction when the number of delta events crosses the set threshold, which is configurable through the `--etcd-events-threshold` CLI flag (1M events by default).

The controller watches for changes in *snapshot* `Leases` associated with `Etcd` resources.
It checks the full and delta snapshot `Leases` and calculates the difference in events between the latest delta snapshot and the previous full snapshot, and initiates the compaction job if the event threshold is crossed.

The number of worker threads for the *compaction controller* needs to be greater than or equal to 0 (default 3).
This is unlike other controllers which need at least one worker thread for the proper functioning of etcd-druid as snapshot compaction is not a core functionality for the etcd clusters to be deployed.
The compaction controller should be explicitly enabled by the user, through the `--enable-backup-compaction` CLI flag.

## Etcdcopybackupstask Controller

The *etcdcopybackupstask controller* is responsible for deploying the [`etcdbrctl copy`](https://github.com/gardener/etcd-backup-restore/blob/master/cmd/copy.go) command as a job.
This controller reacts to create/update events arising from EtcdCopyBackupsTask resources, and deploys the `EtcdCopyBackupsTask` job with source and target backup storage providers as arguments, which are derived from source and target bucket secrets referenced by the `EtcdCopyBackupsTask` resource.

The number of worker threads for the *etcdcopybackupstask controller* needs to be greater than or equal to 0 (default being 3).
This is unlike other controllers who need at least one worker thread for the proper functioning of etcd-druid as `EtcdCopyBackupsTask` is not a core functionality for the etcd clusters to be deployed.

## Secret Controller

The *secret controller*'s primary responsibility is to add a finalizer on `Secret`s referenced by the `Etcd` resource.
The *secret controller* is registered for `Secret`s, and the controller keeps a watch on the `Etcd` CR.
This finalizer is added to ensure that `Secret`s which are referenced by the `Etcd` CR aren't deleted while still being used by the `Etcd` resource.

Events arising from the `Etcd` resource are mapped to a list of `Secret`s such as backup and TLS secrets that are referenced by the `Etcd` resource, and are enqueued into the request queue, which the reconciler then acts on.

The number of worker threads for the secret controller must be at least 1 (default being 10) for this core controller, since the referenced TLS and infrastructure access secrets are essential to the proper functioning of the etcd cluster.
