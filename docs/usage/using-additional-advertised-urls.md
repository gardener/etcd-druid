# Using Additional Advertised URLs

This guide explains how to configure `spec.etcd.additionalAdvertisedURLs` to advertise extra client or peer URLs for etcd members managed by `etcd-druid`. This enables scenarios where members must be reachable via addresses beyond the default internal Kubernetes service DNS — for example, during live control-plane migrations or when load-balancer addresses must be advertised alongside the internal service URL.

## How It Works

By default, `etcd-druid` configures each etcd member's `--advertise-client-urls` and `--initial-advertise-peer-urls` flags to use the internal Kubernetes headless service DNS name. This is sufficient when all clients and peers reside within the same cluster.

`additionalAdvertisedURLs` contains two independent sub-fields:

| Sub-field | etcd flag affected |
|---|---|
| `clientURLs` | `--advertise-client-urls` |
| `peerURLs` | `--initial-advertise-peer-urls` and `--initial-cluster` |

Either or both may be set independently.

> [!NOTE]
> This field applies only to pods managed by etcd-druid. For externally managed members configured via `spec.externallyManagedMemberAddresses`, the provided addresses are used directly and `additionalAdvertisedURLs` has no effect.

### `overrideDefaultURL`

Both `clientURLs` and `peerURLs` support an `overrideDefaultURL` flag that controls whether the default internal service URL is included:

| `overrideDefaultURL` | Result |
|---|---|
| `false` (default) | Additional URLs are **appended** to the default internal URL. |
| `true` | Only the additional URLs are advertised; the default internal URL is **suppressed**. |


Set `overrideDefaultURL: true` when two clusters share an `Etcd` resource name and their internal service DNS names would otherwise collide — for example, during a live control-plane migration where the source and target clusters both use the same `Etcd` CR name.

### Affected ConfigMap Fields

| ConfigMap Field | Affected by |
|---|---|
| `advertise-client-urls` | `additionalAdvertisedURLs.clientURLs` |
| `initial-advertise-peer-urls` | `additionalAdvertisedURLs.peerURLs` |
| `initial-cluster` | `additionalAdvertisedURLs.peerURLs` |

## Field Reference

`spec.etcd.additionalAdvertisedURLs` is an object with two optional sub-fields:

### `clientURLs`

| Field | Type | Required | Description |
|---|---|---|---|
| `overrideDefaultURL` | `bool` | No (default `false`) | When `true`, suppresses the default internal client service URL for configured members. |
| `members` | `[]MemberURLs` | Yes | Per-member list of additional client URLs. Minimum 1, maximum 10 entries. |

### `peerURLs`

| Field | Type | Required | Description |
|---|---|---|---|
| `overrideDefaultURL` | `bool` | No (default `false`) | When `true`, suppresses the default internal pod DNS peer URL for configured members. |
| `members` | `[]MemberURLs` | Yes | Per-member list of additional peer URLs. Minimum 1, maximum 10 entries. |

### `MemberURLs`

| Field | Type | Required | Description |
|---|---|---|---|
| `name` | `string` | Yes | Name of the etcd member. Must match `{etcd-cr-name}-{index}` (e.g., `etcd-main-0`). When `spec.memberNamePrefix` is set, use `{prefix}-{etcd-cr-name}-{index}`. |
| `urls` | `[]string` | Yes | One or more additional URLs for this member. Maximum 5 per member. |

### Validation Rules

- The `name` in each `members` entry must start with the `Etcd` resource name followed by a dash.
- The numeric index at the end of `name` must be less than `spec.replicas`.
- A maximum of **10** member entries may be specified per sub-field.
- A maximum of **5** URLs may be specified per member.
- When the corresponding TLS config is set (`clientUrlTls` for client URLs, `peerUrlTls` for peer URLs), all URLs **must** use `https://`.
- When no TLS is configured, all URLs **must** use `http://`.
- When `overrideDefaultURL: true` is set, at least one member entry must be present.

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
    additionalAdvertisedURLs:
      clientURLs:
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

Each member advertises both its internal DNS URL and the external IP.

### Override Default Client and Peer URLs (TLS, live-CPM scenario)

Use `overrideDefaultURL: true` on both `clientURLs` and `peerURLs` when the internal DNS would collide with another cluster during a live control-plane migration:

```yaml
    additionalAdvertisedURLs:
      clientURLs:
        overrideDefaultURL: true
        members:
          - name: etcd-main-0
            urls:
              - https://lb-0.example.com:2379
          - name: etcd-main-1
            urls:
              - https://lb-1.example.com:2379
          - name: etcd-main-2
            urls:
              - https://lb-2.example.com:2379
      peerURLs:
        overrideDefaultURL: true
        members:
          - name: etcd-main-0
            urls:
              - https://lb-0.example.com:2380
          - name: etcd-main-1
            urls:
              - https://lb-1.example.com:2380
          - name: etcd-main-2
            urls:
              - https://lb-2.example.com:2380
```

Each member advertises **only** the external URL — the internal `svc` DNS URL is suppressed.

### Partial Configuration

You do not need to configure additional URLs for every member. Only members requiring external reachability need an entry:

```yaml
    additionalAdvertisedURLs:
      clientURLs:
        members:
          - name: etcd-main-0
            urls:
              - https://10.0.0.1:2379
```

Members without an entry continue to use only their default internal service DNS URL.

## Troubleshooting

**Validation error: member names must start with Etcd resource name**
: Ensure `name` starts with the `Etcd` resource name followed by a dash. For `etcd-main`, valid names are `etcd-main-0`, `etcd-main-1`, etc.

**Validation error: member name index must be less than replicas**
: The numeric index must be within bounds. For `spec.replicas: 3`, valid indices are `0`, `1`, and `2`.

**Validation error: URLs must use https:// / http://**
: URL schemes must be consistent with the corresponding TLS config. If TLS is configured, use `https://`. If not, use `http://`.

**Validation error: overrideDefaultURL=true requires at least one member with URLs**
: When `overrideDefaultURL: true` is set, the `members` list must have at least one entry.

**External clients cannot reach members**
: Verify that the addresses in `members[*].urls` are reachable from the external network and that firewall rules allow traffic on the appropriate port.
