# Migrating an Existing etcd Cluster to an etcd-druid Managed etcd Cluster

This guide provides a comprehensive procedure to migrate an existing (unmanaged) etcd cluster into an etcd-druid managed etcd cluster.

---

## Prerequisites

- **Deploy etcd-druid:** Follow the installation steps in the [etcd-druid documentation](https://gardener.github.io/etcd-druid/deployment/getting-started-locally/getting-started-locally.html) to deploy the etcd-druid operator in your environment.

## Overview

`etcd-druid` supports the integration of new etcd members into an existing `etcd` cluster through the `etcd.spec.etcd.existingCluster` configuration. This capability ensures that when new managed members are added, they automatically join the pre-existing cluster, facilitating seamless migrations.

> **Note:**  
> - The `existingCluster` configuration is evaluated during the bootstrapping of new cluster members.  
> - It is crucial to utilize identical TLS certificates for the new etcd Custom Resource (CR) when joining an existing cluster to ensure secure communication.

## Key Considerations

### Single-Node `etcd` Cluster Migration

- **From Single-Node to Three-Node:** Transitioning from a single-node to a three-node etcd cluster can be achieved with zero downtime. However, once expanded to a multi-member configuration, reducing back to a single-node setup is not supported by `etcd-druid`.
- **From Single-Node to Single-Node:** Migrations of this nature typically involve some downtime. You are required to capture a snapshot using the [etcd-backup-restore tool](https://github.com/gardener/etcd-backup-restore/blob/master/docs/deployment/getting_started.md#taking-scheduled-snapshot), upload it to your designated backup bucket, and configure your new etcd CR to restore from this snapshot.

### Multi-Node `etcd` Cluster Migration

- **Expanding to a Six-Node Cluster:** Initially, a three-node cluster will temporarily function as a six-node cluster by adding three new managed nodes. After these new members synchronize, the original unmanaged nodes can be decommissioned, enabling a migration without any service interruption.

## Migration Steps

### Step 1: Gather Existing Cluster Information

Collect crucial data about your current cluster configuration:

- **Member URLs:** These are the URLs used for peer communications among cluster members.
- **Endpoint:** This is the service endpoint utilized for etcd client interactions.

**Example Configuration:**

```yaml
memberURLs:
  etcd-0:
    - https://etcd-0:2380
  etcd-1:
    - https://etcd-1:2380
  etcd-2:
    - https://etcd-2:2380
endpoint: https://etcd:2379
```

### Step 2: Configure the etcd-druid Managed Cluster

Draft a new etcd Custom Resource (CR) that includes the `existingCluster` configuration to facilitate the migration:

```yaml
apiVersion: druid.gardener.cloud/v1alpha1
kind: Etcd
metadata:
  name: etcd-test
spec:
  selector:
    matchLabels:
      app: etcd-statefulset
  labels:
    app: etcd-statefulset
  etcd:
    metrics: basic
    existingCluster:
      memberURLs:
        etcd-0:
          - http://etcd-0.etcd:2380
        etcd-1:
          - http://etcd-1.etcd:2380
        etcd-2: 
          - http://etcd-2.etcd:2380
      endpoint: http://etcd:2379
  backup:
    deltaSnapshotPeriod: 10s
    store:
      container: etcd-bucket
      prefix: etcd-test
      provider: S3
      secretRef:
        name: etcd-backup-aws
    compression:
      enabled: false
      policy: "gzip"
  replicas: 3
```

> [!NOTE]  
> The `etcd.yaml` file provided above is for demonstration purposes. For a more detailed and comprehensive configuration, refer to the [etcd-druid sample configuration](https://github.com/gardener/etcd-druid/blob/master/config/samples/druid_v1alpha1_etcd.yaml).


---

### Step 3: Apply the Configuration

Apply the new configuration to your Kubernetes cluster using `kubectl`:

```bash
kubectl apply -f etcd.yaml
```

It is essential to ensure that the new cluster is healthy and all members are synchronized:

```bash
 kubectl get etcd etcd-test -o wide 
NAME        READY   QUORATE   ALL MEMBERS READY   BACKUP READY   AGE   CLUSTER SIZE   CURRENT REPLICAS   READY REPLICAS
etcd-test   true    True      True                True           14h   3              3                  3
```

> **Note:**  
> Port-fwordard localy both the svc,
> `kubectl port-forward svc/etcd 2479:2379`and  `kubectl port-forward svc/etcd-test-client 2379:2379`
> export ETCDCTL_ENDPOINTS=http://localhost:2379,http://localhost:2479


Following deployment, the managed members will join the existing cluster, as confirmed by their updated statuses in the etcd cluster.

```bash
root@etcdctl:/# etcdctl endpoint status --cluster -w table
+----------------------------------------------------+------------------+---------+---------+-----------+------------+-----------+------------+--------------------+--------+
|                      ENDPOINT                      |        ID        | VERSION | DB SIZE | IS LEADER | IS LEARNER | RAFT TERM | RAFT INDEX | RAFT APPLIED INDEX | ERRORS |
+----------------------------------------------------+------------------+---------+---------+-----------+------------+-----------+------------+--------------------+--------+
|                            http://etcd-2.etcd:2379 | 4f98c3545405a0b0 |  3.4.34 |   25 kB |      true |      false |        24 |         46 |                 46 |        |
| http://etcd-test-1.etcd-test-peer.default.svc:2379 | 718c10bb9dd7d641 |  3.4.34 |   33 kB |     false |      false |        24 |         46 |                 46 |        |
|                            http://etcd-0.etcd:2379 | a394e0ee91773643 |  3.4.34 |   33 kB |     false |      false |        24 |         46 |                 46 |        |
| http://etcd-test-0.etcd-test-peer.default.svc:2379 | a516ad3125ca0222 |  3.4.34 |   25 kB |     false |      false |        24 |         46 |                 46 |        |
| http://etcd-test-2.etcd-test-peer.default.svc:2379 | bfd1720eb2d79e93 |  3.4.34 |   29 kB |     false |      false |        24 |         46 |                 46 |        |
|                            http://etcd-1.etcd:2379 | d10297b8d2f01265 |  3.4.34 |   33 kB |     false |      false |        24 |         46 |                 46 |        |
+----------------------------------------------------+------------------+---------+---------+-----------+------------+-----------+------------+--------------------+--------+
```

---

### Step 4: Transition Leadership

If the current leader is an unmanaged member (e.g., `4f98c3545405a0b0`), transfer leadership to one of the newly added etcd-druid managed members (e.g., `a394e0ee91773643`):

```bash
root@etcdctl:/# etcdctl move-leader a394e0ee91773643
Leadership transferred from 4f98c3545405a0b0 to a394e0ee91773643
```

### Step 5: Decommission Old Members

> [!NOTE] 
> Ensure that all applications and services that interact with etcd are updated to use the new client endpoint to avoid disruptions.
Once the new cluster is stable and fully operational, remove the old, unmanaged members from the cluster:

```bash
etcdctl member remove <old-member-id>
```

### Conclusion

By following these steps, you can seamlessly migrate an existing etcd cluster to an `etcd-druid` managed setup while minimizing downtime. This approach enhances lifecycle management, backups, and scaling capabilities for your etcd services, ensuring better reliability and maintainability within your Kubernetes environment.