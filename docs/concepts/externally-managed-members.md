# Externally Managed Members

In certain scenarios, such as [self-hosted shoot clusters](https://github.com/gardener/enhancements/tree/main/geps/0028-self-hosted-shoot-clusters) in Gardener, the etcd members are managed by an external actor (such as static pods managed by the kubelet on the control plane nodes). In these cases, etcd-druid cannot manage the lifecycle of the etcd members directly through Kubernetes constructs like StatefulSets. To accommodate this use case, support for externally managed etcd members is introduced in etcd-druid.

A new field called `spec.externallyManagedMemberAddresses` is introduced which will contain the IP addresses of the externally managed members. An example of this in action is as follows:
```yaml
apiVersion: druid.gardener.cloud/v1alpha1
kind: Etcd
metadata:
  name: etcd-main
spec:
  externallyManagedMemberAddresses:
    - 192.168.0.1
    - 192.168.0.2
    - 192.168.0.3
```

When a list of member IPs is provided to etcd-druid via the `spec.externallyManagedMemberAddresses` field of the Etcd resource, etcd-druid will:
* Disable specific pod-related features:
  * Pods (StatefulSet is created with 0 replicas)
  * Peer/Client Services
  * PodDisruptionBudget
* Populate the etcd ConfigMap with peer/client URLs that use the member IPs from `spec.externallyManagedMemberAddresses` instead of DNS names.
* Populate the Lease object used for sharing state with the sidecar with the member IPs from `spec.externallyManagedMemberAddresses` instead of DNS names.
* Create and assume that the member identities will no longer rely on StatefulSet semantics (like etcd-main-0, etcd-main-1, etc.), instead the identity will be based on the member IPs from `spec.externallyManagedMemberAddresses`. (like etcd-main-192.168.0.1, etcd-main-192.168.0.2, etc.)
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
* If the "service-endpoints" field is not supplied, the endpoints used for creating the etcd client will be derived from the member IPs obtained from the config file as fallback (since the client service is not created in this scenario).


## Removal of members

When a member IP is removed from `spec.externallyManagedMemberAddresses`, etcd-druid will remove the corresponding member from the etcd cluster using etcd's member remove API. etcd-druid will:
* Remove one member per reconciliation.
* Remove a member only when the removal does not cause a loss of quorum.
* Transfer leadership to another member first if the member to be removed is the leader.

> [!WARNING]
> The external actor is expected to update `spec.externallyManagedMemberAddresses` before it changes the members:
> * When adding a member, its IP must be added to the list before the member is started. etcd-druid removes every member of the etcd cluster whose name does not match an IP in the list. A member that has been added to the etcd cluster but has not started yet has no name, so etcd-druid removes it too.
> * When an address is removed from `spec.externallyManagedMemberAddresses`, etcd-druid will remove the corresponding member from the etcd cluster, whether that member is running or not. The members whose addresses remain in `spec.externallyManagedMemberAddresses` must keep running, since etcd-druid removes members only while the etcd cluster has quorum.

Using externally managed members allows etcd-druid to support scenarios where etcd members are managed outside of its direct control, while still providing essential functionalities like backup/restore, maintenance, and monitoring. This allows for etcd-druid to be used for managing etcd clusters in use-cases like [self-hosted shoot clusters in Gardener](https://github.com/gardener/enhancements/tree/main/geps/0028-self-hosted-shoot-clusters), [kubeadm](https://kubernetes.io/docs/reference/setup-tools/kubeadm/), [k3s](https://k3s.io/), etc.

Note that the changes introduced previously to disable runtime components for self-hosted shoot clusters (https://github.com/gardener/etcd-druid/pull/1117) will be subsumed by this new approach.
