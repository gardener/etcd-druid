# Bootstrap with an Existing etcd Cluster

`etcd-druid` can bootstrap a new `Etcd` resource by joining it to an already-running etcd cluster instead of creating a standalone cluster. It uses etcd's native [learner](https://etcd.io/docs/v3.5/learning/design-learner/) mechanism to add new members, synchronize data, and promote them to voting members once they are fully caught up.

This capability enables two common workflows:

- **Migration** — Move etcd state from one cluster to another without restoring snapshots or copying data. New members join the existing cluster, synchronize automatically, and eventually replace the original members.
- **Extension** — Add new etcd-druid managed members to an existing cluster while keeping the original members in service.

In both cases, the source cluster continues serving reads and writes throughout the operation.

The feature currently covers only the **join phase**: adding target members as learners, synchronizing data, and promoting them to voting members. Automated removal of the original source members is proposed in [DEP #1355](https://github.com/gardener/etcd-druid/pull/1355) and will build on the status and trigger semantics described in this document.

> [!NOTE]
> Source-member removal is **not yet implemented**. Clearing `.spec.etcd.bootstrapWithExistingCluster` currently only arms the future removal trigger.

## Terminology

| Term | Definition |
|------|------------|
| **Source etcd cluster** | The existing cluster whose data is being joined. The source may be managed by etcd-druid, self-managed, or hosted outside Kubernetes. |
| **Target etcd cluster** | The new etcd-druid managed cluster that joins the source. Its members are added as learners, synchronize data, and are promoted to voting members. |
| **Combined etcd cluster** | The temporary state in which source and target members form a single etcd cluster. During this period, learners do not contribute to quorum until promoted. |
| **Learner** | A non-voting etcd member that receives Raft log entries but does not participate in quorum decisions. Target members are added as learners before promotion. Because etcd allows only **one learner at a time**, target members join sequentially. |
| **Join phase** | The bootstrap lifecycle during which target members are added as learners, promoted to voting members, and recorded in status. |
| **Removal phase** | The future reconciliation proposed in DEP #1355 that removes source members after migration is complete. |

## Topology

This document uses the following example topology:

- **`etcd-source`** — an existing 3-member etcd cluster.
- **`etcd-target`** — a new 3-member etcd-druid managed cluster configured with `.spec.etcd.bootstrapWithExistingCluster`.

During the join phase, target members join the source cluster one at a time as learners. Each learner synchronizes with the current leader and is promoted before the next learner is added — etcd permits only a single learner in a cluster at any given time, which forces the strict sequence.

As members are promoted by `etcd-backup-restore`, the cluster's voting set grows from **3 → 4 → 5 → 6** and quorum tracks it (`2 → 3 → 3 → 4`). Once all three target members are voting, the combined cluster is `source + target = 6` voting members with quorum 4. The source cluster remains fully available throughout the process.

> [!NOTE]
> If source and target run in different network domains (separate Kubernetes clusters, on-prem to cloud, etc.), each source member must publish a peer URL that the target can dial. For etcd-druid managed sources this is `.spec.etcd.additionalAdvertisePeerURLs` — see [Using Additional Advertise Peer URLs](../usage/using-additional-advertise-peer-urls.md). For externally managed sources, configure the source etcd's `--initial-advertise-peer-urls` directly. The target's `.spec.etcd.bootstrapWithExistingCluster.members[*].peerUrls` must match what the source advertises in its member list.

```text
┌────────────────────────────────────────────────────────────────────────────────┐
│         Combined etcd cluster — join in progress                               │
│         (3 voting + 1 learner; 2 target members not yet joined)                │
│                                                                                │
│   ┌─────────────┐    ┌─────────────┐    ┌─────────────┐                        │
│   │etcd-source-0│    │etcd-source-1│    │etcd-source-2│   voting members       │
│   │   leader    │◄──►│  follower   │◄──►│  follower   │   (quorum = 2)         │
│   └──────┬──────┘    └─────────────┘    └─────────────┘                        │
│          │                                                                     │
│          │  leader streams Raft log to the current learner                     │
│          ▼                                                                     │
│   ┌─────────────┐    ┌─────────────┐    ┌─────────────┐                        │
│   │etcd-target-0│    │etcd-target-1│    │etcd-target-2│                        │
│   │   learner   │    │   pending   │    │   pending   │                        │
│   │ catching up │    │ (MemberAdd  │    │ (MemberAdd  │                        │
│   │             │    │  blocks     │    │  blocks     │                        │
│   │             │    │  until      │    │  until      │                        │
│   │             │    │  target-0   │    │  target-1   │                        │
│   │             │    │  promoted)  │    │  promoted)  │                        │
│   └─────────────┘    └─────────────┘    └─────────────┘                        │
└────────────────────────────────────────────────────────────────────────────────┘
```

The diagram is a snapshot mid-sequence. Once `etcd-target-0` is promoted, `etcd-target-1` is added as the next learner; once it is promoted, `etcd-target-2` follows. After the source-member removal proposed in DEP #1355 completes, only the target remains: a 3-member etcd cluster with quorum 2, with `.spec.etcd.bootstrapWithExistingCluster` cleared and `.status.bootstrapWithExistingClusterMembers` cleared by the removal reconciliation.

## How etcd-druid Implements the Join

When `.spec.etcd.bootstrapWithExistingCluster` is configured, `etcd-druid` modifies three runtime artifacts:

### 1. ConfigMap (`initial-cluster`)

The target cluster's `initial-cluster` configuration is extended with entries for every source member.

The resulting configuration contains, in order:

1. All target member peer URLs (including any `additionalAdvertisePeerURLs`).
2. All source member peer URLs.

This allows target pods to discover the source cluster during startup. Each target pod attempts to join using `MemberAdd(learner=true)`. Because only one learner is allowed at a time, subsequent pods wait until the previous learner has been promoted.

### 2. StatefulSet (`--service-endpoints`)

The `etcd-backup-restore` sidecar is configured with the source cluster's client endpoints by appending `.spec.etcd.bootstrapWithExistingCluster.clientEndpoints` to `--service-endpoints`.

This enables the sidecar to:

- Probe source-cluster health.
- Monitor learner synchronization progress.
- Promote learners using `MemberPromote` once they have caught up.
- Coordinate the sequential learner workflow.

### 3. Status Reconciliation

After all target members become Ready, the `BootstrapWithExistingCluster` condition transitions to `True` with reason `BootstrapSucceeded`.

During the same reconciliation pass, `etcd-druid` records the source-member inventory in `.status.bootstrapWithExistingClusterMembers`, assigning a shared `joinedAt` timestamp to every entry.

This snapshot is written only once:

- Existing entries are never overwritten.
- `joinedAt` is never modified.
- Missing `peerUrls` may be backfilled during later reconciliations for compatibility with older API versions.

### Code references

- [`prepareInitialCluster`](https://github.com/gardener/etcd-druid/blob/master/internal/component/configmap/etcdconfig.go) in `internal/component/configmap/etcdconfig.go` — extends `initial-cluster` with source members.
- [StatefulSet builder](https://github.com/gardener/etcd-druid/blob/master/internal/component/statefulset/builder.go) in `internal/component/statefulset/builder.go` — appends source client endpoints to the backup-restore sidecar's `--service-endpoints`.
- [`check_bootstrap_with_existing_cluster.go`](https://github.com/gardener/etcd-druid/blob/master/internal/health/condition/check_bootstrap_with_existing_cluster.go) — implements the `BootstrapWithExistingCluster` condition check.
- [`mutateBootstrapWithExistingClusterStatus`](https://github.com/gardener/etcd-druid/blob/master/internal/controller/etcd/reconcile_status.go) in `internal/controller/etcd/reconcile_status.go` — populates `.status.bootstrapWithExistingClusterMembers` once the condition is `True` and back-fills empty `peerUrls` on later reconciles.

## API Surface

### Spec

```yaml
spec:
  etcd:
    bootstrapWithExistingCluster:
      members:
        - name: etcd-source-0
          peerUrls:
            - https://etcd-source-0.etcd-source-peer.source-ns.svc:2380
        - name: etcd-source-1
          peerUrls:
            - https://etcd-source-1.etcd-source-peer.source-ns.svc:2380
        - name: etcd-source-2
          peerUrls:
            - https://etcd-source-2.etcd-source-peer.source-ns.svc:2380
      clientEndpoints:
        - https://etcd-source-client.source-ns.svc:2379
```

### Status

```yaml
status:
  bootstrapWithExistingClusterMembers:
    - name: etcd-source-0
      peerUrls:
        - https://etcd-source-0.etcd-source-peer.source-ns.svc:2380
      joinedAt: "2026-06-04T10:11:00Z"
    - name: etcd-source-1
      peerUrls:
        - https://etcd-source-1.etcd-source-peer.source-ns.svc:2380
      joinedAt: "2026-06-04T10:11:00Z"
    - name: etcd-source-2
      peerUrls:
        - https://etcd-source-2.etcd-source-peer.source-ns.svc:2380
      joinedAt: "2026-06-04T10:11:00Z"
```

`.status.bootstrapWithExistingClusterMembers` is a flat `[]BootstrapJoinedMember` list on `EtcdStatus`. The status reconciler writes all entries in a single call with one shared timestamp; `joinedAt` is set once and never updated, giving operators a single record of when the target finished its join phase against the source.

### Condition

The `BootstrapWithExistingCluster` condition type is added to `.status.conditions` when the feature is in use. It tracks whether the join has completed successfully. See [Lifecycle States](#lifecycle-states) for the full state machine.

### Validation

The validations below are enforced at admission time — partly by kubebuilder validation markers (required, list-size, regex), partly by CEL rules (URL-scheme coupling, mutability). Together they prevent the most common authoring mistakes.

| Field | Constraint |
|-------|------------|
| `.members` | required; **at most 10** entries |
| `.members[*].name` | required; RFC 1123 label (pattern `^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`, max 253 chars) — must match a real source member name |
| `.members[*].peerUrls` | required; **at most 5** URLs per member; each must be `https://` when `.spec.etcd.peerUrlTls` is set, `http://` otherwise |
| `.clientEndpoints` | required; **at most 10** entries; each must be `https://` when `.spec.etcd.clientUrlTls` is set, `http://` otherwise |

> [!NOTE]
> The list-size caps (5 URLs per member, 10 members, 10 client endpoints) are imposed by Kubernetes' per-rule **CEL validation cost budget** — the URL-scheme rules iterate over every entry, so cost scales with list length. Raising the limits requires restructuring the CEL rules first.

URL-scheme mismatches are rejected at admission time. **Certificate-trust mismatches are detected only at runtime** — see [TLS Requirements](#tls-requirements).

#### Mutability

A CEL transition rule controls whether `.spec.etcd.bootstrapWithExistingCluster` can be set or cleared on an update:

```text
!has(self.bootstrapWithExistingCluster) || has(oldSelf.bootstrapWithExistingCluster)
```

| # | Old has field? | New has field? | Allowed? | Meaning |
|---|----------------|----------------|----------|---------|
| 1 | no | no | Yes | Non-bootstrap clusters stay non-bootstrap |
| 2 | yes | yes | Yes | Edits to `members`/`clientEndpoints` after creation |
| 3 | no | yes | **No** | Cannot retrofit the feature onto an existing `Etcd` |
| 4 | yes | no | Yes | Clears the spec — arms the removal trigger |

> [!WARNING]
> The API server rejects retrofit attempts (row 3) with the message:
> *"etcd.spec.etcd.bootstrapWithExistingCluster cannot be added after the Etcd resource has been created"*


## Lifecycle States

The feature progresses through three observable states:

| State | Condition | When |
|-------|-----------|------|
| **Not in use** | absent (no `BootstrapWithExistingCluster` condition is emitted) | Spec unset and status empty. Either a standard standalone cluster, or — once [DEP #1355](https://github.com/gardener/etcd-druid/pull/1355) lands — the terminal state after source-member removal. |
| **In progress** | `False / BootstrapInProgress` | Spec set, status empty; either not all target pods are up yet (`len(.status.members) < .spec.replicas`) or some target members are not Ready. |
| **Succeeded** | `True / BootstrapSucceeded` (sticky) | Status populated. Stays `True` for the lifetime of the resource — including after spec is cleared to arm the removal trigger. |

The condition check evaluates `len(.status.bootstrapWithExistingClusterMembers) > 0` **before** inspecting the spec, which is why **Succeeded** is sticky regardless of whether spec is set or cleared.

### Behavior across restarts

Once the **Succeeded** state is reached, pod restarts, `StatefulSet` rolls, and `etcd-druid` restarts do not regress the condition — the join phase is recorded as a one-time fact in `.status`, not re-evaluated as a recurring readiness probe. If `etcd-druid` restarts *before* the condition first flips to `True`, the next reconcile re-evaluates from spec and observed pod readiness; nothing is lost because no status was written yet. If a target pod restarts mid-join, etcd's data directory either contains a partial replica that catches up by replaying the leader's log, or is empty and re-joins as a fresh learner via the same `MemberAdd` flow.

## TLS Requirements

URL-scheme rules are enforced at admission time and documented under [Validation](#validation). This section covers the runtime CA-trust workflow that the operator must set up *outside* of API admission.

There is no separate source-CA reference field on `bootstrapWithExistingCluster`. At runtime, the target etcd and backup-restore processes dial source endpoints using the target's own TLS trust roots:

- Target etcd dials source peer URLs over TLS using `.spec.etcd.peerUrlTls` trust roots.
- Target `etcd-backup-restore` dials source client endpoints using `.spec.etcd.clientUrlTls` trust roots.

When TLS is enabled, the operator must:

1. **Copy the source cluster's CA certificate** to the target cluster.
2. **Create a combined CA bundle** containing both the target CA and the source CA. Store this bundle in the `Secret`s referenced by `.spec.etcd.peerUrlTls.tlsCASecretRef` and `.spec.etcd.clientUrlTls.tlsCASecretRef`.
3. **Sign the target's TLS certificates** using a CA the source also trusts. The source etcd members must accept incoming TLS connections from target members during the join phase.

If both clusters already share a common CA, no merging is needed — the shared CA handles trust in both directions.

> [!WARNING]
> A URL-scheme mismatch is rejected at admission time. A certificate-trust mismatch is detected only at runtime: the target etcd exits with `tls: failed to verify certificate: x509: certificate is not trusted` and the `BootstrapWithExistingCluster` condition stays at `False/BootstrapInProgress`.

## Related

- [Using Additional Advertise Peer URLs](../usage/using-additional-advertise-peer-urls.md) — frequently paired when source and target are in different network domains
- [etcd Cluster Components](etcd-cluster-components.md) — resources `etcd-druid` manages for every `Etcd` resource
- [DEP #1355](https://github.com/gardener/etcd-druid/pull/1355) — design proposal for the source-member removal reconciliation
- [etcd Learner Design](https://etcd.io/docs/v3.5/learning/design-learner/) — upstream documentation on the learner protocol
- [etcd Clustering Guide](https://etcd.io/docs/v3.5/op-guide/clustering/) — etcd cluster formation and `initial-cluster` semantics
