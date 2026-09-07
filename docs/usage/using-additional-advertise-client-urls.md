# Using Additional Advertise Client URLs

This guide explains how to configure `spec.etcd.additionalAdvertiseClientURLs` to advertise extra client URLs for etcd members managed by `etcd-druid`. This enables scenarios where clients outside a Kubernetes cluster need to reach individual etcd members directly — for example, during live control-plane migrations or when a load-balancer or NodePort address must be advertised alongside the internal service URL.

## How It Works

By default, `etcd-druid` configures each etcd member's `--advertise-client-urls` flag to use the internal Kubernetes headless service DNS name. This is sufficient when all clients reside within the same cluster.

When `additionalAdvertiseClientURLs` is configured, `etcd-druid` appends the specified URLs to `advertise-client-urls` for matching members. Peer URLs are **not** affected — only client advertise URLs are extended.

### `overrideDefaultURL`

The `overrideDefaultURL` field controls whether the default internal client service URL is included:

| `overrideDefaultURL` | Result |
|---|---|
| `false` (default) | Additional URLs are **appended** to the default internal URL. |
| `true` | Only the additional URLs are advertised; the default internal URL is **suppressed**. |

Set `overrideDefaultURL: true` when two clusters share an `Etcd` resource name and their internal client service DNS names would otherwise collide — for example, during a live control-plane migration where the source and target clusters both use the same `Etcd` CR name.

### Affected ConfigMap Field

When `additionalAdvertiseClientURLs` is set, `etcd-druid` modifies one field in the generated etcd ConfigMap:

| ConfigMap Field | Effect |
|---|---|
| `advertise-client-urls` | Additional URLs are appended to (or replace) the per-member client advertise URL list. |

The `initial-advertise-peer-urls` and `initial-cluster` fields are **not** affected.

## Field Reference

`spec.etcd.additionalAdvertiseClientURLs` is an object with two fields:

| Field | Type | Required | Description |
|---|---|---|---|
| `overrideDefaultURL` | `bool` | No (default `false`) | When `true`, suppresses the default internal client service URL for configured members. Only honored for members that have URLs configured — members without a matching entry in `members` always use the default internal URL. |
| `members` | `[]MemberClientURLs` | Yes | Per-member list of additional client URLs. Minimum 1, maximum 10 entries. |

Each `members` entry:

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | `string` | Yes | Name of the etcd member. Must match the pattern `{etcd-cr-name}-{index}` (e.g., `etcd-main-0`). |
| `urls` | `[]string` | Yes | One or more additional client URLs. Maximum 5 per member. |

### Validation Rules

The following validations are enforced at admission time via [CEL](https://kubernetes.io/docs/reference/using-api/cel/) expressions:

- The `name` must start with the `Etcd` resource name followed by a dash (e.g., for an `Etcd` named `etcd-main`, valid names are `etcd-main-0`, `etcd-main-1`, etc.).
- The numeric index at the end of `name` must be less than `spec.replicas`.
- A maximum of **10** member entries may be specified.
- A maximum of **5** URLs may be specified per member.
- When `spec.etcd.clientUrlTls` is configured, all URLs **must** use the `https://` scheme.
- When `spec.etcd.clientUrlTls` is not configured, all URLs **must** use the `http://` scheme.
- URLs must include scheme and host; port is optional (e.g., `https://10.0.0.1:2379`).

> [!WARNING]
> Invalid member names or scheme mismatches are **rejected** by the API server. Ensure member names and URL schemes are correct before applying.

## Examples

### Append External Client URLs (Non-TLS)

```yaml
apiVersion: druid.gardener.cloud/v1alpha1
kind: Etcd
metadata:
  name: etcd-main
  namespace: default
spec:
  replicas: 3
  etcd:
    additionalAdvertiseClientURLs:
      overrideDefaultURL: false
      members:
        - name: etcd-main-0
          urls:
            - http://10.0.0.1:2379
        - name: etcd-main-1
          urls:
            - http://10.0.0.2:2379
        - name: etcd-main-2
          urls:
            - http://10.0.0.3:2379
```

Each member will advertise both its internal DNS URL and the external IP.

### Override Default URL (TLS, collision-avoidance)

Use `overrideDefaultURL: true` when the internal DNS would collide with another cluster:

```yaml
    clientUrlTls:
      tlsCASecretRef:
        name: etcd-client-ca
        namespace: default
      serverTLSSecretRef:
        name: etcd-client-server-tls
        namespace: default
      clientTLSSecretRef:
        name: etcd-client-tls
        namespace: default
    additionalAdvertiseClientURLs:
      overrideDefaultURL: true
      members:
        - name: etcd-main-0
          urls:
            - https://10.0.0.1:2379
        - name: etcd-main-1
          urls:
            - https://10.0.0.2:2379
        - name: etcd-main-2
          urls:
            - https://10.0.0.3:2379
```

Each member advertises **only** the external URL — the internal `svc` DNS URL is suppressed.

### Partial Configuration

You do not need to configure additional URLs for every member. Only members requiring external reachability need an entry:

```yaml
    additionalAdvertiseClientURLs:
      members:
        - name: etcd-main-0
          urls:
            - https://10.0.0.1:2379
```

Members without an entry continue to use only their default internal service DNS URL.

## Troubleshooting

**Validation error: member name must start with Etcd resource name**
: Ensure `name` starts with the `Etcd` resource name followed by a dash. For `etcd-main`, valid names are `etcd-main-0`, `etcd-main-1`, etc.

**Validation error: member name index must be less than replicas**
: The numeric index must be within bounds. For `spec.replicas: 3`, valid indices are `0`, `1`, and `2`.

**Validation error: URLs must use https:// / http://**
: URL schemes must be consistent with `spec.etcd.clientUrlTls`. If TLS is configured, use `https://`. If not, use `http://`.

**External clients cannot reach members**
: Verify that the addresses in `additionalAdvertiseClientURLs.members[*].urls` are reachable from the external network and that firewall rules allow traffic on port 2379.
