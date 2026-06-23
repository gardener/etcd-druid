# Bootstrapping with an Existing etcd Cluster

This guide explains how to configure `spec.etcd.bootstrapWithExistingCluster` to make a new `Etcd` resource managed by `etcd-druid` join an already-running etcd cluster (managed or not managed by etcd-druid) instead of starting as a standalone cluster.

The conceptual background — terminology, lifecycle states, condition semantics, status shape, TLS trust model, etc. — is covered in [Bootstrap with an Existing etcd Cluster](../concepts/bootstrap-with-existing-cluster.md). This guide focuses on the **how**: prerequisites, step-by-step setup, and verification.

> [!NOTE]
> This feature covers only the **join phase**: target members are added to the source cluster and synchronized. Removal of the original source members is tracked separately in [DEP-08](https://github.com/gardener/etcd-druid/pull/1369) and is **not implemented yet**. Until DEP-08 lands, the end state is a *combined* cluster of source + target members; if your goal is full migration, plan to remove source members manually for now.

## Prerequisites

Before configuring `bootstrapWithExistingCluster` on a new `Etcd` resource, ensure:

1. **All source etcd cluster members are healthy and reachable.** Every peer URL listed under `bootstrapWithExistingCluster.members[*].peerUrls` must be dialable from the target's network namespace, and at least one source client endpoint must be reachable from the target's `etcd-backup-restore` sidecar. If source and target run in different network domains, list URLs that the source has explicitly advertised for cross-domain reachability — see the IMPORTANT note in step 1 below.
2. **Source member names and peer URLs are known and stable.** You'll list them verbatim in the target etcd resource spec; they must match what the source advertises in its own member list (`etcdctl member list --endpoints=<source-client-endpoints>`).
3. **TLS trust is set up if either side uses TLS.** See the [concept doc's TLS Trust Model](../concepts/bootstrap-with-existing-cluster.md#tls-trust-model) — this is the most common source of failure.
4. **The new `Etcd` resource has not yet been created.** `bootstrapWithExistingCluster` is **create-only** — it cannot be added to an existing `Etcd` resource. Plan the field into the manifest you apply at creation time.
5. **Source and target run the same etcd minor version.** etcd's join (member-add / learner-promote) flow does **not** support cross-minor-version joins — both clusters must be on the same `3.X.*` series. The exact target version is fixed by the `etcd-wrapper` image referenced from `.spec.etcd.image`:
    - With the [`UpgradeEtcdVersion` feature gate](../deployment/feature-gates.md#feature-gates-for-alpha-or-beta-features) enabled, `etcd-druid` selects an `etcd-wrapper` image bundling etcd `3.5.x` for the target — the source must therefore also be on a `3.5.x` release.
    - With `UpgradeEtcdVersion` disabled (the current default), `etcd-druid` selects an `etcd-wrapper` image bundling etcd `3.4.x` for the target — the source must therefore also be on a `3.4.x` release.

## Step-by-Step Setup

### 1. Inspect the source cluster

Run the following from a host (or pod) that can reach the source cluster:

```bash
etcdctl --endpoints=<source-client-endpoint> \
  --cacert=<source-ca-path> \
  --cert=<source-client-cert-path> \
  --key=<source-client-key-path> \
  member list -w table
```

> [!NOTE]
> If the source cluster does not have TLS enabled, omit the `--cacert`, `--cert`, and `--key` flags.

Sample output:

```
+------------------+---------+---------------+---------------------------------------------------------+---------------------------------------------------------+
|        ID        | STATUS  |     NAME      |                       PEER ADDRS                        |                      CLIENT ADDRS                       |
+------------------+---------+---------------+---------------------------------------------------------+---------------------------------------------------------+
| abc...           | started | etcd-source-0 | https://etcd-source-0.etcd-source-peer.source-ns.svc:2380 | https://etcd-source-client.source-ns.svc:2379           |
| def...           | started | etcd-source-1 | https://etcd-source-1.etcd-source-peer.source-ns.svc:2380 | https://etcd-source-client.source-ns.svc:2379           |
| 012...           | started | etcd-source-2 | https://etcd-source-2.etcd-source-peer.source-ns.svc:2380 | https://etcd-source-client.source-ns.svc:2379           |
+------------------+---------+---------------+---------------------------------------------------------+---------------------------------------------------------+
```

Record each member's **`NAME`** and **`PEER ADDRS`** — they go into `bootstrapWithExistingCluster.members`. Record one or more **`CLIENT ADDRS`** for `bootstrapWithExistingCluster.clientEndpoints`.

> [!IMPORTANT]
> If source and target run in **different network domains** (separate Kubernetes clusters, on-prem ↔ cloud, etc.), the source must advertise peer URLs that are routable from the target. For etcd-druid managed sources, set `.spec.etcd.additionalAdvertisePeerURLs` on the source — see [Using Additional Advertise Peer URLs](using-additional-advertise-peer-urls.md). For externally managed sources, configure the source etcd's `--initial-advertise-peer-urls` directly. Use those externally-routable URLs in the target spec.

### 2. Author the target `Etcd` manifest

Add `bootstrapWithExistingCluster` to the target spec. A minimal 3-replica example:

```yaml
apiVersion: druid.gardener.cloud/v1alpha1
kind: Etcd
metadata:
  name: etcd-target
  namespace: target-ns
spec:
  replicas: 3
  etcd:
    image: <etcd-wrapper-image>
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
    peerUrlTls:
      tlsCASecretRef:
        name: ca-etcd-peer-combined
        namespace: target-ns
      serverTLSSecretRef:
        name: etcd-target-server
        namespace: target-ns
    clientUrlTls:
      tlsCASecretRef:
        name: ca-etcd-client-combined
        namespace: target-ns
      serverTLSSecretRef:
        name: etcd-target-server-client
        namespace: target-ns
      clientTLSSecretRef:
        name: etcd-target-client
        namespace: target-ns
  backup: {}
  # ... other fields as needed
```

### 3. Apply the manifest

```bash
kubectl apply -f etcd-target.yaml
```

`etcd-druid` will:

- Append source member peer URLs to the target's `initial-cluster` ConfigMap entry.
- Append source `clientEndpoints` to the `--service-endpoints` flag of the target's `etcd-backup-restore` sidecar.
- Bring up target pods, which join the source cluster.

### 4. Watch the bootstrap progress

Track the dedicated condition:

```bash
kubectl -n target-ns get etcd etcd-target \
  -o jsonpath='{.status.conditions[?(@.type=="BootstrappedWithExistingCluster")]}{"\n"}'
```

While the join is in progress:

```json
{"type":"BootstrappedWithExistingCluster","status":"False","reason":"BootstrapInProgress","message":"Not all members have joined the cluster yet"}
```

`.status.bootstrapWithExistingClusterMembers` is **empty** (or absent) until the join succeeds — the snapshot is written atomically once all target members have joined. To watch in-progress per-member state, inspect `.status.members` (the standard member list) instead.

Once it succeeds:

```json
{"type":"BootstrappedWithExistingCluster","status":"True","reason":"BootstrapSucceeded","message":"All members have successfully joined the existing cluster"}
```

The condition is **sticky**: once it flips to `True`, it stays `True` for the lifetime of the `Etcd` resource — pod restarts and `StatefulSet` rolls do not regress it.

### 5. Verify the combined cluster

The `etcd-wrapper` container uses a [distroless](https://github.com/GoogleContainerTools/distroless) image and ships neither `etcdctl` nor a shell, so `kubectl exec` cannot run `etcdctl` inside it. The recommended verification path is `kubectl debug` — attach an ephemeral container that already has `etcdctl`, sharing the target pod's network namespace and mounted client TLS secret. This avoids extracting TLS material onto the operator's workstation.

```bash
kubectl -n target-ns debug -it pod/etcd-target-0 \
  --image=europe-docker.pkg.dev/sap-se-gcp-k8s-delivery/releases-public/europe-docker_pkg_dev/gardener-project/releases/gardener/ops-toolbelt:latest \
  --target=etcd \
  -- /bin/bash
```

The `ops-toolbelt` image pre-configures the `ETCDCTL_*` environment variables (endpoint, CA, client cert/key) for the targeted etcd pod, so `etcdctl` can be invoked without repeating the flags. List the members:

```sh
etcdctl member list -w table
```

Expected: 6 members total: 3 source members + 3 target members — all `started`.

Verify that source and target members report the **same `clusterID`** — proof that the target has joined the source cluster rather than forming a separate cluster:

```sh
etcdctl endpoint status \
  --endpoints=<source-client-endpoint>,https://localhost:2379 \
  -w table
```

The `CLUSTER ID` column must be identical for the source endpoint and the target endpoint.

> [!NOTE]
> If your cluster does not allow ephemeral debug containers (`kubectl debug` is gated by the `EphemeralContainers` feature gate and may be restricted by admission policy), use a one-off pod scheduled to the same node and namespace running an `etcdctl` image, with the client TLS `Secret`s mounted as volumes. Avoid copying secret material to a workstation.

The target's status records the source members it joined to:

```bash
kubectl -n target-ns get etcd etcd-target \
  -o jsonpath='{.status.bootstrapWithExistingClusterMembers}{"\n"}'
```

```json
[
  {"name":"etcd-source-0","peerUrls":["https://etcd-source-0.etcd-source-peer.source-ns.svc:2380"],"joinedAt":"2026-06-04T10:11:00Z"},
  {"name":"etcd-source-1","peerUrls":["https://etcd-source-1.etcd-source-peer.source-ns.svc:2380"],"joinedAt":"2026-06-04T10:11:00Z"},
  {"name":"etcd-source-2","peerUrls":["https://etcd-source-2.etcd-source-peer.source-ns.svc:2380"],"joinedAt":"2026-06-04T10:11:00Z"}
]
```

`joinedAt` is set once when the bootstrap completes and is the same value across all entries.

## Related

- [Concept doc: Bootstrap with an Existing etcd Cluster](../concepts/bootstrap-with-existing-cluster.md) — terminology, lifecycle states, status semantics, TLS trust model.
- [Using Additional Advertise Peer URLs](using-additional-advertise-peer-urls.md) — frequently paired when source and target run in different network domains.
- [Securing Etcd Clusters](securing-etcd-clusters.md) — TLS configuration reference for `peerUrlTls` / `clientUrlTls`.
- [DEP-08](https://github.com/gardener/etcd-druid/pull/1369) — design proposal for source-member removal (companion to this feature).
- [etcd Learner Design](https://etcd.io/docs/v3.5/learning/design-learner/) — upstream documentation on the learner protocol used internally during the join.
