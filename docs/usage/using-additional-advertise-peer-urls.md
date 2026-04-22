# Using Additional Advertise Peer URLs

This guide explains how to configure `spec.etcd.additionalAdvertisePeerUrls` to advertise extra peer URLs for etcd members managed by `etcd-druid`. This enables scenarios where etcd members hosted in Kubernetes need to participate in peer communication across network boundaries — for example, between an on-premises environment and a cloud-hosted Kubernetes cluster.

> [!NOTE]
> This feature was introduced in etcd-druid `v0.37.0`. Support for bootstrapping etcd clusters with external members (once [#1239](https://github.com/gardener/etcd-druid/issues/1239) is implemented) will build on this field.

## How It Works

By default, `etcd-druid` configures each etcd member's `--initial-advertise-peer-urls` and `--initial-cluster` flags to use internal Kubernetes service DNS names. This is sufficient when all members reside within the same Kubernetes cluster:

```
┌────────────────────────────────────────────────────────────────┐
│                       Kubernetes Cluster                       │
│                                                                │
│   ┌────────────┐      ┌────────────┐      ┌────────────┐      │
│   │etcd-main-0 │      │etcd-main-1 │      │etcd-main-2 │      │
│   │            │      │            │      │            │      │
│   │ Advertises │      │ Advertises │      │ Advertises │      │
│   │ DNS only   │      │ DNS only   │      │ DNS only   │      │
│   └─────┬──────┘      └─────┬──────┘      └─────┬──────┘      │
│         │                   │                    │             │
│         │    Peer Communication (DNS :2380)      │             │
│         └───────────────────┼────────────────────┘             │
│                             │                                  │
└────────────────────────────────────────────────────────────────┘
  No external access — peers communicate only via internal DNS.
```

When `additionalAdvertisePeerUrls` is configured, `etcd-druid` appends the specified URLs to these flags for matching members. Each member then advertises both its internal DNS URL **and** the additional external URLs (e.g., LoadBalancer IPs), making it reachable from outside the cluster. Client advertise URLs are **not** affected — only peer URLs are extended.

```
┌─────────────────────────────────────────────────────────────────┐
│                        Kubernetes Cluster                        │
│                                                                  │
│    ┌────────────┐      ┌────────────┐      ┌────────────┐       │
│    │etcd-main-0 │      │etcd-main-1 │      │etcd-main-2 │       │
│    │            │      │            │      │            │       │
│    │ Advertises │      │ Advertises │      │ Advertises │       │
│    │ DNS +      │      │ DNS +      │      │ DNS +      │       │
│    │ 10.0.0.1   │      │ 10.0.0.2   │      │ 10.0.0.3   │       │
│    └──┬──────┬──┘      └──┬──────┬──┘      └──┬──────┬──┘       │
│       │      │             │      │             │      │         │
│       │      │   Internal peer communication   │      │         │
│       │      └─────────────┼──────┴─────────────┘      │         │
│       │                    │                           │         │
│       │  LB 10.0.0.1      │  LB 10.0.0.2              │         │
│       │                    │                 LB 10.0.0.3         │
├───────┼────────────────────┼───────────────────────────┼─────────┤
│       │   :2380            │   :2380                   │  :2380  │
└───────┴────────────────────┴───────────────────────────┴─────────┘
        │                    │                           │
        ▼                    ▼                           ▼
┌─────────────────────────────────────────────────────────────────┐
│                        External Network                          │
│                                                                  │
│  External peers can reach K8s-hosted etcd members via the        │
│  LoadBalancer IPs advertised in additionalAdvertisePeerUrls.     │
│  Communication is bidirectional over port 2380.                  │
│                                                                  │
└─────────────────────────────────────────────────────────────────┘
```

Each member has two communication paths:

- **Internal (right line):** Peers within the cluster communicate via the headless Service DNS names as before.
- **External (left line):** The same member is also reachable from outside via a LoadBalancer IP, which is advertised in `--initial-advertise-peer-urls` thanks to `additionalAdvertisePeerUrls`.

### Affected ConfigMap Fields

When `additionalAdvertisePeerUrls` is set, `etcd-druid` appends the additional URLs to two fields in the generated etcd ConfigMap:

| ConfigMap Field | Effect |
|---|---|
| `initial-advertise-peer-urls` | Additional URLs are appended to the per-member peer advertise URL list. |
| `initial-cluster` | Each additional URL is added as a separate `member=url` entry in the cluster membership string. |

The `advertise-client-urls` field is **not** affected — only peer URLs are extended.

For example, given a 3-replica cluster named `etcd-main` with `additionalAdvertisePeerUrls` configured for `etcd-main-0` with `https://10.0.1.10:2380`:

**`initial-advertise-peer-urls`** for `etcd-main-0`:

```
https://etcd-main-0.etcd-main-peer.default.svc:2380,https://10.0.1.10:2380
```

**`initial-cluster`**:

```
etcd-main-0=https://etcd-main-0.etcd-main-peer.default.svc:2380,etcd-main-0=https://10.0.1.10:2380,etcd-main-1=https://etcd-main-1.etcd-main-peer.default.svc:2380,etcd-main-2=https://etcd-main-2.etcd-main-peer.default.svc:2380
```

Note that each additional URL produces a separate `member=url` entry in `initial-cluster` (this is the format [etcd expects](https://etcd.io/docs/v3.5/op-guide/clustering/#etcd-discovery) for multiple peer URLs per member).

## Prerequisites

1. **Network connectivity** between the Kubernetes cluster and external etcd members.
2. **DNS or IP routing** configured so that the addresses specified in `additionalAdvertisePeerUrls` are reachable from outside the cluster.
3. **TLS certificates** with SANs covering the external IPs/hostnames, if peer TLS is enabled.
4. Familiarity with [etcd clustering concepts](https://etcd.io/docs/v3.5/op-guide/clustering/).

## Field Reference

`spec.etcd.additionalAdvertisePeerUrls` is a list of `AdditionalPeerURL` entries:

| Field | Type | Required | Description |
|---|---|---|---|
| `memberName` | `string` | Yes | Name of the etcd member. Must match the pattern `{etcd-cr-name}-{index}` (e.g., `etcd-main-0`). |
| `urls` | `[]string` | Yes | One or more additional peer URLs to advertise for this member. Maximum 5 per member. |

### Validation Rules

The following validations are enforced at admission time via [CEL](https://kubernetes.io/docs/reference/using-api/cel/) expressions:

- The `memberName` must start with the `Etcd` resource name followed by a dash (e.g., for an `Etcd` named `etcd-main`, valid names are `etcd-main-0`, `etcd-main-1`, etc.).
- The numeric index at the end of `memberName` must be less than `spec.replicas`.
- A maximum of **10** entries may be specified.
- A maximum of **5** URLs may be specified per member.
- When `spec.etcd.peerUrlTls` is configured, all URLs **must** use the `https://` scheme.
- When `spec.etcd.peerUrlTls` is not configured, all URLs **must** use the `http://` scheme.
- URLs must include scheme, host, and port (e.g., `https://10.0.0.1:2380`).

> [!WARNING]
> Invalid member names or scheme mismatches are **rejected** by the API server — they do not silently fall through. Ensure the member name and URL schemes are correct before applying.

## Examples

### Non-TLS Cluster

```yaml
apiVersion: druid.gardener.cloud/v1alpha1
kind: Etcd
metadata:
  name: etcd-main
  namespace: default
spec:
  replicas: 3
  etcd:
    metrics: basic
    resources:
      limits:
        cpu: 500m
        memory: 1Gi
      requests:
        cpu: 100m
        memory: 200Mi
    additionalAdvertisePeerUrls:
      - memberName: etcd-main-0
        urls:
          - http://10.0.0.1:2380
      - memberName: etcd-main-1
        urls:
          - http://10.0.0.2:2380
      - memberName: etcd-main-2
        urls:
          - http://10.0.0.3:2380
  backup:
    port: 8080
    fullSnapshotSchedule: "0 */24 * * *"
    garbageCollectionPolicy: Exponential
  sharedConfig:
    autoCompactionMode: periodic
    autoCompactionRetention: "30m"
```

### TLS-Enabled Cluster

When `spec.etcd.peerUrlTls` is configured, all additional peer URLs must use `https://`:

```yaml
    peerUrlTls:
      tlsCASecretRef:
        name: etcd-peer-ca
        namespace: default
      serverTLSSecretRef:
        name: etcd-peer-server-tls
        namespace: default
    additionalAdvertisePeerUrls:
      - memberName: etcd-main-0
        urls:
          - https://10.0.0.1:2380
      - memberName: etcd-main-1
        urls:
          - https://10.0.0.2:2380
      - memberName: etcd-main-2
        urls:
          - https://10.0.0.3:2380
```

> [!WARNING]
> Ensure the TLS server certificate includes the external IPs or hostnames as Subject Alternative Names (SANs). Without this, peer TLS handshakes from external members will fail.

### Multiple URLs per Member

Each member supports up to 5 additional URLs. This is useful when a member needs to be reachable via more than one external address (e.g., multiple load balancers or a failover IP):

```yaml
    additionalAdvertisePeerUrls:
      - memberName: etcd-main-0
        urls:
          - https://10.0.0.1:2380
          - https://lb-primary.example.com:2380
      - memberName: etcd-main-1
        urls:
          - https://10.0.0.2:2380
```

### Partial Configuration

You do not need to configure additional URLs for every member. Only members that need external reachability require an entry:

```yaml
    additionalAdvertisePeerUrls:
      - memberName: etcd-main-0
        urls:
          - https://10.0.0.1:2380
```

Members without an entry use only their default internal service DNS URL.

## Exposing Members via LoadBalancer Services

A common pattern for making etcd members reachable across network boundaries is to create one `LoadBalancer` Service per member. Each Service selects a single StatefulSet pod using the `statefulset.kubernetes.io/pod-name` label that Kubernetes automatically sets.

> [!NOTE]
> These LoadBalancer Services are **user-managed** resources separate from the Services created by `etcd-druid`. Do not modify druid-managed Services.

Example for one member:

```yaml
apiVersion: v1
kind: Service
metadata:
  name: etcd-main-0-peer-lb
  namespace: default
spec:
  type: LoadBalancer
  selector:
    statefulset.kubernetes.io/pod-name: etcd-main-0
  ports:
    - name: peer
      port: 2380
      targetPort: 2380
      protocol: TCP
```

Create one Service per member, then retrieve the assigned external IPs:

```bash
for i in 0 1 2; do
  echo "etcd-main-${i}: $(kubectl get svc etcd-main-${i}-peer-lb -n default \
    -o jsonpath='{.status.loadBalancer.ingress[0].ip}')"
done
```

Use the assigned IPs in `additionalAdvertisePeerUrls`.

> [!NOTE]
> If the cloud provider assigns a DNS hostname instead of an IP (`.status.loadBalancer.ingress[0].hostname`), use the hostname in the URL. The URL must still include the port.

> [!NOTE]
> On [KinD](https://kind.sigs.k8s.io/) clusters, `LoadBalancer` Services remain in `Pending` state. For local testing, use in-cluster DNS names (e.g., `http://etcd-main-0-peer-lb.default.svc.cluster.local:2380`) or a tool such as [MetalLB](https://metallb.universe.tf/).

## Applying Changes

To add or update `additionalAdvertisePeerUrls` on an existing `Etcd` resource:

```bash
kubectl patch etcd etcd-main -n default --type merge -p '{
  "spec": {
    "etcd": {
      "additionalAdvertisePeerUrls": [
        {"memberName": "etcd-main-0", "urls": ["http://10.0.0.1:2380"]},
        {"memberName": "etcd-main-1", "urls": ["http://10.0.0.2:2380"]},
        {"memberName": "etcd-main-2", "urls": ["http://10.0.0.3:2380"]}
      ]
    }
  }
}'
```

Reconciliation is triggered automatically on `Etcd` resource updates (post `v0.23.0`). Refer to [Managing Etcd Clusters](managing-etcd-clusters.md) for further details.

## Troubleshooting

**Validation error: member name must start with Etcd resource name**
: Ensure `memberName` starts with the `Etcd` resource name followed by a dash. For `etcd-main`, valid names are `etcd-main-0`, `etcd-main-1`, etc.

**Validation error: member name index must be less than replicas**
: The numeric index must be within bounds. For `spec.replicas: 3`, valid indices are `0`, `1`, and `2`.

**Validation error: URLs must use https:// / http://**
: URL schemes must be consistent with `spec.etcd.peerUrlTls`. If TLS is configured, use `https://`. If not, use `http://`.

**External members cannot reach Kubernetes-hosted peers**
: Verify that the addresses in `additionalAdvertisePeerUrls` are reachable from the external network, firewall rules allow traffic on port 2380, and DNS/IP routes are correctly configured.
