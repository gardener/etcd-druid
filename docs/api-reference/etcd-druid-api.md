# API Reference

## Packages
- [config.druid.gardener.cloud/v1alpha1](#configdruidgardenercloudv1alpha1)
- [druid.gardener.cloud/v1alpha1](#druidgardenercloudv1alpha1)


## config.druid.gardener.cloud/v1alpha1




#### ClientConnectionConfiguration



ClientConnectionConfiguration defines the configuration for constructing a client.Client to connect to k8s kube-apiserver.



_Appears in:_
- [OperatorConfiguration](#operatorconfiguration)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `qps` _float_ | QPS controls the number of queries per second allowed for a connection.<br />Setting this to a negative value will disable client-side rate limiting. |  |  |
| `burst` _integer_ | Burst allows extra queries to accumulate when a client is exceeding its rate. |  |  |
| `contentType` _string_ | ContentType is the content type used when sending data to the server from this client. |  |  |
| `acceptContentTypes` _string_ | AcceptContentTypes defines the Accept header sent by clients when connecting to the server,<br />overriding the default value of 'application/json'. This field will control all connections<br />to the server used by a particular client. |  |  |


#### CompactionControllerConfiguration



CompactionControllerConfiguration defines the configuration for the compaction controller.



_Appears in:_
- [ControllerConfiguration](#controllerconfiguration)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enabled` _boolean_ | Enabled specifies whether backup compaction should be enabled. |  |  |
| `concurrentSyncs` _integer_ | ConcurrentSyncs is the max number of concurrent workers that can be run, each worker servicing a reconcile request. |  |  |
| `eventsThreshold` _integer_ | EventsThreshold denotes total number of etcd events to be reached upon which a backup compaction job is triggered. |  |  |
| `triggerFullSnapshotThreshold` _integer_ | TriggerFullSnapshotThreshold denotes the upper threshold for the number of etcd events before giving up on compaction job and triggering a full snapshot. |  |  |
| `activeDeadlineDuration` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.29/#duration-v1-meta)_ | ActiveDeadlineDuration is the duration after which a running compaction job will be killed. |  |  |
| `metricsScrapeWaitDuration` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.29/#duration-v1-meta)_ | MetricsScrapeWaitDuration is the duration to wait for after compaction job is completed, to allow Prometheus metrics to be scraped |  |  |


#### ControllerConfiguration



ControllerConfiguration defines the configuration for the controllers.



_Appears in:_
- [OperatorConfiguration](#operatorconfiguration)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `disableLeaseCache` _boolean_ | DisableLeaseCache disables the cache for lease.coordination.k8s.io resources.<br />Deprecated: This field will be eventually removed. It is recommended to not use this.<br />It has only been introduced to allow for backward compatibility with the old CLI flags. |  |  |
| `etcd` _[EtcdControllerConfiguration](#etcdcontrollerconfiguration)_ | Etcd is the configuration for the Etcd controller. |  |  |
| `compaction` _[CompactionControllerConfiguration](#compactioncontrollerconfiguration)_ | Compaction is the configuration for the compaction controller. |  |  |
| `etcdCopyBackupsTask` _[EtcdCopyBackupsTaskControllerConfiguration](#etcdcopybackupstaskcontrollerconfiguration)_ | EtcdCopyBackupsTask is the configuration for the EtcdCopyBackupsTask controller. |  |  |
| `secret` _[SecretControllerConfiguration](#secretcontrollerconfiguration)_ | Secret is the configuration for the Secret controller. |  |  |


#### EtcdComponentProtectionWebhookConfiguration



EtcdComponentProtectionWebhookConfiguration defines the configuration for EtcdComponentProtection webhook.
NOTE: At least one of ReconcilerServiceAccountFQDN or ServiceAccountInfo must be set. It is recommended to switch to ServiceAccountInfo.



_Appears in:_
- [WebhookConfiguration](#webhookconfiguration)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enabled` _boolean_ | Enabled indicates whether the EtcdComponentProtection webhook is enabled. |  |  |
| `reconcilerServiceAccountFQDN` _string_ | ReconcilerServiceAccountFQDN is the FQDN of the reconciler service account used by the etcd-druid operator.<br />Deprecated: Please use ServiceAccountInfo instead and ensure that both Name and Namespace are set via projected volumes and downward API in the etcd-druid deployment spec. |  |  |
| `serviceAccountInfo` _[ServiceAccountInfo](#serviceaccountinfo)_ | ServiceAccountInfo contains paths to gather etcd-druid service account information. |  |  |
| `exemptServiceAccounts` _string array_ | ExemptServiceAccounts is a list of service accounts that are exempt from Etcd Components Webhook checks. |  |  |


#### EtcdControllerConfiguration



EtcdControllerConfiguration defines the configuration for the Etcd controller.



_Appears in:_
- [ControllerConfiguration](#controllerconfiguration)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `concurrentSyncs` _integer_ | ConcurrentSyncs is the max number of concurrent workers that can be run, each worker servicing a reconcile request. |  |  |
| `enableEtcdSpecAutoReconcile` _boolean_ | EnableEtcdSpecAutoReconcile controls how the Etcd Spec is reconciled. If set to true, then any change in Etcd spec<br />will automatically trigger a reconciliation of the Etcd resource. If set to false, then an operator needs to<br />explicitly set gardener.cloud/operation=reconcile annotation on the Etcd resource to trigger reconciliation<br />of the Etcd spec.<br />NOTE: Decision to enable it should be carefully taken as spec updates could potentially result in rolling update<br />of the StatefulSet which will cause a minor downtime for a single node etcd cluster and can potentially cause a<br />downtime for a multi-node etcd cluster. |  |  |
| `disableEtcdServiceAccountAutomount` _boolean_ | DisableEtcdServiceAccountAutomount controls the auto-mounting of service account token for etcd StatefulSets. |  |  |
| `etcdStatusSyncPeriod` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.29/#duration-v1-meta)_ | EtcdStatusSyncPeriod is the duration after which an event will be re-queued ensuring etcd status synchronization. |  |  |
| `etcdMember` _[EtcdMemberConfiguration](#etcdmemberconfiguration)_ | EtcdMember holds configuration related to etcd members. |  |  |


#### EtcdCopyBackupsTaskControllerConfiguration



EtcdCopyBackupsTaskControllerConfiguration defines the configuration for the EtcdCopyBackupsTask controller.



_Appears in:_
- [ControllerConfiguration](#controllerconfiguration)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enabled` _boolean_ | Enabled specifies whether EtcdCopyBackupsTaskController should be enabled. |  |  |
| `concurrentSyncs` _integer_ | ConcurrentSyncs is the max number of concurrent workers that can be run, each worker servicing a reconcile request. |  |  |


#### EtcdMemberConfiguration



EtcdMemberConfiguration holds configuration related to etcd members.



_Appears in:_
- [EtcdControllerConfiguration](#etcdcontrollerconfiguration)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `notReadyThreshold` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.29/#duration-v1-meta)_ | NotReadyThreshold is the duration after which an etcd member's state is considered `NotReady`. |  |  |
| `unknownThreshold` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.29/#duration-v1-meta)_ | UnknownThreshold is the duration after which an etcd member's state is considered `Unknown`. |  |  |


#### LeaderElectionConfiguration



LeaderElectionConfiguration defines the configuration for the leader election.
It should be enabled when you deploy etcd-druid in HA mode. For single replica etcd-druid deployments
it will not really serve any purpose.



_Appears in:_
- [OperatorConfiguration](#operatorconfiguration)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enabled` _boolean_ | Enabled specifies whether leader election is enabled. Set this<br />to true when running replicated instances of the operator for high availability. |  |  |
| `leaseDuration` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.29/#duration-v1-meta)_ | LeaseDuration is the duration that non-leader candidates will wait<br />after observing a leadership renewal until attempting to acquire<br />leadership of the occupied but un-renewed leader slot. This is effectively the<br />maximum duration that a leader can be stopped before it is replaced<br />by another candidate. This is only applicable if leader election is<br />enabled. |  |  |
| `renewDeadline` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.29/#duration-v1-meta)_ | RenewDeadline is the interval between attempts by the acting leader to<br />renew its leadership before it stops leading. This must be less than or<br />equal to the lease duration.<br />This is only applicable if leader election is enabled. |  |  |
| `retryPeriod` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.29/#duration-v1-meta)_ | RetryPeriod is the duration leader elector clients should wait<br />between attempting acquisition and renewal of leadership.<br />This is only applicable if leader election is enabled. |  |  |
| `resourceLock` _string_ | ResourceLock determines which resource lock to use for leader election.<br />This is only applicable if leader election is enabled. |  |  |
| `resourceName` _string_ | ResourceName determines the name of the resource that leader election<br />will use for holding the leader lock.<br />This is only applicable if leader election is enabled. |  |  |


#### LogConfiguration



LogConfiguration contains the configuration for logging.



_Appears in:_
- [OperatorConfiguration](#operatorconfiguration)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `logLevel` _[LogLevel](#loglevel)_ | LogLevel is the level/severity for the logs. Must be one of [info,debug,error]. |  |  |
| `logFormat` _[LogFormat](#logformat)_ | LogFormat is the output format for the logs. Must be one of [text,json]. |  |  |


#### LogFormat

_Underlying type:_ _string_

LogFormat is the format of the log.



_Appears in:_
- [LogConfiguration](#logconfiguration)

| Field | Description |
| --- | --- |
| `json` | LogFormatJSON is the JSON log format.<br /> |
| `text` | LogFormatText is the text log format.<br /> |


#### LogLevel

_Underlying type:_ _string_

LogLevel represents the level for logging.



_Appears in:_
- [LogConfiguration](#logconfiguration)

| Field | Description |
| --- | --- |
| `debug` | LogLevelDebug is the debug log level, i.e. the most verbose.<br /> |
| `info` | LogLevelInfo is the default log level.<br /> |
| `error` | LogLevelError is a log level where only errors are logged.<br /> |




#### SecretControllerConfiguration



SecretControllerConfiguration defines the configuration for the Secret controller.



_Appears in:_
- [ControllerConfiguration](#controllerconfiguration)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `concurrentSyncs` _integer_ | ConcurrentSyncs is the max number of concurrent workers that can be run, each worker servicing a reconcile request. |  |  |


#### Server



Server contains information for HTTP(S) server configuration.



_Appears in:_
- [ServerConfiguration](#serverconfiguration)
- [TLSServer](#tlsserver)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `bindAddress` _string_ | BindAddress is the IP address on which to listen for the specified port. |  |  |
| `port` _integer_ | Port is the port on which to serve unsecured, unauthenticated access. |  |  |


#### ServerConfiguration



ServerConfiguration contains the server configurations.



_Appears in:_
- [OperatorConfiguration](#operatorconfiguration)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `webhooks` _[TLSServer](#tlsserver)_ | Webhooks is the configuration for the TLS webhook server. |  |  |
| `metrics` _[Server](#server)_ | Metrics is the configuration for serving the metrics endpoint. |  |  |


#### ServiceAccountInfo



ServiceAccountInfo contains paths to gather etcd-druid service account information.
Usually downward API and projected volumes are used in the deployment specification of etcd-druid to provide this information as mounted volume files.



_Appears in:_
- [EtcdComponentProtectionWebhookConfiguration](#etcdcomponentprotectionwebhookconfiguration)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is the name of the service account associated with etcd-druid deployment. |  |  |
| `namespace` _string_ | Namespace is the namespace in which the service account has been deployed.<br />Usually this information is usually available at /var/run/secrets/kubernetes.io/serviceaccount/namespace.<br />However, if automountServiceAccountToken is set to false then this file will not be available. |  |  |


#### TLSServer



TLSServer is the configuration for a TLS enabled server.



_Appears in:_
- [ServerConfiguration](#serverconfiguration)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `bindAddress` _string_ | BindAddress is the IP address on which to listen for the specified port. |  |  |
| `port` _integer_ | Port is the port on which to serve unsecured, unauthenticated access. |  |  |
| `serverCertDir` _string_ | ServerCertDir is the path to a directory containing the server's TLS certificate and key (the files must be<br />named tls.crt and tls.key respectively). |  |  |


#### WebhookConfiguration



WebhookConfiguration defines the configuration for admission webhooks.



_Appears in:_
- [OperatorConfiguration](#operatorconfiguration)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `etcdComponentProtection` _[EtcdComponentProtectionWebhookConfiguration](#etcdcomponentprotectionwebhookconfiguration)_ | EtcdComponentProtection is the configuration for EtcdComponentProtection webhook. |  |  |



## druid.gardener.cloud/v1alpha1

Package v1alpha1 contains API Schema definitions for the druid v1alpha1 API group

### Resource Types
- [Etcd](#etcd)
- [EtcdCopyBackupsTask](#etcdcopybackupstask)



#### BackupSpec



BackupSpec defines parameters associated with the full and delta snapshots of etcd.



_Appears in:_
- [EtcdSpec](#etcdspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `port` _integer_ | Port define the port on which etcd-backup-restore server will be exposed. |  |  |
| `tls` _[TLSConfig](#tlsconfig)_ |  |  |  |
| `image` _string_ | Image defines the etcd container image and tag |  |  |
| `store` _[StoreSpec](#storespec)_ | Store defines the specification of object store provider for storing backups. |  |  |
| `resources` _[ResourceRequirements](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.29/#resourcerequirements-v1-core)_ | Resources defines compute Resources required by backup-restore container.<br />More info: https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/ |  |  |
| `snapshotCompaction` _[SnapshotCompactionSpec](#snapshotcompactionspec)_ | SnapshotCompaction defines the specification for compaction of backups. |  |  |
| `fullSnapshotSchedule` _string_ | FullSnapshotSchedule defines the cron standard schedule for full snapshots. |  | Pattern: `^(\*\|[1-5]?[0-9]\|[1-5]?[0-9]-[1-5]?[0-9]\|(?:[1-9]\|[1-4][0-9]\|5[0-9])\/(?:[1-9]\|[1-4][0-9]\|5[0-9]\|60)\|\*\/(?:[1-9]\|[1-4][0-9]\|5[0-9]\|60))\s+(\*\|[0-9]\|1[0-9]\|2[0-3]\|[0-9]-(?:[0-9]\|1[0-9]\|2[0-3])\|1[0-9]-(?:1[0-9]\|2[0-3])\|2[0-3]-2[0-3]\|(?:[1-9]\|1[0-9]\|2[0-3])\/(?:[1-9]\|1[0-9]\|2[0-4])\|\*\/(?:[1-9]\|1[0-9]\|2[0-4]))\s+(\*\|[1-9]\|[12][0-9]\|3[01]\|[1-9]-(?:[1-9]\|[12][0-9]\|3[01])\|[12][0-9]-(?:[12][0-9]\|3[01])\|3[01]-3[01]\|(?:[1-9]\|[12][0-9]\|30)\/(?:[1-9]\|[12][0-9]\|3[01])\|\*\/(?:[1-9]\|[12][0-9]\|3[01]))\s+(\*\|[1-9]\|1[0-2]\|[1-9]-(?:[1-9]\|1[0-2])\|1[0-2]-1[0-2]\|(?:[1-9]\|1[0-2])\/(?:[1-9]\|1[0-2])\|\*\/(?:[1-9]\|1[0-2]))\s+(\*\|[1-7]\|[1-6]-[1-7]\|[1-6]\/[1-7]\|\*\/[1-7])$` <br /> |
| `garbageCollectionPolicy` _[GarbageCollectionPolicy](#garbagecollectionpolicy)_ | GarbageCollectionPolicy defines the policy for garbage collecting old backups |  | Enum: [Exponential LimitBased] <br /> |
| `maxBackupsLimitBasedGC` _integer_ | MaxBackupsLimitBasedGC defines the maximum number of Full snapshots to retain in Limit Based GarbageCollectionPolicy<br />All full snapshots beyond this limit will be garbage collected. |  |  |
| `garbageCollectionPeriod` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.29/#duration-v1-meta)_ | GarbageCollectionPeriod defines the period for garbage collecting old backups |  | Pattern: `^([0-9]+(\.[0-9]+)?(ns\|us\|µs\|ms\|s\|m\|h))+$` <br />Type: string <br /> |
| `deltaSnapshotPeriod` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.29/#duration-v1-meta)_ | DeltaSnapshotPeriod defines the period after which delta snapshots will be taken |  | Pattern: `^([0-9]+(\.[0-9]+)?(ns\|us\|µs\|ms\|s\|m\|h))+$` <br />Type: string <br /> |
| `deltaSnapshotMemoryLimit` _[Quantity](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.29/#quantity-resource-api)_ | DeltaSnapshotMemoryLimit defines the memory limit after which delta snapshots will be taken |  |  |
| `deltaSnapshotRetentionPeriod` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.29/#duration-v1-meta)_ | DeltaSnapshotRetentionPeriod defines the duration for which delta snapshots will be retained, excluding the latest snapshot set.<br />The value should be a string formatted as a duration (e.g., '1s', '2m', '3h', '4d') |  | Pattern: `^([0-9]+(\.[0-9]+)?(ns\|us\|µs\|ms\|s\|m\|h))+$` <br />Type: string <br /> |
| `compression` _[CompressionSpec](#compressionspec)_ | SnapshotCompression defines the specification for compression of Snapshots. |  |  |
| `enableProfiling` _boolean_ | EnableProfiling defines if profiling should be enabled for the etcd-backup-restore-sidecar |  |  |
| `etcdSnapshotTimeout` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.29/#duration-v1-meta)_ | EtcdSnapshotTimeout defines the timeout duration for etcd FullSnapshot operation |  | Pattern: `^([0-9]+(\.[0-9]+)?(ns\|us\|µs\|ms\|s\|m\|h))+$` <br />Type: string <br /> |
| `leaderElection` _[LeaderElectionSpec](#leaderelectionspec)_ | LeaderElection defines parameters related to the LeaderElection configuration. |  |  |


#### ClientService



ClientService defines the parameters of the client service that a user can specify



_Appears in:_
- [EtcdConfig](#etcdconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `annotations` _object (keys:string, values:string)_ | Annotations specify the annotations that should be added to the client service |  |  |
| `labels` _object (keys:string, values:string)_ | Labels specify the labels that should be added to the client service |  |  |
| `trafficDistribution` _string_ | TrafficDistribution defines the traffic distribution preference that should be added to the client service.<br />More info: https://kubernetes.io/docs/reference/networking/virtual-ips/#traffic-distribution |  | Enum: [PreferClose] <br /> |


#### CompactionMode

_Underlying type:_ _string_

CompactionMode defines the auto-compaction-mode: 'periodic' or 'revision'.
'periodic' for duration based retention and 'revision' for revision number based retention.

_Validation:_
- Enum: [periodic revision]

_Appears in:_
- [SharedConfig](#sharedconfig)

| Field | Description |
| --- | --- |
| `periodic` | Periodic is a constant to set auto-compaction-mode 'periodic' for duration based retention.<br /> |
| `revision` | Revision is a constant to set auto-compaction-mode 'revision' for revision number based retention.<br /> |


#### CompressionPolicy

_Underlying type:_ _string_

CompressionPolicy defines the type of policy for compression of snapshots.

_Validation:_
- Enum: [gzip lzw zlib]

_Appears in:_
- [CompressionSpec](#compressionspec)

| Field | Description |
| --- | --- |
| `gzip` | GzipCompression is constant for gzip compression policy.<br /> |
| `lzw` | LzwCompression is constant for lzw compression policy.<br /> |
| `zlib` | ZlibCompression is constant for zlib compression policy.<br /> |


#### CompressionSpec



CompressionSpec defines parameters related to compression of Snapshots(full as well as delta).



_Appears in:_
- [BackupSpec](#backupspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enabled` _boolean_ |  |  |  |
| `policy` _[CompressionPolicy](#compressionpolicy)_ |  |  | Enum: [gzip lzw zlib] <br /> |


#### Condition



Condition holds the information about the state of a resource.



_Appears in:_
- [EtcdCopyBackupsTaskStatus](#etcdcopybackupstaskstatus)
- [EtcdStatus](#etcdstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `type` _[ConditionType](#conditiontype)_ | Type of the Etcd condition. |  |  |
| `status` _[ConditionStatus](#conditionstatus)_ | Status of the condition, one of True, False, Unknown. |  |  |
| `lastTransitionTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.29/#time-v1-meta)_ | Last time the condition transitioned from one status to another. |  |  |
| `lastUpdateTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.29/#time-v1-meta)_ | Last time the condition was updated. |  |  |
| `reason` _string_ | The reason for the condition's last transition. |  |  |
| `message` _string_ | A human-readable message indicating details about the transition. |  |  |


#### ConditionStatus

_Underlying type:_ _string_

ConditionStatus is the status of a condition.



_Appears in:_
- [Condition](#condition)

| Field | Description |
| --- | --- |
| `True` | ConditionTrue means a resource is in the condition.<br /> |
| `False` | ConditionFalse means a resource is not in the condition.<br /> |
| `Unknown` | ConditionUnknown means Gardener can't decide if a resource is in the condition or not.<br /> |
| `Progressing` | ConditionProgressing means the condition was seen true, failed but stayed within a predefined failure threshold.<br />In the future, we could add other intermediate conditions, e.g. ConditionDegraded.<br />Deprecated: Will be removed in the future since druid conditions will be replaced by metav1.Condition<br />which has only three status options: True, False, Unknown.<br /> |
| `ConditionCheckError` | ConditionCheckError is a constant for a reason in condition.<br />Deprecated: Will be removed in the future since druid conditions will be replaced by metav1.Condition<br />which has only three status options: True, False, Unknown.<br /> |


#### ConditionType

_Underlying type:_ _string_

ConditionType is the type of condition.



_Appears in:_
- [Condition](#condition)

| Field | Description |
| --- | --- |
| `Ready` | ConditionTypeReady is a constant for a condition type indicating that the etcd cluster is ready.<br /> |
| `LastSnapshotCompactionSucceeded` | ConditionTypeLastSnapshotCompactionSucceeded is a constant for a condition type indicating the status of last snapshot compaction.<br />If `ConditionTypeLastSnapshotCompactionSucceeded` condition status is `False`, it means the compaction controller is currently retrying the compaction operation.<br />Compaction operation can either be a compaction job or a full snapshot.<br /> |
| `AllMembersReady` | ConditionTypeAllMembersReady is a constant for a condition type indicating that all members of the etcd cluster are ready.<br /> |
| `AllMembersUpdated` | ConditionTypeAllMembersUpdated is a constant for a condition type indicating that all members<br />of the etcd cluster have been updated with the desired spec changes.<br /> |
| `BackupReady` | ConditionTypeBackupReady is a constant for a condition type indicating that the etcd backup is ready.<br /> |
| `DataVolumesReady` | ConditionTypeDataVolumesReady is a constant for a condition type indicating that the etcd data volumes are ready.<br /> |
| `Succeeded` | EtcdCopyBackupsTaskSucceeded is a condition type indicating that a EtcdCopyBackupsTask has succeeded.<br /> |
| `Failed` | EtcdCopyBackupsTaskFailed is a condition type indicating that a EtcdCopyBackupsTask has failed.<br /> |


#### CrossVersionObjectReference



CrossVersionObjectReference contains enough information to let you identify the referred resource.



_Appears in:_
- [EtcdStatus](#etcdstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `kind` _string_ | Kind of the referent |  |  |
| `name` _string_ | Name of the referent |  |  |
| `apiVersion` _string_ | API version of the referent |  |  |


#### ErrorCode

_Underlying type:_ _string_

ErrorCode is a string alias representing an error code that identifies an error.



_Appears in:_
- [LastError](#lasterror)



#### Etcd



Etcd is the Schema for the etcds API





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `druid.gardener.cloud/v1alpha1` | | |
| `kind` _string_ | `Etcd` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.29/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[EtcdSpec](#etcdspec)_ |  |  |  |
| `status` _[EtcdStatus](#etcdstatus)_ |  |  |  |


#### EtcdConfig



EtcdConfig defines the configuration for the etcd cluster to be deployed.



_Appears in:_
- [EtcdSpec](#etcdspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `quota` _[Quantity](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.29/#quantity-resource-api)_ | Quota defines the etcd DB quota. |  |  |
| `snapshotCount` _integer_ | SnapshotCount defines the number of applied Raft entries to hold in-memory before compaction.<br />More info: https://etcd.io/docs/v3.4/op-guide/maintenance/#raft-log-retention |  |  |
| `defragmentationSchedule` _string_ | DefragmentationSchedule defines the cron standard schedule for defragmentation of etcd. |  | Pattern: `^(\*\|[1-5]?[0-9]\|[1-5]?[0-9]-[1-5]?[0-9]\|(?:[1-9]\|[1-4][0-9]\|5[0-9])\/(?:[1-9]\|[1-4][0-9]\|5[0-9]\|60)\|\*\/(?:[1-9]\|[1-4][0-9]\|5[0-9]\|60))\s+(\*\|[0-9]\|1[0-9]\|2[0-3]\|[0-9]-(?:[0-9]\|1[0-9]\|2[0-3])\|1[0-9]-(?:1[0-9]\|2[0-3])\|2[0-3]-2[0-3]\|(?:[1-9]\|1[0-9]\|2[0-3])\/(?:[1-9]\|1[0-9]\|2[0-4])\|\*\/(?:[1-9]\|1[0-9]\|2[0-4]))\s+(\*\|[1-9]\|[12][0-9]\|3[01]\|[1-9]-(?:[1-9]\|[12][0-9]\|3[01])\|[12][0-9]-(?:[12][0-9]\|3[01])\|3[01]-3[01]\|(?:[1-9]\|[12][0-9]\|30)\/(?:[1-9]\|[12][0-9]\|3[01])\|\*\/(?:[1-9]\|[12][0-9]\|3[01]))\s+(\*\|[1-9]\|1[0-2]\|[1-9]-(?:[1-9]\|1[0-2])\|1[0-2]-1[0-2]\|(?:[1-9]\|1[0-2])\/(?:[1-9]\|1[0-2])\|\*\/(?:[1-9]\|1[0-2]))\s+(\*\|[1-7]\|[1-6]-[1-7]\|[1-6]\/[1-7]\|\*\/[1-7])$` <br /> |
| `serverPort` _integer_ |  |  |  |
| `clientPort` _integer_ |  |  |  |
| `wrapperPort` _integer_ |  |  |  |
| `image` _string_ | Image defines the etcd container image and tag |  |  |
| `authSecretRef` _[SecretReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.29/#secretreference-v1-core)_ |  |  |  |
| `metrics` _[MetricsLevel](#metricslevel)_ | Metrics defines the level of detail for exported metrics of etcd, specify 'extensive' to include histogram metrics. |  | Enum: [basic extensive] <br /> |
| `resources` _[ResourceRequirements](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.29/#resourcerequirements-v1-core)_ | Resources defines the compute Resources required by etcd container.<br />More info: https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/ |  |  |
| `clientUrlTls` _[TLSConfig](#tlsconfig)_ | ClientUrlTLS contains the ca, server TLS and client TLS secrets for client communication to ETCD cluster |  |  |
| `peerUrlTls` _[TLSConfig](#tlsconfig)_ | PeerUrlTLS contains the ca and server TLS secrets for peer communication within ETCD cluster<br />Currently, PeerUrlTLS does not require client TLS secrets for gardener implementation of ETCD cluster. |  |  |
| `etcdDefragTimeout` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.29/#duration-v1-meta)_ | EtcdDefragTimeout defines the timeout duration for etcd defrag call |  | Pattern: `^([0-9]+(\.[0-9]+)?(ns\|us\|µs\|ms\|s\|m\|h))+$` <br />Type: string <br /> |
| `heartbeatDuration` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.29/#duration-v1-meta)_ | HeartbeatDuration defines the duration for members to send heartbeats. The default value is 10s. |  | Pattern: `^([0-9]+(\.[0-9]+)?(ns\|us\|µs\|ms\|s\|m\|h))+$` <br />Type: string <br /> |
| `clientService` _[ClientService](#clientservice)_ | ClientService defines the parameters of the client service that a user can specify |  |  |


#### EtcdCopyBackupsTask



EtcdCopyBackupsTask is a task for copying etcd backups from a source to a target store.





| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `druid.gardener.cloud/v1alpha1` | | |
| `kind` _string_ | `EtcdCopyBackupsTask` | | |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.29/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[EtcdCopyBackupsTaskSpec](#etcdcopybackupstaskspec)_ |  |  |  |
| `status` _[EtcdCopyBackupsTaskStatus](#etcdcopybackupstaskstatus)_ |  |  |  |


#### EtcdCopyBackupsTaskSpec



EtcdCopyBackupsTaskSpec defines the parameters for the copy backups task.



_Appears in:_
- [EtcdCopyBackupsTask](#etcdcopybackupstask)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `podLabels` _object (keys:string, values:string)_ | PodLabels is a set of labels that will be added to pod(s) created by the copy backups task. |  |  |
| `sourceStore` _[StoreSpec](#storespec)_ | SourceStore defines the specification of the source object store provider for storing backups. |  |  |
| `targetStore` _[StoreSpec](#storespec)_ | TargetStore defines the specification of the target object store provider for storing backups. |  |  |
| `maxBackupAge` _integer_ | MaxBackupAge is the maximum age in days that a backup must have in order to be copied.<br />By default, all backups will be copied. |  | Minimum: 0 <br /> |
| `maxBackups` _integer_ | MaxBackups is the maximum number of backups that will be copied starting with the most recent ones. |  | Minimum: 0 <br /> |
| `waitForFinalSnapshot` _[WaitForFinalSnapshotSpec](#waitforfinalsnapshotspec)_ | WaitForFinalSnapshot defines the parameters for waiting for a final full snapshot before copying backups. |  |  |


#### EtcdCopyBackupsTaskStatus



EtcdCopyBackupsTaskStatus defines the observed state of the copy backups task.



_Appears in:_
- [EtcdCopyBackupsTask](#etcdcopybackupstask)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `conditions` _[Condition](#condition) array_ | Conditions represents the latest available observations of an object's current state. |  |  |
| `observedGeneration` _integer_ | ObservedGeneration is the most recent generation observed for this resource. |  |  |
| `lastError` _string_ | LastError represents the last occurred error. |  |  |


#### EtcdMemberConditionStatus

_Underlying type:_ _string_

EtcdMemberConditionStatus is the status of an etcd cluster member.



_Appears in:_
- [EtcdMemberStatus](#etcdmemberstatus)

| Field | Description |
| --- | --- |
| `Ready` | EtcdMemberStatusReady indicates that the etcd member is ready.<br /> |
| `NotReady` | EtcdMemberStatusNotReady indicates that the etcd member is not ready.<br /> |
| `Unknown` | EtcdMemberStatusUnknown indicates that the status of the etcd member is unknown.<br /> |


#### EtcdMemberStatus



EtcdMemberStatus holds information about etcd cluster membership.



_Appears in:_
- [EtcdStatus](#etcdstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is the name of the etcd member. It is the name of the backing `Pod`. |  |  |
| `id` _string_ | ID is the ID of the etcd member. |  |  |
| `role` _[EtcdRole](#etcdrole)_ | Role is the role in the etcd cluster, either `Leader` or `Member`. |  |  |
| `status` _[EtcdMemberConditionStatus](#etcdmemberconditionstatus)_ | Status of the condition, one of True, False, Unknown. |  |  |
| `reason` _string_ | The reason for the condition's last transition. |  |  |
| `lastTransitionTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.29/#time-v1-meta)_ | LastTransitionTime is the last time the condition's status changed. |  |  |


#### EtcdRole

_Underlying type:_ _string_

EtcdRole is the role of an etcd cluster member.



_Appears in:_
- [EtcdMemberStatus](#etcdmemberstatus)

| Field | Description |
| --- | --- |
| `Leader` | EtcdRoleLeader describes the etcd role `Leader`.<br /> |
| `Member` | EtcdRoleMember describes the etcd role `Member`.<br /> |


#### EtcdSpec



EtcdSpec defines the desired state of Etcd



_Appears in:_
- [Etcd](#etcd)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `selector` _[LabelSelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.29/#labelselector-v1-meta)_ | selector is a label query over pods that should match the replica count.<br />It must match the pod template's labels.<br />More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#label-selectors<br />Deprecated: this field will be removed in the future. |  |  |
| `labels` _object (keys:string, values:string)_ |  |  |  |
| `annotations` _object (keys:string, values:string)_ |  |  |  |
| `etcd` _[EtcdConfig](#etcdconfig)_ |  |  |  |
| `backup` _[BackupSpec](#backupspec)_ |  |  |  |
| `sharedConfig` _[SharedConfig](#sharedconfig)_ |  |  |  |
| `schedulingConstraints` _[SchedulingConstraints](#schedulingconstraints)_ |  |  |  |
| `replicas` _integer_ |  |  |  |
| `priorityClassName` _string_ | PriorityClassName is the name of a priority class that shall be used for the etcd pods. |  |  |
| `storageClass` _string_ | StorageClass defines the name of the StorageClass required by the claim.<br />More info: https://kubernetes.io/docs/concepts/storage/persistent-volumes#class-1 |  |  |
| `storageCapacity` _[Quantity](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.29/#quantity-resource-api)_ | StorageCapacity defines the size of persistent volume. |  |  |
| `volumeClaimTemplate` _string_ | VolumeClaimTemplate defines the volume claim template to be created |  |  |
| `runAsRoot` _boolean_ | RunAsRoot defines whether the securityContext of the pod specification should indicate that the containers shall<br />run as root. By default, they run as non-root with user 'nobody'. |  |  |
| `emptyDirVolumeSource` _[EmptyDirVolumeSource](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.29/#emptydirvolumesource-v1-core)_ | EmptyDirVolumeSource defines the emptyDirVolumeSource that is specified to make use of emtpyDir storage for the etcd data directories.<br />The user does not get the option to configure the name of these volumes.<br />This feature is currently in ALPHA. etcd-druid only supports migrating etcd clusters from using PVCs to emptyDir volumes.<br />The inverse is explicitly NOT supported currently. Once migrated, you CANNOT migrate back to CSI volumes. |  |  |


#### EtcdStatus



EtcdStatus defines the observed state of Etcd.



_Appears in:_
- [Etcd](#etcd)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `observedGeneration` _integer_ | ObservedGeneration is the most recent generation observed for this resource. |  |  |
| `etcd` _[CrossVersionObjectReference](#crossversionobjectreference)_ |  |  |  |
| `conditions` _[Condition](#condition) array_ | Conditions represents the latest available observations of an etcd's current state. |  |  |
| `lastErrors` _[LastError](#lasterror) array_ | LastErrors captures errors that occurred during the last operation. |  |  |
| `lastOperation` _[LastOperation](#lastoperation)_ | LastOperation indicates the last operation performed on this resource. |  |  |
| `currentReplicas` _integer_ | CurrentReplicas is the current replica count for the etcd cluster. |  |  |
| `replicas` _integer_ | Replicas is the replica count of the etcd cluster. |  |  |
| `readyReplicas` _integer_ | ReadyReplicas is the count of replicas being ready in the etcd cluster. |  |  |
| `ready` _boolean_ | Ready is `true` if all etcd replicas are ready. |  |  |
| `labelSelector` _[LabelSelector](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.29/#labelselector-v1-meta)_ | LabelSelector is a label query over pods that should match the replica count.<br />It must match the pod template's labels.<br />Deprecated: this field will be removed in the future. |  |  |
| `members` _[EtcdMemberStatus](#etcdmemberstatus) array_ | Members represents the members of the etcd cluster |  |  |
| `peerUrlTLSEnabled` _boolean_ | PeerUrlTLSEnabled captures the state of peer url TLS being enabled for the etcd member(s) |  |  |
| `selector` _string_ | Selector is a label query over pods that should match the replica count.<br />It must match the pod template's labels. |  |  |


#### GarbageCollectionPolicy

_Underlying type:_ _string_

GarbageCollectionPolicy defines the type of policy for snapshot garbage collection.

_Validation:_
- Enum: [Exponential LimitBased]

_Appears in:_
- [BackupSpec](#backupspec)



#### LastError



LastError stores details of the most recent error encountered for a resource.



_Appears in:_
- [EtcdStatus](#etcdstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `code` _[ErrorCode](#errorcode)_ | Code is an error code that uniquely identifies an error. |  |  |
| `description` _string_ | Description is a human-readable message indicating details of the error. |  |  |
| `observedAt` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.29/#time-v1-meta)_ | ObservedAt is the time the error was observed. |  |  |


#### LastOperation



LastOperation holds the information on the last operation done on the Etcd resource.



_Appears in:_
- [EtcdStatus](#etcdstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `type` _[LastOperationType](#lastoperationtype)_ | Type is the type of last operation. |  |  |
| `state` _[LastOperationState](#lastoperationstate)_ | State is the state of the last operation. |  |  |
| `description` _string_ | Description describes the last operation. |  |  |
| `runID` _string_ | RunID correlates an operation with a reconciliation run.<br />Every time an Etcd resource is reconciled (barring status reconciliation which is periodic), a unique ID is<br />generated which can be used to correlate all actions done as part of a single reconcile run. Capturing this<br />as part of LastOperation aids in establishing this correlation. This further helps in also easily filtering<br />reconcile logs as all structured logs in a reconciliation run should have the `runID` referenced. |  |  |
| `lastUpdateTime` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.29/#time-v1-meta)_ | LastUpdateTime is the time at which the operation was last updated. |  |  |


#### LastOperationState

_Underlying type:_ _string_

LastOperationState is a string alias representing the state of the last operation.



_Appears in:_
- [LastOperation](#lastoperation)

| Field | Description |
| --- | --- |
| `Processing` | LastOperationStateProcessing indicates that an operation is in progress.<br /> |
| `Succeeded` | LastOperationStateSucceeded indicates that an operation has completed successfully.<br /> |
| `Error` | LastOperationStateError indicates that an operation is completed with errors and will be retried.<br /> |
| `Requeue` | LastOperationStateRequeue indicates that an operation is not completed and either due to an error or unfulfilled conditions will be retried.<br /> |


#### LastOperationType

_Underlying type:_ _string_

LastOperationType is a string alias representing type of the last operation.



_Appears in:_
- [LastOperation](#lastoperation)

| Field | Description |
| --- | --- |
| `Create` | LastOperationTypeCreate indicates that the last operation was a creation of a new Etcd resource.<br /> |
| `Reconcile` | LastOperationTypeReconcile indicates that the last operation was a reconciliation of the spec of an Etcd resource.<br /> |
| `Delete` | LastOperationTypeDelete indicates that the last operation was a deletion of an existing Etcd resource.<br /> |


#### LeaderElectionSpec



LeaderElectionSpec defines parameters related to the LeaderElection configuration.



_Appears in:_
- [BackupSpec](#backupspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `reelectionPeriod` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.29/#duration-v1-meta)_ | ReelectionPeriod defines the Period after which leadership status of corresponding etcd is checked. |  | Pattern: `^([0-9]+(\.[0-9]+)?(ns\|us\|µs\|ms\|s\|m\|h))+$` <br />Type: string <br /> |
| `etcdConnectionTimeout` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.29/#duration-v1-meta)_ | EtcdConnectionTimeout defines the timeout duration for etcd client connection during leader election. |  | Pattern: `^([0-9]+(\.[0-9]+)?(ns\|us\|µs\|ms\|s\|m\|h))+$` <br />Type: string <br /> |


#### MetricsLevel

_Underlying type:_ _string_

MetricsLevel defines the level 'basic' or 'extensive'.

_Validation:_
- Enum: [basic extensive]

_Appears in:_
- [EtcdConfig](#etcdconfig)

| Field | Description |
| --- | --- |
| `basic` | Basic is a constant for metrics level basic.<br /> |
| `extensive` | Extensive is a constant for metrics level extensive.<br /> |


#### SchedulingConstraints



SchedulingConstraints defines the different scheduling constraints that must be applied to the
pod spec in the etcd statefulset.
Currently supported constraints are Affinity and TopologySpreadConstraints.



_Appears in:_
- [EtcdSpec](#etcdspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `affinity` _[Affinity](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.29/#affinity-v1-core)_ | Affinity defines the various affinity and anti-affinity rules for a pod<br />that are honoured by the kube-scheduler. |  |  |
| `topologySpreadConstraints` _[TopologySpreadConstraint](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.29/#topologyspreadconstraint-v1-core) array_ | TopologySpreadConstraints describes how a group of pods ought to spread across topology domains,<br />that are honoured by the kube-scheduler. |  |  |


#### SecretReference



SecretReference defines a reference to a secret.



_Appears in:_
- [TLSConfig](#tlsconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `dataKey` _string_ | DataKey is the name of the key in the data map containing the credentials. |  |  |


#### SharedConfig



SharedConfig defines parameters shared and used by Etcd as well as backup-restore sidecar.



_Appears in:_
- [EtcdSpec](#etcdspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `autoCompactionMode` _[CompactionMode](#compactionmode)_ | AutoCompactionMode defines the auto-compaction-mode:'periodic' mode or 'revision' mode for etcd and embedded-etcd of backup-restore sidecar. |  | Enum: [periodic revision] <br /> |
| `autoCompactionRetention` _string_ | AutoCompactionRetention defines the auto-compaction-retention length for etcd as well as for embedded-etcd of backup-restore sidecar. |  |  |


#### SnapshotCompactionSpec



SnapshotCompactionSpec defines parameters related to the compaction job configuration.



_Appears in:_
- [BackupSpec](#backupspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `resources` _[ResourceRequirements](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.29/#resourcerequirements-v1-core)_ | Resources defines compute Resources required by compaction job.<br />More info: https://kubernetes.io/docs/concepts/configuration/manage-compute-resources-container/ |  |  |
| `eventsThreshold` _integer_ | EventsThreshold defines the threshold for the number of etcd events before triggering a compaction job |  |  |
| `triggerFullSnapshotThreshold` _integer_ | TriggerFullSnapshotThreshold defines the upper threshold for the number of etcd events before giving up on compaction job and triggering a full snapshot. |  |  |


#### StorageProvider

_Underlying type:_ _string_

StorageProvider defines the type of object store provider for storing backups.



_Appears in:_
- [StoreSpec](#storespec)



#### StoreSpec



StoreSpec defines parameters related to ObjectStore persisting backups



_Appears in:_
- [BackupSpec](#backupspec)
- [EtcdCopyBackupsTaskSpec](#etcdcopybackupstaskspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `container` _string_ | Container is the name of the container the backup is stored at. |  |  |
| `prefix` _string_ | Prefix is the prefix used for the store. |  |  |
| `provider` _[StorageProvider](#storageprovider)_ | Provider is the name of the backup provider. |  |  |
| `secretRef` _[SecretReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.29/#secretreference-v1-core)_ | SecretRef is the reference to the secret which used to connect to the backup store. |  |  |


#### TLSConfig



TLSConfig hold the TLS configuration details.



_Appears in:_
- [BackupSpec](#backupspec)
- [EtcdConfig](#etcdconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `tlsCASecretRef` _[SecretReference](#secretreference)_ |  |  |  |
| `serverTLSSecretRef` _[SecretReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.29/#secretreference-v1-core)_ |  |  |  |
| `clientTLSSecretRef` _[SecretReference](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.29/#secretreference-v1-core)_ |  |  |  |


#### WaitForFinalSnapshotSpec



WaitForFinalSnapshotSpec defines the parameters for waiting for a final full snapshot before copying backups.



_Appears in:_
- [EtcdCopyBackupsTaskSpec](#etcdcopybackupstaskspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enabled` _boolean_ | Enabled specifies whether to wait for a final full snapshot before copying backups. |  |  |
| `timeout` _[Duration](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.29/#duration-v1-meta)_ | Timeout is the timeout for waiting for a final full snapshot. When this timeout expires, the copying of backups<br />will be performed anyway. No timeout or 0 means wait forever. |  | Pattern: `^(0\|([0-9]+(\.[0-9]+)?(ns\|us\|µs\|ms\|s\|m\|h))+)$` <br />Type: string <br /> |


