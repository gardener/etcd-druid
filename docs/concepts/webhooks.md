# Webhooks

The [etcd-druid controller-manager](controllers.md#controller-manager) registers certain [admission webhooks](https://kubernetes.io/docs/reference/access-authn-authz/extensible-admission-controllers/) that allow for validation or mutation of requests on resources in the cluster, in order to prevent misconfiguration and restrict access to the etcd cluster resources.

All webhooks that are a part of etcd-druid reside in package `internal/webhook`, as sub-packages.

## Package Structure

The typical package structure for the webhooks that are part of etcd-druid is shown with the *EtcdComponents Webhook*:

``` bash
internal/webhook/etcdcomponents
├── config.go
├── handler.go
└── register.go
```

- `config.go`: contains all the logic for the configuration of the webhook, including feature gate activations, CLI flag parsing and validations.
- `register.go`: contains the logic for registering the webhook with the etcd-druid controller manager.
- `handler.go`: contains the webhook admission handler logic.

Each webhook package may also contain auxiliary files which are relevant to that specific webhook.

## Etcd Components Webhook

Druid controller-manager registers and runs the [etcd controller](controllers.md#etcd-controller), which creates and manages various components/resources such as `Leases`, `ConfigMap`s, and the `Statefulset` for the etcd cluster. It is essential for all these resources to contain correct configuration for the proper functioning of the etcd cluster.

Unintended changes to any of these *managed resources* can lead to misconfiguration of the etcd cluster, leading to unwanted downtime for etcd traffic. To prevent such unintended changes, a validating webhook called *EtcdComponents Webhook* guards these managed resources, ensuring that only authorized entities can perform operations on these managed resources.

*EtcdComponents webhook* prevents *UPDATE* and *DELETE* operations on all resources managed by *etcd controller*, unless such an operation is performed by druid itself, and during reconciliation of the `Etcd` resource. Operations are also allowed if performed by one of the authorized entities specified by CLI flag `--etcd-components-webhook-exempt-service-accounts`, but only if the `Etcd` resource is not being reconciled by etcd-druid at that time.

There may be specific cases where a human operator may need to make changes to the managed resources, possibly to test or fix an etcd cluster. An example of this is [recovery from permanent quorum loss](../operations/recovery-from-permanent-quorum-loss-in-etcd-cluster.md), where a human operator will need to suspend reconciliation of the `Etcd` resource, make changes to the underlying managed resources such as `StatefulSet` and `ConfigMap`, and then resume reconciliation for the `Etcd` resource. Such manual interventions will require out-of-band changes to the managed resources. Protection of managed resources for such `Etcd` resources can be turned off by adding an annotation `druid.gardener.cloud/disable-etcd-component-protection` on the `Etcd` resource. This will effectively disable *EtcdComponents Webhook* protection for all managed resources for the specific `Etcd`.

**Note:** *UPDATE* operations for `Lease`s by etcd members are always allowed, since these are regularly updated by the etcd-backup-restore sidecar.

The *Etcd Components Webhook* is disabled by default, and can be enabled via the CLI flag `--enable-etcd-components-webhook.
