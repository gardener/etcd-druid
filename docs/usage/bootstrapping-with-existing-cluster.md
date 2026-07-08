# Bootstrapping with an Existing etcd Cluster

This guide shows how to create a new etcd-druid managed `Etcd` resource that joins an already-running etcd cluster instead of starting as a standalone cluster.

The source cluster may be managed by etcd-druid or externally managed. In both cases, the source member names, peer URLs, and client endpoints that you put into the target spec must already be valid from the target's network.

For background and lifecycle semantics, see [Bootstrap with an Existing etcd Cluster](../concepts/bootstrap-with-existing-cluster.md). This guide uses "source cluster", "target `Etcd` resource", and "join phase" as defined in that document's [Terminology](../concepts/bootstrap-with-existing-cluster.md#terminology) section.

## Prerequisites

Before creating the target `Etcd` resource, ensure:

1. **The source cluster is healthy and reachable from the target's network.** For each source member you list in `.spec.etcd.bootstrapWithExistingCluster.members`, its peer URL must be dialable from the target pods, and the member must be responsive. `etcdctl member list --write-out=table` reporting `started` for the member is necessary but not sufficient — confirm the member also responds to `etcdctl endpoint health` before you list it.
2. **The source peer URLs in the target spec are reachable from the target pods.** Only the URLs listed under `.spec.etcd.bootstrapWithExistingCluster.members[*].peerUrls` need to be reachable from the target network.
3. **At least one source client endpoint is reachable from the target backup-restore sidecar.** These endpoints go into `.spec.etcd.bootstrapWithExistingCluster.clientEndpoints`.
4. **The source and target use compatible TLS trust roots.** The target's peer/client TLS configuration is used when dialing source peer/client endpoints. See the [TLS trust model](../concepts/bootstrap-with-existing-cluster.md#tls-trust-model).
5. **The target `Etcd` resource does not already exist.** `bootstrapWithExistingCluster` is create-only and cannot be added to an existing `Etcd` resource.
6. **Source and target run the same etcd minor version.** The etcd version `etcd-druid` deploys on the target depends on the [`UpgradeEtcdVersion`](../deployment/feature-gates.md#feature-gates-for-alpha-or-beta-features) feature gate:
    - With `UpgradeEtcdVersion` enabled, `etcd-druid` deploys etcd `3.5.x` on the target; the source must also be on `3.5.x`.
    - With `UpgradeEtcdVersion` disabled (current default), `etcd-druid` deploys etcd `3.4.x` on the target; the source must also be on `3.4.x`.

## Step 1: Inspect the source cluster

Run `etcdctl member list` against the source cluster from a location that can reach its client endpoint:

```bash
etcdctl --endpoints=<source-client-endpoint> \
  --cacert=<source-ca-path> \
  --cert=<source-client-cert-path> \
  --key=<source-client-key-path> \
  member list -w table
```

If the source cluster does not use TLS, omit the TLS flags.

Example output:

```text
+------------------+---------+---------------+-----------------------------------------------------------------------------------+---------------------------------------------------------+
|        ID        | STATUS  |     NAME      |                                    PEER ADDRS                                     |                      CLIENT ADDRS                       |
+------------------+---------+---------------+-----------------------------------------------------------------------------------+---------------------------------------------------------+
| abc...           | started | etcd-source-0 | https://etcd-source-0.etcd-source-peer.source-ns.svc:2380,https://10.0.0.1:2380   | https://etcd-source-client.source-ns.svc:2379           |
| def...           | started | etcd-source-1 | https://etcd-source-1.etcd-source-peer.source-ns.svc:2380,https://10.0.0.2:2380   | https://etcd-source-client.source-ns.svc:2379           |
| 012...           | started | etcd-source-2 | https://etcd-source-2.etcd-source-peer.source-ns.svc:2380,https://10.0.0.3:2380   | https://etcd-source-client.source-ns.svc:2379           |
+------------------+---------+---------------+-----------------------------------------------------------------------------------+---------------------------------------------------------+
```

Record:

- each source member `NAME`,
- the source peer URLs that are reachable from the target pods,
- one or more source client endpoints reachable from the target backup-restore sidecar.

> [!IMPORTANT]
> The `PEER ADDRS` shown above are same-network examples. If source and target run in different network domains, replace them with peer URLs that the source cluster advertises and the target can dial.
>
> For an etcd-druid managed source, configure [`.spec.etcd.additionalAdvertisePeerURLs`](../usage/using-additional-advertise-peer-urls.md) on the source if extra reachable peer URLs are needed. For an externally managed source, prepare the advertised peer URLs outside etcd-druid. The target spec must use the source member names and peer URLs as advertised by the source member list.

## Step 2: Create the target `Etcd` manifest

Add `spec.etcd.bootstrapWithExistingCluster` when creating the target resource:

```yaml
apiVersion: druid.gardener.cloud/v1alpha1
kind: Etcd
metadata:
  name: etcd-target
  namespace: target-ns
spec:
  # replicas is the target-side member count only. The joined cluster ends up with
  # (replicas + number of source members listed below) etcd members in total.
  replicas: 3
  etcd:
    image: <etcd-wrapper-image>
    bootstrapWithExistingCluster:
      members:
        - name: etcd-source-0
          peerUrls:
            - https://etcd-source-0.etcd-source-peer.source-ns.svc:2380
            - https://10.0.0.1:2380
        - name: etcd-source-1
          peerUrls:
            - https://etcd-source-1.etcd-source-peer.source-ns.svc:2380
            - https://10.0.0.2:2380
        - name: etcd-source-2
          peerUrls:
            - https://etcd-source-2.etcd-source-peer.source-ns.svc:2380
            - https://10.0.0.3:2380
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

Use `http://` URLs when TLS is not enabled for the corresponding peer/client side. Admission validation rejects URL schemes that do not match `.spec.etcd.peerUrlTls` or `.spec.etcd.clientUrlTls`.

## Step 3: Apply the manifest

```bash
kubectl apply -f etcd-target.yaml
```

etcd-druid then reconciles the target resources:

- the target ConfigMap `initial-cluster` includes both target members and source members,
- the target backup-restore `--service-endpoints` includes the target client service and the configured source `clientEndpoints`,
- the target pods start and join the existing source cluster.

## Step 4: Watch join progress

Check the dedicated condition:

```bash
kubectl -n target-ns get etcd etcd-target \
  -o jsonpath='{.status.conditions[?(@.type=="BootstrappedWithExistingCluster")]}{"\n"}'
```

While the join is still in progress, the condition is `False`:

```json
{"type":"BootstrappedWithExistingCluster","status":"False","reason":"BootstrapInProgress","message":"Not all members have joined the cluster yet"}
```

During this phase `.status.bootstrapWithExistingCluster` is absent. To inspect per-target-member readiness, use `.status.members`.

After the join succeeds, the condition becomes `True`:

```json
{"type":"BootstrappedWithExistingCluster","status":"True","reason":"BootstrapSucceeded","message":"All members have successfully joined the existing cluster"}
```

The success condition is sticky. Once true, pod restarts or transient readiness changes do not reset the historical bootstrap result.

## Step 5: Verify the combined cluster

The `etcd-wrapper` container is distroless and does not ship a shell or `etcdctl`. Use an ephemeral debug container with `etcdctl` instead of copying TLS secrets to your workstation:

```bash
kubectl -n target-ns debug -it pod/etcd-target-0 \
  --image=europe-docker.pkg.dev/sap-se-gcp-k8s-delivery/releases-public/europe-docker_pkg_dev/gardener-project/releases/gardener/ops-toolbelt:latest 
```

List the combined membership:

```sh
etcdctl member list -w table
```

Expected: source members and target members are present, and all are `started`.

Verify that source and target endpoints report the same cluster ID:

```sh
etcdctl endpoint status \
  --endpoints=<source-client-endpoint>,https://localhost:2379 \
  -w table
```

The `CLUSTER ID` column must match for source and target endpoints. If the IDs differ, the target formed or contacted a different cluster instead of joining the source cluster.

> [!NOTE]
> If ephemeral debug containers are not allowed in your environment, use a short-lived pod in the same namespace with `etcdctl` and mount the required client TLS secrets as volumes. Avoid copying certificate/key material to a workstation.

## Step 6: Inspect recorded source-member status

After the join succeeds, etcd-druid writes the source-member inventory:

```bash
kubectl -n target-ns get etcd etcd-target \
  -o jsonpath='{.status.bootstrapWithExistingCluster}{"\n"}'
```

Example:

```json
{
  "joinedAt": "2026-06-04T10:11:00Z",
  "members": [
    {"name":"etcd-source-0","peerUrls":["https://etcd-source-0.etcd-source-peer.source-ns.svc:2380","https://10.0.0.1:2380"]},
    {"name":"etcd-source-1","peerUrls":["https://etcd-source-1.etcd-source-peer.source-ns.svc:2380","https://10.0.0.2:2380"]},
    {"name":"etcd-source-2","peerUrls":["https://etcd-source-2.etcd-source-peer.source-ns.svc:2380","https://10.0.0.3:2380"]}
  ]
}
```

This status records source members that were present when the target completed bootstrap. `joinedAt` is a single timestamp for the whole snapshot — the moment etcd-druid recorded the successful bootstrap.

## Related

- [Concept doc: Bootstrap with an Existing etcd Cluster](../concepts/bootstrap-with-existing-cluster.md)
- [Using Additional Advertise Peer URLs](using-additional-advertise-peer-urls.md)
- [Securing Etcd Clusters](securing-etcd-clusters.md)
- [etcd Learner Design](https://etcd.io/docs/v3.5/learning/design-learner/)
