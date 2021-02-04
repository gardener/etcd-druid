# Backup Compaction for ETCD

## Current Problem
To ensure recoverability of ETCD, backups of the database are taken at regular interval.
Backups are of two types: Full Snapshots and Incremental Snapshots. 

### Full Snapshots
Full snapshot is a snapshot of the complete database at given point in time.The size of the database keeps changing with time and typically the size is relatively large (measured in 100s of megabytes or even in gigabytes. For this reason, full snapshots are taken after some large intervals.

### Incremental Snapshots
Incremental Snapshots are collection of events on ETCD database, obtained through running WATCH API Call on ETCD. After some short intervals, all the events that are accumulated through WATCH API Call are saved in a file and named as Incremental Snapshots at relatively short time intervals.

### Recovery from the Snapshots

#### Recovery from Full Snapshots
As the full snapshots are snapshots of the complete database, the whole database can be recovered from a full snapshot in one go. ETCD provides API Call to restore the database from a full snapshot file.

#### Recovery from Incremental Snapshots
Delta snapshots are collection of retrospective ETCD events. So, to restore from Incremental snapshot file, the events from the file are needed to be applied sequentially on ETCD database through ETCD Put/Delete API calls. As it is heavily dependent on ETCD calls sequentially, restoring from Incremental Snapshot files can take long if there are numerous commands captured in Incremental Snapshot files.

Delta snapshots are applied on top of running ETCD database. So, if there is inconsistency between the state of database at the point of applying and the state of the database when the delta snapshot commands were captured, restoration will fail.

Currently, in Gardener setup, ETCD is restored from the last full snapshot and then the delta snapshots, which were captured after the last full snapshot.

The main problem with this is that the complete restoration time can be unacceptably large if the rate of change coming into the etcd database is quite high because there are large number of events in the delta snapshots to be applied sequentially.
A secondary problem is that, though auto-compaction is enabled for etcd, it is not quick enough to compact all the changes from the incremental snapshots being re-applied during the relatively short period of time of restoration (as compared to the actual period of time when the incremental snapshots were accumulated). This may lead to the etcd pod (the backup-restore sidecar container, to be precise) to run out of memory and/or storage space even if it is sufficient for normal operations.

## Solution
### Compaction command
To help with the problem mentioned earlier, our proposal is to introduce `compact` subcommand with `etcdbrctl`. On execution of `compact` command, A separate embedded ETCD process will be started  where the ETCD data will be restored from the snapstore (exactly as in the restoration scenario today). Then the new ETCD database will be compacted and defragmented using ETCD API calls. The compaction will strip off the ETCD database of old revisions as per the ETCD auto-compaction configuration. The defragmentation will free up the unused fragment memory space released after compaction. Then a full snapshot of the compacted database will be saved in snapstore which then can be used as the base snapshot during any subsequent restoration (or backup compaction).

### How the solution works
The newly introduced compact command does not disturb the running ETCD while compacting the backup snapshots. The command is designed to run potentially separately (from the main ETCD process/container/pod). ETCD Druid can be configured to periodically schedule the newly introduced compact command as a separate job (scheduled periodically) based on a parameter which is defined in ETCD CRD as  `backupCompactionSchedule`.

### Example ETCD CRD: 
```yaml
apiVersion: druid.gardener.cloud/v1alpha1
kind: Etcd
...
spec:
  ...
  backup:
    ...
    backupCompactionSchedule: "0/30 * * * *" # Backup compaction job is executed every 30m on the clock
    ...
  ...
```

### Example Cron Job:
An example can be found [here](https://github.com/gardener/etcd-druid/blob/master/charts/etcd/templates/etcd-compaction-cronjob.yaml)

### **Points to take care while saving the compacted snapshot:**
As compacted snapshot and the existing periodic full snapshots are taken by different processes running in different pods but accessing same store to save the snapshots, some problems may arise:
1. When uploading the compacted snapshot to the snapstore, there is the problem of how does the restorer know when to start using the newly compacted snapshot. This communication needs to be atomic.
2. With a regular schedule for compaction that happens potentially separately from the main etcd pod, is there a need for regular scheduled full snapshots anymore?
3. We are planning to introduce new directory structure, under v2 prefix, for saving the snapshots (compacted and full), as mentioned in details below. But for backward compatibility, we also need to consider the older directory, which is currently under v1 prefix, during accessing snapshots.

#### **How to swap full snapshot with compacted snapshot atomically**

Currently, full snapshots and the subsequent delta snapshots are grouped under same prefix path in the snapstore. When a full snapshot is created, it is placed under a prefix/directory with the name comprising of timestamp. Then subsequent delta snapshots are also pushed into the same directory. Thus each prefix/directory contains a single full snapshot and the subsequent delta snapshots. So far, it is the job of ETCDBR to start main ETCD process and snapshotter process which takes full snapshot and delta snapshot periodically. But as per our proposal, compaction will be running as parallel process to main ETCD process and snapshotter process. So we can't reliably co-ordinate between the processes to achieve switching to the compacted snapshot as the base snapshot atomically.

##### **Current Directory Structure**
```yaml
- Backup-192345
    - Full-Snapshot-0-1-192345
    - Incremental-Snapshot-1-100-192355
    - Incremental-Snapshot-100-200-192365
    - Incremental-Snapshot-200-300-192375
- Backup-192789
    - Full-Snapshot-0-300-192789
    - Incremental-Snapshot-300-400-192799
    - Incremental-Snapshot-400-500-192809
    - Incremental-Snapshot-500-600-192819
```

To solve the problem, proposal is:
1. ETCDBR will take the first full snapshot after it starts main ETCD Process and snapshotter process. After taking the first full snapshot, snapshotter will continue taking full snapshots. On the other hand, ETCDBR compactor command will be run as periodic job in a separate pod and use the existing full or compacted snapshots to produce further compacted snapshots. Full snapshots and compacted snapshots will be named after same fashion. So, there is no need of any mechanism to choose which snapshots(among full and compacted snapshot) to consider as base snapshots. 
2. Flatten the directory structure of backup folder. Save all the full snapshots, delta snapshots and compacted snapshots under same directory/prefix. Restorer will restore from full/compacted snapshots and delta snapshots sorted based on the revision numbers in name (or timestamp if the revision numbers are equal).

##### **Proposed Directory Structure**
```yaml
Backup :
    - Full-Snapshot-0-1-192355 (Taken by snapshotter)
    - Incremental-Snapshot-revision-1-100-192365
    - Incremental-Snapshot-revision-100-200-192375
    - Full-Snapshot-revision-0-200-192379 (Taken by snapshotter)
    - Incremental-Snapshot-revision-200-300-192385
    - Full-Snapshot-revision-0-300-192386 (Taken by compaction job)
    - Incremental-Snapshot-revision-300-400-192396
    - Incremental-Snapshot-revision-400-500-192406
    - Incremental-Snapshot-revision-500-600-192416
    - Full-Snapshot-revision-0-600-192419 (Taken by snapshotter)
    - Full-Snapshot-revision-0-600-192420 (Taken by compaction job)
```

##### What happens to the delta snapshots that were compacted?
The proposed `compaction` sub-command in `etcdbrctl` (and hence,  the `CronJob` provisioned by `etcd-druid` that will schedule it at a regular interval) would only upload the compacted full snapshot.
It will not delete the snapshots (delta or full snapshots) that were compacted.
These snapshots which were superseded by a freshly uploaded compacted snapshot would follow the same life-cycle as other older snapshots.
I.e. they will be garbage collected according to the configured backup snapshot retention policy.
For example, if an `exponential` retention policy is configured and if compaction is done every `30m` then there might be at most `48` additional (compacted) full snapshots (`24h * 2`) in the backup for the latest day. As time rolls forward to the next day, these additional compacted snapshots (along with the delta snapshots that were compacted into them) will get garbage collected retaining only one full snapshot for the day before according to the retention policy.

##### **Future work**
In future, we have plan to stop the snapshotter just after taking the first full snapshot. Then, the compaction job will be solely responsible for taking subsequent full snapshots. The directory structure would be looking like following:

```yaml
Backup :
    - Full-Snapshot-0-1-192355 (Taken by snapshotter)
    - Incremental-Snapshot-revision-1-100-192365
    - Incremental-Snapshot-revision-100-200-192375
    - Incremental-Snapshot-revision-200-300-192385
    - Full-Snapshot-revision-0-300-192386 (Taken by compaction job)
    - Incremental-Snapshot-revision-300-400-192396
    - Incremental-Snapshot-revision-400-500-192406
    - Incremental-Snapshot-revision-500-600-192416
    - Full-Snapshot-revision-0-600-192420 (Taken by compaction job)
```

#### Backward Compatibility
1. **Restoration** : The changes to handle the newly proposed backup directory structure must be backward compatible with older structures at least for restoration because we need have to restore from backups in the older structure. This includes the support for restoring from a backup without a metadata file if that is used in the actual implementation.
2. **Backup** : For new snapshots (even on a backup containing the older structure), the new structure may be used. The new structure must be setup automatically including creating the base full snapshot.
3. **Garbage collection** : The existing functionality of garbage collection of snapshots (full and incremental) according to the backup retention policy must be compatible with both old and new backup folder structure. I.e. the snapshots in the older backup structure must be retained in their own structure and the snapshots in the proposed backup structure should be retained in the proposed structure. Once all the snapshots in the older backup structure go out of the retention policy and are garbage collected, we can think of removing the support for older backup folder structure.

**Note:** Compactor will run parallel to current snapshotter process and work only if there is any full snapshot already present in the store. By current design, a full snapshot will be taken if there is already no full snapshot or the existing full snapshot is older than 24 hours. It is not limitation but a design choice. As per proposed design, the backup storage will contain both periodic full snapshots as well as periodic compacted snapshot. Restorer will pickup the base snapshot whichever is latest one.
