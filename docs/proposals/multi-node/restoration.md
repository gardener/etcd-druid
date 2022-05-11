# Restoration of whole cluster from the backup bucket
A cluster may need to be restarted from backup bucket if for some reason all the persistent volumes are deleted or corrupted. This situation may occur more prominently while the cluster is hibernating during maintenance and the persistent volumes are deleted.

A backup bucket contains multiple full snapshots and delta snapshots.

Full snapshot is the snapshot of database memory at a particular time. Restoration of ETCD cluster from full snapshot means the ETCD cluster will be bootstrapped with that database memory.

Delta snapshot is the ETCD events listed together in a file. Restoring from delta snapshot means applying the ETCD events one by one onto the ETCD cluster. This takes longer than restoring from full snapshot.

For restoration, ETCD Backup Restore container checks first whether the PV is avialable or not. If PV is available, ETCD BR simply starts the respective ETCD nodes. ETCD nodes connect with each other to form a cluster.

If PV is not available, ETCD  BR container connects to the cloud storage and fetches the latest full snapshot and subsequent delta snapshots.

In the particular scenario that we are considering, all the ETCD BR discovers that the respective PVs are unavialble. So, all of them connect to the backup bucket and fetches latest full snapshot and delta snapshots.

All the ETCD BRs form a cluster among themselves. Cluster details are provided by ETCD Druid through configmap. ETCD BR process the configmap and extract the cluster details. Then ETCD BR uses that cluster details to form a temporary cluster to restore the ETCD data directory.

Restoring from the full snapshot only needs the cluster details but no actual ETCD needs to be run.

If there are subsequent delta snapshot after the latest full snapshot, then the delta events needs to be applied from delta snapahot. Applying delta snapshots needs to run an actual ETCD cluster. So an actual ETCD cluster is bootstrapped with cluster details that are already processed earlier. Client ports in the cluster details are changed so that the external clients can't access the cluster while the cluster is restoring from delta snaphots.

The leader in the temporary ETCD cluster applies the delta events. The restoration from delta is co-ordinated by a lease created by ETCD Druid. The leader updates the lease holder identity with code RESTORED once the restoration of the cluster is done. The leading ETCD is stopped then. Rest of the members in the cluster check whether the holder identity of the lease is updated with the code RESTORED or not and if it's updated with the code RESTORED then the respective ETCD is stopped as well.

Main ETCDs are started using restored ETCD Directory in every ETCD BR pod.

## Flow Chart

1. ETCD Druid generates the configmap. This configmap is fed to the ETCD BR pod as ETCD config. Some of the fields in the configmap are supplied as placeholder.
2. ETCD Druid deploys restoration indicator lease.
3. ETCD Druid deploys configmap and statefulset with necessary numbers of replicas for ETCD cluster.
4. ETCD pods are started. ETCD BR checks if there is need of restoration of the cluster.
5. If there is need of restoration from backup bucket, ETCD configs are processed on each ETCD node and proper cluster details are provided in the ETCD config.
6. If no delta snapshot is present in the backup bucket, ETCD data directory is prepared from the latest full snapshots in each ETCD pod using cluster details from the ETCD config.
7. If there are delta snapshots, temporary ETCD is started in each ETCD BR pod and a temporary ETCD cluster is formed using the cluster details from the ETCD config. Temporary ETCD cluster is run on different client port than the client port which is used while running the main ETCD cluster.
8. The leader among the temporary ETCD cluster applies the delta events from the delta snapshot. When the changes in the ETCD cluster are verified, the leader updates the restoration indicator lease with the code RESTORED and shuts down the ETCD process in that particular pod.
9. Meanwhile, the followers in the cluster only checks the restoration indicator lease and if the lease is updated with the code RESTORED then the ETCD process is shut down.
10. ETCD process in ETCD container is started with restored ETCD data directory.