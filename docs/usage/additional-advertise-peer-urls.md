# Using Additional Advertise Peer URLs

## Overview

The `additionalAdvertisePeerUrls` feature allows you to specify additional peer URLs that etcd members should advertise beyond the default internal Kubernetes service DNS URLs. This enables cross-cluster etcd communication via external endpoints such as LoadBalancer IPs or DNS names.

## Use Cases

- **Live Cluster Migration**: Enable etcd members from different clusters to communicate during migration
- **Multi-Cluster Setup**: Connect etcd clusters across different Kubernetes clusters
- **External Access**: Expose etcd peer communication through load balancers or external DNS

## How It Works

When you configure `additionalAdvertisePeerUrls`, etcd-druid appends these URLs to the default internal service URLs in the etcd  configuration's `initial-advertise-peer-urls` parameter.

**Example**: For member `etcd-main-0` with additional URL `https://52.1.2.3:2380`, the resulting configuration will be:

```
initial-advertise-peer-urls=https://etcd-main-0.etcd-main-peer.default.svc:2380,https://52.1.2.3:2380
```

## Configuration

### Basic Example

```yaml
apiVersion: druid.gardener.cloud/v1alpha1
kind: Etcd
metadata:
  name: etcd-main
  namespace: default
spec:
  replicas: 3
  labels:
    app: etcd
  backup:
    store:
      provider: S3
      # ... backup configuration
  etcd:
    # Enable peer TLS for secure communication
    peerUrlTls:
      tlsCASecretRef:
        name: etcd-peer-ca
      serverTLSSecretRef:
        name: etcd-peer-server-tls
    
    # Configure additional peer URLs
    additionalAdvertisePeerUrls:
      - memberName: etcd-main-0
        urls:
          - "https://load-balancer-0.example.com:2380"
      - memberName: etcd-main-1
        urls:
          - "https://load-balancer-1.example.com:2380"
      - memberName: etcd-main-2
        urls:
          - "https://load-balancer-2.example.com:2380"
```

### Multiple URLs Per Member

You can specify multiple additional URLs for each member:

```yaml
additionalAdvertisePeerUrls:
  - memberName: etcd-main-0
    urls:
      - "https://primary-lb-0.example.com:2380"
      - "https://backup-lb-0.example.com:2380"
      - "https://10.0.0.1:2380"
  - memberName: etcd-main-1
    urls:
      - "https://primary-lb-1.example.com:2380"
```

### Without TLS

If `peerUrlTls` is not configured, use HTTP URLs:

```yaml
etcd:
  additionalAdvertisePeerUrls:
    - memberName: etcd-main-0
      urls:
        - "http://10.0.0.1:2380"
    - memberName: etcd-main-1
      urls:
        - "http://10.0.0.2:2380"
```

## Field Reference

### AdditionalPeerURL

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `memberName` | string | Yes | The etcd member name (e.g., `etcd-main-0`). Must match the actual etcd member name in the cluster. Non-matching names are silently ignored. |
| `urls` | []string | Yes | List of additional peer URLs for this member. These will be appended to the default internal service URL. |

## Validation Rules

The following validation rules apply:

### URL Format

- Must be valid absolute URLs with scheme `http://` or `https://`
- Must include a host and port
- Must NOT include a path

✅ **Valid**:

- `https://example.com:2380`
- `http://10.0.0.1:2380`
- `https://load-balancer.region.cloud:2380`

❌ **Invalid**:

- `https://example.com` (missing port)
- `https://example.com:2380/path` (includes path)
- `ftp://example.com:2380` (wrong scheme)

### TLS Scheme Matching

- If `peerUrlTls` is enabled, all URLs must use `https://`
- If `peerUrlTls` is not configured, all URLs must use `http://`

### Member Name

- Must be at least 1 character long
- Should match the etcd member name (pod name) to take effect
- Non-matching names are silently ignored (no error)

## TLS Certificate Requirements

### For Cross-Cluster Communication

> [!IMPORTANT]
> When using `peerUrlTls` with `additionalAdvertisePeerUrls` for cross-cluster communication (e.g., live migration), **all etcd members across different clusters MUST use TLS certificates signed by the same Certificate Authority (CA)**.

This is required because:

- Etcd members validate each other's certificates during peer communication
- All peers must trust the certificates presented by other members
- Without a shared CA, TLS handshakes will fail and members cannot join the cluster

### Certificate Setup Example

**Source Cluster:**

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: etcd-peer-ca
type: Opaque
data:
  ca.crt: <base64-encoded-shared-ca-cert>
---
apiVersion: v1
kind: Secret
metadata:
  name: etcd-peer-server-tls
type: kubernetes.io/tls
data:
  tls.crt: <base64-encoded-cert-signed-by-shared-ca>
  tls.key: <base64-encoded-private-key>
```

**Target Cluster:**

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: etcd-peer-ca
type: Opaque
data:
  ca.crt: <base64-encoded-SAME-shared-ca-cert>  # Must be identical
---
apiVersion: v1
kind: Secret
metadata:
  name: etcd-peer-server-tls
type: kubernetes.io/tls
data:
  tls.crt: <base64-encoded-cert-signed-by-SAME-shared-ca>
  tls.key: <base64-encoded-private-key>
```

### Certificate Generation

When generating certificates for cross-cluster communication:

1. **Create a shared CA** (do this once):

   ```bash
   # Generate CA private key
   openssl genrsa -out ca-key.pem 2048
   
   # Generate CA certificate
   openssl req -x509 -new -nodes -key ca-key.pem \
     -days 3650 -out ca.crt -subj "/CN=etcd-ca"
   ```

2. **Generate member certificates** for each cluster using the same CA:

   ```bash
   # For source cluster member

  openssl req -new -key member-key.pem -out member.csr \
     -subj "/CN=etcd-source-0" \
     -addext "subjectAltName=DNS:etcd-source-0.etcd-source-peer.default.svc,DNS:source-lb-0.example.com,IP:10.0.0.1"

# Sign with shared CA

   openssl x509 -req -in member.csr -CA ca.crt -CAkey ca-key.pem \
     -CAcreateserial -out member.crt -days 365 -copy_extensions=copy

   ```

3. **Deploy the same CA** to both clusters

4. **Deploy member-specific certificates** to each cluster

## Best Practices

1. **Use TLS in Production**: Always enable `peerUrlTls` and use HTTPS URLs for secure peer communication

2. **Shared CA for Cross-Cluster**: When connecting etcd members across clusters, ensure all members use certificates signed by the same CA

3. **Include All Endpoints in SAN**: When generating TLS certificates, include all advertise peer URLs (LoadBalancer IPs, DNS names) in the Subject Alternative Name (SAN) field

4. **Match Member Names**: Ensure `memberName` values exactly match your etcd member names (pod names)

5. **Verify DNS Resolution**: Ensure all specified URLs are resolvable from the etcd pods

6. **Test Connectivity**: Verify network connectivity to all additional peer URLs before deploying

7. **Use LoadBalancers**: For cross-cluster communication, use LoadBalancer services or external DNS names

## Troubleshooting

### URLs Not Being Used

**Symptom**: Additional peer URLs don't appear in etcd configuration

**Solutions**:

- Verify `memberName` exactly matches the etcd member name (check pod name)
- Check for typos in the member name
- Remember that non-matching names are silently ignored

### Validation Errors

**Symptom**: Etcd resource creation fails with validation error

**Solutions**:

- Ensure URL scheme matches TLS configuration (https with TLS, http without)
- Verify all URLs include a port number
- Check that URLs don't include paths
- Confirm URL scheme is http or https (not ftp, etc.)

### Connection Issues

**Symptom**: Etcd members cannot communicate via additional URLs

**Solutions**:

- Verify network connectivity between clusters/endpoints
- Check firewall rules allow traffic on port 2380
- Ensure DNS resolution works for hostname-based URLs
- **Verify TLS certificates are signed by the same CA** (critical for cross-cluster)
- Check certificate SANs include all advertise peer URLs
- Review etcd logs for TLS handshake errors

## Migration Example

For live cluster migration scenarios, configure both source and target clusters:

**Target Cluster** (new cluster joining existing):

```yaml
apiVersion: druid.gardener.cloud/v1alpha1
kind: Etcd
metadata:
  name: etcd-target
spec:
  replicas: 3
  etcd:
    peerUrlTls:
      # ... TLS configuration
    additionalAdvertisePeerUrls:
      # Point to source cluster members via LoadBalancer
      - memberName: etcd-target-0
        urls:
          - "https://source-lb-0.example.com:2380"
      - memberName: etcd-target-1
        urls:
          - "https://source-lb-1.example.com:2380"
      - memberName: etcd-target-2
        urls:
          - "https://source-lb-2.example.com:2380"
```

## See Also

- [API Reference](../api-reference/etcd-druid-api.md#additionalpeerurl)
- [Live Etcd Cluster Migration Proposal](../proposals/07-live-etcd-cluster-migration.md)
- [Securing Etcd Clusters](securing-etcd-clusters.md)
