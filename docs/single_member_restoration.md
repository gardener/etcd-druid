## Restoration of a single member in multi-node etcd-backup-restore cluster.

**Note**:
Please note here we are proposing the solution to only single member restoration within a cluster not the quorum loss scenario(when majority of members within a cluster fail).

### Motivation:
If a single etcd member within a multi-node etcd cluster goes down due to PVC corruption(invalid data-dir) then it needs to be brought back. But it can't be restore from the snapshots present in storage container as you can't restore a part of whole cluster.

### Solution
- Corresponding backup-restore sidecar if detects that its corresponding etcd is down due to [data-dir corruption](https://github.com/gardener/etcd-backup-restore/blob/7d27a47f5793b0949492d225ada5fd8344b6b6a2/pkg/initializer/validator/datavalidator.go#L177) or [Invalid data-dir](https://github.com/gardener/etcd-backup-restore/blob/7d27a47f5793b0949492d225ada5fd8344b6b6a2/pkg/initializer/validator/datavalidator.go#L204)
- Then, if its is a cluster with cluster size>1 then backup-restore will first remove the etcd failed member using the [MemberRemove API](https://github.com/etcd-io/etcd/blob/ae9734ed278b7a1a7dfc82e800471ebbf9fce56f/clientv3/cluster.go#L45-L46) call and clean the data-dir of failed etcd member.
- It won't affect the etcd cluster as quorum is still maintained.
- After successfully removing failed member from the cluster, backup-restore sidecar will try to add a new member in a cluster to get the same cluster size as before.
- Backup-restore firstly add new member as a [Learner](https://etcd.io/docs/v3.3/learning/learner/) using the [MemberAddAsLearner API](https://github.com/etcd-io/etcd/blob/ae9734ed278b7a1a7dfc82e800471ebbf9fce56f/clientv3/cluster.go#L42-L43) call, once learner added to the cluster and its get in sync with leader and becomes up-to-date then promote the learner to a voting member using [MemberPromote API](https://github.com/etcd-io/etcd/blob/ae9734ed278b7a1a7dfc82e800471ebbf9fce56f/clientv3/cluster.go#L51-L52) call.
- So, the failed member first needs to be removed removed from the cluster and then add a new member.
