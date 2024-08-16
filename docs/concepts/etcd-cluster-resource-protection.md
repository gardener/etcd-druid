# Etcd Cluster Resource Protection

`etcd-druid` provisions [kubernetes resources (a.k.a components)](etcd-cluster-components.md) for each `Etcd` cluster. Lifecycle of all of these components is managed by etcd-druid. To ensure that each component's specification is in line with the configured attributes defined in `Etcd` custom resource and to protect unintended changes done to any of these *managed components* a [Validating Webhook](https://kubernetes.io/docs/reference/access-authn-authz/extensible-admission-controllers/) is employed.

[Etcd Components Webhook](https://github.com/gardener/etcd-druid/tree/55efca1c8f6c852b0a4e97f08488ffec2eed0e68/internal/webhook/etcdcomponents) is the *validating webhook* which prevents unintended *UPDATE* and *DELETE* operations on all managed resources. Following sections describe what is prohibited and in which specific conditions the changes are permitted.

## Configure Etcd Components Webhook

Prerequisite to enable the validation webhook is to [configure the Webhook Server](../deployment/configure-etcd-druid.md#webhook-server). Additionally you need to enable the `Etcd Components` validating webhook and optionally configure other options. You can look at all the options [here](../deployment/configure-etcd-druid.md#etcd-components-webhook).

## What is allowed?

Modifications to managed resources under the following circumstances will be allowed:

* `Create` and `Connect` operations are allowed and no validation is done.
* Changes to a kubernetes resource (e.g. StatefulSet, ConfigMap etc) not managed by etcd-druid are allowed.
* Changes to a resource whose Group-Kind is amongst the resources managed by etcd-druid but does not have a parent `Etcd` resource are allowed.
* It is possible that an operator wishes to explicitly disable etcd-component protection. This can be done by setting [annotation](https://github.com/gardener/etcd-druid/blob/55efca1c8f6c852b0a4e97f08488ffec2eed0e68/api/v1alpha1/constants.go#L30) on an `Etcd` resource. If this annotation is present then changes to managed components will be allowed.
* If `Etcd` resource has a deletion timestamp set indicating that it is marked for deletion and is awaiting etcd-druid to delete all managed resources then deletion requests for all managed resources for this etcd cluster will be allowed if:
  * The deletion request has come from a `ServiceAccount` associated to etcd-druid. If not explicitly specified via `--reconciler-service-account` then a [default-reconciler-service-account](https://github.com/gardener/etcd-druid/blob/55efca1c8f6c852b0a4e97f08488ffec2eed0e68/internal/webhook/etcdcomponents/config.go#L23) will be assumed.
  * The deletion request has come from a `ServiceAccount` configured via `--etcd-components-webhook-exempt-service-accounts`.
* `Lease` objects are periodically updated by each etcd member pod. A single `ServiceAccount` is created for all members. `Update` operation on `Lease` objects from [this ServiceAccount](https://github.com/gardener/etcd-druid/blob/55efca1c8f6c852b0a4e97f08488ffec2eed0e68/api/v1alpha1/helper.go#L28) is allowed.
* If an active reconciliation is in-progress then only allow operations that are initiated by etcd-druid.
* If no active reconciliation is currently in-progress, then allow updates to managed resource from `ServiceAccounts` configured via `--etcd-components-webhook-exempt-service-accounts`.
