# Externally Managed Members

In certain scenarios, such as [self-hosted shoot clusters](https://github.com/gardener/enhancements/tree/main/geps/0028-self-hosted-shoot-clusters) in Gardener, the etcd members are managed by an external actor (such as static pods managed by the kubelet on the control plane nodes). In these cases, etcd-druid cannot manage the lifecycle of the etcd members directly through Kubernetes constructs like StatefulSets. To accommodate this use case, support for externally managed etcd members is introduced in etcd-druid.

The field `spec.etcd.externallyManagedMembers.addresses` contains the IP addresses of the externally managed members. An example of this in action is as follows:
```yaml
apiVersion: druid.gardener.cloud/v1alpha1
kind: Etcd
metadata:
  name: etcd-main
spec:
  etcd:
    externallyManagedMembers:
      addresses:
        - 192.168.0.1
        - 192.168.0.2
        - 192.168.0.3
```

The old `spec.externallyManagedMemberAddresses` field is deprecated and remains supported.

When a list of member IPs is provided to etcd-druid via the `spec.etcd.externallyManagedMembers.addresses` field of the Etcd resource, etcd-druid will:
* Disable specific pod-related features:
  * Pods (StatefulSet is created with 0 replicas)
  * Peer/Client Services
  * PodDisruptionBudget
* Populate the etcd ConfigMap with peer/client URLs that use the member IPs from `spec.etcd.externallyManagedMembers.addresses` instead of DNS names, or localhost placeholders when [endpoint discovery](#endpoint-discovery) is enabled.
* Populate the Lease object used for sharing state with the sidecar with the member IPs from `spec.etcd.externallyManagedMembers.addresses` instead of DNS names.
* Create and assume that the member identities will no longer rely on StatefulSet semantics (like etcd-main-0, etcd-main-1, etc.), instead the identity will be based on the member IPs from `spec.etcd.externallyManagedMembers.addresses`. (like etcd-main-192.168.0.1, etcd-main-192.168.0.2, etc.)
* The `Status.Ready` field in the `Etcd` CR will be populated with the value of the `AllMembersReady` condition instead of the current logic that depends on the StatefulSet Ready status when members are being managed externally.
* When the list of member IPs is changed, etcd-druid will update the ConfigMap and Lease objects accordingly (i.e getting rid of old leases), and remove the members whose IPs are no longer in the list from the etcd cluster (see [Removal of members](#removal-of-members)). It will be the external actor's responsibility to ensure that the new members are added correctly.

The field is subject to the following validations:
* The field is a list of valid IPv4 address strings.
* If the field is specified, it's length must equal `spec.replicas`. This also applies during updates to the field.
* The field can be only be specified during the creation of the `Etcd` CR. The transition from druid-managed to externally-managed members and vice-versa are not supported. i.e the field cannot be introduced when druid is already managing a cluster and the field cannot be removed when the members are being managed externally.
* Removal of the field is not allowed once set.
* The list of member IPs cannot contain duplicates.
* The list of member IPs can be updated, but the updated list must still satisfy the above constraints.

In the etcd-backup-restore sidecar,
* When endpoint discovery is disabled and the "service-endpoints" field is not supplied, the endpoints used for creating the etcd client will be derived from the member IPs obtained from the config file as fallback (since the client service is not created in this scenario).


## Endpoint discovery

Externally managed members can use a node-local file to discover etcd endpoints when a stable service endpoint is unavailable. This integrates the `ENDPOINTS` file mechanism introduced in [etcd-backup-restore#1053](https://github.com/gardener/etcd-backup-restore/pull/1053). The file supplies IPs for contacting the etcd cluster; backup-restore then queries the live member list to build the `initial-cluster` configuration served to etcd-wrapper.

### Configuration

Configure endpoint discovery when creating the `Etcd` resource:

```yaml
spec:
  replicas: 3
  etcd:
    externallyManagedMembers:
      addresses:
        - 192.168.0.1
        - 192.168.0.2
        - 192.168.0.3
      endpointsDiscovery:
        endpointsFilePath: /var/lib/etcd/endpoints/ENDPOINTS
        refreshEnabled: true
        refreshInterval: 30s
```

| Field | Behavior |
| --- | --- |
| `endpointsFilePath` | Path to the file on each host node. Setting this field enables endpoint discovery and requires a non-empty `addresses` list. |
| `refreshEnabled` | Enables periodic updates of the file from the live etcd member list. Defaults to `false`. |
| `refreshInterval` | Interval between refresh attempts. If omitted, backup-restore uses its default of `30s`. An explicitly configured interval must be positive when `refreshEnabled` is `true`. |

Endpoint discovery cannot be enabled or disabled after the `Etcd` resource has been created.

### Providing the file

The external actor must create the file on each node before starting its etcd pod. Each non-empty line contains one IP address, without a scheme or port:

```text
192.168.0.1
192.168.0.2
192.168.0.3
```

For a member joining an existing cluster, provide the existing members' IPs so that backup-restore can contact the cluster. For the first member of a new cluster, an empty file is valid: backup-restore uses `POD_IP` as its endpoint during bootstrap. The file must still exist. Backup-restore exits during startup if the file is missing, unreadable, or contains an invalid IP.

Druid mounts the file's parent directory into the backup-restore container at `/var/etcd/endpoints` and sets the `ENDPOINTS` environment variable to the mounted file path. In the example above, the container reads `/var/etcd/endpoints/ENDPOINTS`. Druid also supplies `POD_IP` from the pod's IP.

### Endpoint and etcd configuration discovery

To contact cluster members, backup-restore combines the file's IPs with the client URL scheme and port from the ConfigMap. These endpoints take precedence over configured service endpoints and per-member client URLs in the ConfigMap. The file is read when creating clients that connect to cluster members, so subsequent clients can use updated contents.

With endpoint discovery enabled, Druid generates localhost placeholders for `advertise-client-urls`, `initial-advertise-peer-urls`, and `initial-cluster` in the ConfigMap. The ConfigMap still provides the URL schemes and ports and the remaining etcd settings. When serving `/config` to etcd-wrapper, backup-restore:

* Builds this member's advertised client and peer URLs from `POD_IP` and the schemes and ports in the ConfigMap.
* Builds `initial-cluster` from the live members' names and peer URLs. It includes the current member if that member is not yet listed by name, which also supports initial bootstrap.

### Keeping endpoints current

When `refreshEnabled` is `true`, Druid passes `--enable-endpoints-refresh=true` and the configured `--endpoints-refresh-interval` to backup-restore. Backup-restore periodically queries the live etcd member list, extracts member IPs from their peer URLs, and atomically replaces the file. The parent directory must be writable by the backup-restore process because replacement creates a temporary file in that directory and renames it over the existing file. This is why Druid mounts the directory rather than the individual file.

If a refresh fails, backup-restore logs the failure, retains the previous file, and retries on a later interval. When refresh is disabled, the external actor is responsible for keeping the file current.

The file supplies discovered endpoints; `spec.etcd.externallyManagedMembers.addresses` continues to define the desired members for Druid. Refreshing the file does not update the Etcd resource. The external actor must still update the address list as described in [Removal of members](#removal-of-members).

## Removal of members

When a member IP is removed from `spec.etcd.externallyManagedMembers.addresses`, etcd-druid will remove the corresponding member from the etcd cluster using etcd's member remove API. etcd-druid will:
* Remove one member per reconciliation.
* Remove a member only when the removal does not cause a loss of quorum.
* Transfer leadership to another member first if the member to be removed is the leader.

> [!WARNING]
> The external actor is expected to update `spec.etcd.externallyManagedMembers.addresses` before it changes the members:
> * When adding a member, its IP must be added to the list before the member is started. etcd-druid removes every member of the etcd cluster whose name does not match an IP in the list. A member that has been added to the etcd cluster but has not started yet has no name, so etcd-druid removes it too.
> * When an address is removed from `spec.etcd.externallyManagedMembers.addresses`, etcd-druid will remove the corresponding member from the etcd cluster, whether that member is running or not. The members whose addresses remain in `spec.etcd.externallyManagedMembers.addresses` must keep running, since etcd-druid removes members only while the etcd cluster has quorum.

Using externally managed members allows etcd-druid to support scenarios where etcd members are managed outside of its direct control, while still providing essential functionalities like backup/restore, maintenance, and monitoring. This allows for etcd-druid to be used for managing etcd clusters in use-cases like [self-hosted shoot clusters in Gardener](https://github.com/gardener/enhancements/tree/main/geps/0028-self-hosted-shoot-clusters), [kubeadm](https://kubernetes.io/docs/reference/setup-tools/kubeadm/), [k3s](https://k3s.io/), etc.

Note that the changes introduced previously to disable runtime components for self-hosted shoot clusters (https://github.com/gardener/etcd-druid/pull/1117) will be subsumed by this new approach.
