# Bootstrap with an Existing etcd Cluster

`etcd-druid` can create a new `Etcd` resource by joining it to an already-running etcd cluster instead of forming a standalone cluster. The new etcd-druid managed members become part of the existing etcd cluster and replicate its data while the existing members continue serving traffic.

This document describes the **join phase**: how the target `Etcd` resource is configured to join an existing source cluster, what etcd-druid does during the join, and the status it records once the join has succeeded.

## Terminology

| Term | Meaning |
|------|---------|
| **Source etcd cluster** | The already-running etcd cluster that the new target members join. The source can be managed by etcd-druid or by something else. etcd-druid does not modify externally managed source clusters; the operator must provide source peer/client URLs that are advertised by the source and reachable from the target network. |
| **Target `Etcd` resource** | The new etcd-druid managed `Etcd` resource configured with `.spec.etcd.bootstrapWithExistingCluster`. Its pods join the source cluster. |
| **Combined etcd cluster** | The single etcd cluster after the target members have joined. It contains both source and target members. |
| **Join phase** | The phase covered by `bootstrapWithExistingCluster`: target members are added to the existing cluster and synchronized. |


## Network reachability model

The target members can join only through peer URLs that the source cluster already advertises in its etcd member list. `etcd-druid` does not invent or rewrite source peer URLs.

For same-network scenarios, the source's existing in-cluster peer URLs may already be reachable from the target.

For cross-network scenarios, the operator must prepare reachable peer URLs first:

- **etcd-druid managed source:** configure `.spec.etcd.additionalAdvertisePeerURLs` on the source cluster so source members advertise peer URLs reachable from the target network. See [Using Additional Advertise Peer URLs](../usage/using-additional-advertise-peer-urls.md).
- **Externally managed source:** configure the source cluster outside etcd-druid so its member list advertises peer URLs reachable from the target network. The target spec must use those advertised URLs.

Only the peer URLs listed in `.spec.etcd.bootstrapWithExistingCluster.members[*].peerUrls` need to be reachable from the target members. The source's other internal peer URLs may remain private if they are not used in the target spec.

## API shape

### Spec

```yaml
spec:
  etcd:
    bootstrapWithExistingCluster:
      members:
        - name: <source-member-name>
          peerUrls:
            - <source-peer-url-reachable-from-target>
        # one entry per source member to include in the initial cluster view
      clientEndpoints:
        - <source-client-endpoint-reachable-from-target>
```

The values must match the source cluster's member/client endpoints as seen by the target network.

### Status

After the join succeeds, `etcd-druid` writes a snapshot of the source-cluster members that were part of the combined cluster:

```yaml
status:
  bootstrapWithExistingClusterMembers:
    - name: <source-member-name>
      peerUrls:
        - <source-peer-url-recorded-from-spec>
      joinedAt: "2026-06-04T10:11:00Z"
```

This list records source members that were present when the target successfully bootstrapped with the existing cluster. It does not describe target members.

`joinedAt` is a shared bootstrap-completion timestamp. It is not a per-member join timestamp; all entries written in the same bootstrap completion have the same value.

### Condition

`BootstrappedWithExistingCluster` reports the join state:

| State | Condition | Meaning |
|-------|-----------|---------|
| Not in use | absent | `bootstrapWithExistingCluster` is not configured and no bootstrap status exists. |
| In progress | `False / BootstrapInProgress` | The target has not yet fully joined, or not all target members are ready. |
| Succeeded | `True / BootstrapSucceeded` | The target has joined successfully. Success is sticky once `.status.bootstrapWithExistingClusterMembers` is populated. |

The sticky success behavior is intentional: bootstrap is a one-time event. After the join has succeeded, transient pod restarts or member readiness changes should not make the historical bootstrap result regress to in-progress.

## What etcd-druid changes during the join

When `bootstrapWithExistingCluster` is configured, etcd-druid changes only the target `Etcd` resource it manages:

1. **ConfigMap `initial-cluster`:** source members from `.spec.etcd.bootstrapWithExistingCluster.members` are appended to the target's `initial-cluster` configuration together with target members.
2. **StatefulSet `--service-endpoints`:** source `clientEndpoints` are appended so the target sidecar can observe the combined cluster.
3. **Status:** once the join succeeds, `.status.bootstrapWithExistingClusterMembers` is written as the source-member snapshot.

etcd-druid does not modify an externally managed source cluster. For such sources, reachable advertised peer URLs and TLS trust must be prepared by the source operator before creating the target `Etcd` resource.

## Validation and mutability

`.spec.etcd.bootstrapWithExistingCluster` has to be part of the manifest applied at creation time on an `Etcd` resource; it cannot be added on an update. Once set, modification of `.spec.etcd.bootstrapWithExistingCluster.members` and `.spec.etcd.bootstrapWithExistingCluster.clientEndpoints` is not allowed until bootstrap is complete (`status.conditions[?(@.type=='BootstrappedWithExistingCluster')].status` is `True`).

At creation time, admission also catches the common authoring errors:

| Field | Constraint |
|-------|------------|
| `.spec.etcd.bootstrapWithExistingCluster.members` | required; between 1 and 10 entries |
| `.spec.etcd.bootstrapWithExistingCluster.members[*].name` | required; RFC 1123 label; must match the source member name |
| `.spec.etcd.bootstrapWithExistingCluster.members[*].peerUrls` | required; between 1 and 5 URLs per member; URL scheme must match `.spec.etcd.peerUrlTls` |
| `.spec.etcd.bootstrapWithExistingCluster.clientEndpoints` | required; between 1 and 10 entries; URL scheme must match `.spec.etcd.clientUrlTls` |

The URL-scheme constraints exist because the target reuses its own peer/client TLS configuration to dial the source. A scheme mismatch — for example, `http://` source URLs while the target enables `.spec.etcd.peerUrlTls` — would not produce a usable cluster and is rejected up front.

## TLS trust model

There is no separate source CA field on `bootstrapWithExistingCluster`. The target uses its normal etcd TLS configuration when dialing source endpoints:

- target etcd dials source peer URLs using `.spec.etcd.peerUrlTls`,
- target backup-restore sidecar dials source client endpoints using `.spec.etcd.clientUrlTls`.

URL schemes are validated at admission time, but certificate trust is validated only at runtime. If the target does not trust the source CA, or if the source does not accept the target peer certificates, the join will not complete and the condition remains `False / BootstrapInProgress`.

## Related

- [Bootstrapping with an Existing etcd Cluster](../usage/bootstrapping-with-existing-cluster.md) — user-facing setup and verification guide.
- [Using Additional Advertise Peer URLs](../usage/using-additional-advertise-peer-urls.md) — useful when an etcd-druid managed source must advertise peer URLs reachable from another network.
- [etcd Learner Design](https://etcd.io/docs/v3.5/learning/design-learner/) — upstream learner protocol.
- [etcd Clustering Guide](https://etcd.io/docs/v3.5/op-guide/clustering/) — etcd cluster formation and `initial-cluster` semantics.
