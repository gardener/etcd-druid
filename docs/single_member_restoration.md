# Restoration of a single member in multi-node etcd deployed by etcd-druid.

**Note**:
- For a cluster with n members, we are proposing the solution to only single member restoration within a etcd cluster not the quorum loss scenario (when majority of members within a cluster fail).
- In this proposal we are not targetting the recovery of single member which got separated from cluster due to [network partition](https://etcd.io/docs/v3.3/op-guide/failures/#network-partition).

## Motivation
If a single etcd member within a multi-node etcd cluster goes down due to DB corruption/PVC corruption/Invalid data-dir then it needs to be brought back. Unlike in the single-node case, a minority member of a multi-node cluster can't be restored from the snapshots present in storage container as you can't restore from the old snapshots as it contains the metadata information of cluster which leads to **memberID mismatch** that prevents the new member from coming up as new member is getting it's metadata information from db which got restore from old snapshots.

## Solution
- If a corresponding backup-restore sidecar detects that its corresponding etcd is down due to [data-dir corruption](https://github.com/gardener/etcd-backup-restore/blob/7d27a47f5793b0949492d225ada5fd8344b6b6a2/pkg/initializer/validator/datavalidator.go#L177) or [Invalid data-dir](https://github.com/gardener/etcd-backup-restore/blob/7d27a47f5793b0949492d225ada5fd8344b6b6a2/pkg/initializer/validator/datavalidator.go#L204)
- Then backup-restore will first remove the failing etcd member from the cluster using the [MemberRemove API](https://github.com/etcd-io/etcd/blob/ae9734ed278b7a1a7dfc82e800471ebbf9fce56f/clientv3/cluster.go#L45-L46) call and clean the data-dir of failed etcd member.
- It won't affect the etcd cluster as quorum is still maintained.
- After successfully removing failed etcd member from the cluster, backup-restore sidecar will try to add a new etcd member to a cluster to get the same cluster size as before.
- Backup-restore firstly adds new member as a [Learner](https://etcd.io/docs/v3.3/learning/learner/) using the [MemberAddAsLearner API](https://github.com/etcd-io/etcd/blob/ae9734ed278b7a1a7dfc82e800471ebbf9fce56f/clientv3/cluster.go#L42-L43) call, once learner is added to the cluster and it's get in sync with leader and becomes up-to-date then promote the learner(non-voting member) to a voting member using [MemberPromote API](https://github.com/etcd-io/etcd/blob/ae9734ed278b7a1a7dfc82e800471ebbf9fce56f/clientv3/cluster.go#L51-L52) call.
- So, the failed member first needs to be removed from the cluster and then added as a new member.

### Example: 
1. If a `3` member etcd cluster has 1 downed member(due to invalid data-dir), the cluster can still make forward progress because the quorum is `2`.
2. Backup-restore detects this downed member and then removed the downed member from cluster.
3. The number of members in a cluster becomes `2` and the quorum remains at `2`, so it won't affect the etcd cluster.
4. Clean the data-dir and add a member as a learner(non-voting member).
5. As soon as learner gets in sync with leader, promote the learner to a voting member, hence increasing number of members in a cluster back to `3`.