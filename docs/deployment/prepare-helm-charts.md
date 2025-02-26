# Prepare etcd-druid Helm charts

`etcd-druid` operator can be deployed via [helm charts](https://helm.sh/). The charts can be found [here](https://github.com/gardener/etcd-druid/tree/master/charts/). All `Makefile` `deploy*` targets employ [skaffold](https://skaffold.dev/) which internally uses the same helm charts to deploy all resources to setup etcd-druid. In the following sections you will learn on the prerequisites, generated/copied resources and kubernetes resources that are deployed via helm charts to setup etcd-druid.

## Prerequisite

### Installing Helm

If you wish to directly use helm charts then please ensure that helm is already installed.

On MacOS you can install via `brew`:
```bash
brew install helm
```

For all other OS please check [Helm installation instructions](https://helm.sh/docs/intro/install/).

### Installing OpenSSL

[OpenSSL](https://www.openssl.org/) is used to generate PKI resources that are used to configure TLS connectivity with the webhook(s). 
On MacOS you can install via brew:

```bash
brew install openssl
```

For all other OS please check [OpenSSL download instructions](https://github.com/openssl/openssl?tab=readme-ov-file#download).

> NOTE: On linux, the library is available via native package managers like `apt`, `yum` etc. On Windows, you can get the installer [here](https://slproweb.com/products/Win32OpenSSL.html).

##  Generated/Copied resources

To leverage etcd-druid helm charts you need to ensure that the charts contains the required CRD yaml files and PKI resources.

### CRDs

[Heml-3](https://helm.sh/docs/topics/charts/#custom-resource-definitions-crds) provides special status to CRDs. CRD YAML files should be placed in `crds/` directory inside of a chart. Helm will attempt to load all the files in this directory. We now generate the CRDs and keep these at `etcd-druid/api/core/crds` which serves as a single source of truth for all custom resource specifications under etcd-druid operator. These CRDs needs to be copied to `etcd-druid/charts/crds`. 

### PKI resources

Webhooks communicate over TLS with the kube-apiserver. It is therefore essential to generate PKI resources (CA certificate, Server certificate and Server key) to be used to configure Webhook configuration and mount it to the etcd-druid operator `Deployment`. 

## Kubernetes Resources

etcd-druid helm charts creates the following kubernetes resources:

| Resource                                                     | Description                                                  |
| ------------------------------------------------------------ | ------------------------------------------------------------ |
| ApiVersion: apps/v1<br />Kind: Deployment                    | This is the etcd-druid Deployment which runs etcd-druid operator. All reconcilers run as part of this operator. |
| ApiVersion: rbac.authorization.k8s.io/v1<br />Kind: ClusterRole | etcd-druid manages `Etcd` resources deployed across namespaces. A cluster role provides required roles to etcd-druid operator for all the resources that are created for an etcd cluster. |
| ApiVersion: v1<br />Kind: ServiceAccount                     | It defines a system user with which etcd-druid operator will function. The service account name will be configured in the Deployment at `spec.template.spec.serviceAccountName` |
| ApiVersion:rbac.authorization.k8s.io/v1<br />Kind: ClusterRoleBinding | Binds the cluster roles to the `ServiceAccount` thus associating all cluster roles to the system user with which etcd-druid operator will be run. |
| ApiVersion: v1<br />Kind: Service                            | ClusterIP service which will provide a logical endpoint to reach any etcd-druid pods. |
| ApiVersion: v1<br />Kind: Secret                             | A secret containing the webhook server certificate and key will be created and mounted onto the Deployment. |
| ApiVersion: admissionregistration.k8s.io/v1<br />Kind: ValidatingWebhookConfiguration | It is the validation webhook configuration. Currently there is only one webhook `etcdcomponents` . For more details see [here](../concepts/etcd-cluster-resource-protection.md). |

## Chart Values

A [values.yaml](https://github.com/gardener/etcd-druid/blob/master/charts//values.yaml) is defined which contains the default set of values for all configurable properties. You can change the values as per your needs. A few properties of note:

`image`: Points to the image URL that will be configured in etcd-druid `Deployment`. If you are building the image on your own and pushing it to the repository of your choice then ensure that you change the value accordingly.

`webhooks.pki`: This YAML map contains paths to required PKI artifacts. If you are generating these on your own then ensure that you provide correct paths to these resources.

`webhooks.etcdComponents.enabled`: By default the `etcd-components` webhook is enabled. This is a good default for production environments. However while you are actively developing then you can choose to disable this webhook.

## Makefile target

A convenience Makefile target `make prepare-helm-charts` is provided which leverages `OpenSSL` to generate the required PKI artifacts.
If you wish to deploy `etcd-druid` in a specific namespace then prior to running this Makefile target you can run:

```bash
export NAMESPACE=<namespace>
```

!!! info
	Specifying a namespace other than `default` will result in additional SAN being added in the webhook server certificate.

By default the certificates generated have an expiry of 12h. If you wish to have a different expiry then prior to running this Makefile target you can run:

```bash
export CERT_EXPIRY=<duration-of-your-choice>
# example: export CERT_EXPIRY=6h
```

!!! note

​	If you are using `make deploy*` targets directly which leverages [skaffold](https://skaffold.dev/) then Makefile target `prepare-helm-charts` will be invoked automatically.