# Managing ETCD Clusters

## Create an Etcd Cluster

Creating an `Etcd` cluster can be done either by explicitly creating a manifest file or it can also be done programmatically.  You can refer to and/or modify any [sample](../../config/samples/) `Etcd` manifest to create an etcd cluster. In order to programmatically create an `Etcd` cluster you can refer to the `Golang` [API](../../api/v1alpha1/)  to create an `Etcd` custom resource and using a [k8s client](https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/client#Client) you can apply an instance of a `Etcd` custom resource targetting any namespace in a k8s cluster.

Prior to `v0.23.0` version of etcd-druid, after creating an `Etcd` custom resource, you will have to annotate the resource with `gardener.cloud/operation=reconcile` in order to trigger a reconciliation for the newly created `Etcd` resource. Post `v0.23.0` version of etcd-druid, there is no longer any need to explicitly trigger reconciliations for creating new `Etcd` clusters.

### Track etcd cluster creation

In order to track the progress of creation of etcd cluster resources you can do the following:

* `status.lastOperation` can be monitored to check the status of reconciliation.

* Additional printer columns have been defined for `Etcd` custom resource. You can execute the following command to know if an `Etcd` cluster is ready/quorate.
  ```bash
  kubectl get etcd <etcd-name> -n <namespace> -owide
  # you will see additional columns which will indicate the state of an etcd cluster
  NAME        READY   QUORATE   ALL MEMBERS READY   BACKUP READY   AGE    CLUSTER SIZE   CURRENT REPLICAS   READY REPLICAS
  etcd-main   true    True      True                True           235d   3              3                  3
  ```

* You can additional monitor [all etcd cluster resources](../concepts/etcd-cluster-components.md) that are created for every etcd cluster. 

  For etcd-druid version <v0.23.0 use the following command:
  ```bash
  kubectl get all,cm,role,rolebinding,lease,sa -n <namespace> --selector=instance=<etcd-name>
  ```

  For etcd-druid version >=v0.23.0 use the following command:
  ```bash
  kubectl get all,cm,role,rolebinding,lease,sa -n <namespace> --selector=app.kubernetes.io/managed-by=etcd-druid,app.kubernetes.io/part-of=<etcd-name>
  ```

  

## Update & Reconcile an Etcd Cluster

### Edit the Etcd custom resource

To update an etcd cluster, you should usually *only* be updating the `Etcd` custom resource representing the etcd cluster.
You can make changes to the existing `Etcd` resource by invoking the following command:

```bash
kubectl edit etcd <etcd-name> -n <namespace>
```

This will open up the linked editor where you can make the edits.

### Reconcile

There are two ways to control reconciliation of any changes done to `Etcd` custom resources.

#### Auto reconciliation

If `etcd-druid` has been deployed with auto-reconciliation then any change done to an `Etcd` resource will be automatically reconciled. 
Prior to v0.23.0 you can do this by using `--ignore-operation-annotation` CLI flag. This flag has been marked as deprecated and will be removed in later versions of `etcd-druid`. With etcd-druid version v0.23.x it is recommended that you use `--enable-etcd-spec-auto-reconcile` CLI flag to enable auto-reconcile.

> For a complete list of CLI args you can see [this](../deployment/configure-etcd-druid.md) document.

#### Explicit reconciliation
If `--enable-etcd-spec-auto-reconcile` or `--ignore-operation-annotation` is set to false or not set at all, then any change to an `Etcd` resource will not be automatically reconciled. To trigger a reconcile you must set the following annotation on the `Etcd` resource:
```bash
kubectl annotate etcd <etcd-name> gardener.cloud/operation=reconcile -n <namespace>
```

This option is sometimes recommeded as you would like avoid auto-reconciliation of accidental changes to `Etcd` resources outside the maintenance time window, thus preventing a potential transient quorum loss due to misconfiguration, attach-detach issues of persistent volumes etc.

## Overwrite Container OCI Images

To find out image versions of `etcd-backup-restore` and `etcd-wrapper` used by a specific version of `etcd-druid` one way is look for the image versions in [images.yaml](https://github.com/gardener/etcd-druid/blob/1007780853d5282e615db401db58e823de3c1944/internal/images/images.yaml). There are times that you might wish to override these images that come bundled with `etcd-druid`. There are two ways in which you can do that:

**Option #1**
We leverage [Overwrite ImageVector](https://github.com/gardener/gardener/blob/75c8f652fd31319608ac73257d587c3d31a9cc8f/docs/deployment/image_vector.md#overwriting-image-vector) facility provided by [gardener](https://github.com/gardener/gardener/). This capability can be used without bringing in gardener as well. To illustrate this in context of `etcd-druid` you will create a `ConfigMap` with the following content:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: etcd-druid-images-overwrite
  namespace: <etcd-druid-namespace>
data:
  images_overwrite.yaml: |
    images:
    - name: etcd-backup-restore
      sourceRepository: github.com/gardener/etcd-backup-restore
      repository: <your-own-custom-etcd-backup-restore-repo-url>
      tag: "v<custom-tag>"
    - name: etcd-wrapper
      sourceRepository: github.com/gardener/etcd-wrapper
      repository: <your-own-custom-etcd-wrapper-repo-url>
      tag: "v<custom-tag>"
    - name: alpine
      repository: <your-own-custom-alpine-repo-url>
      tag: "v<custom-tag>"
```

You can use [images.yaml](https://github.com/gardener/etcd-druid/blob/1007780853d5282e615db401db58e823de3c1944/internal/images/images.yaml) as a reference to create the overwrite images YAML `ConfigMap`.

Edit the `etcd-druid` `Deployment` with:

* Mount the `ConfigMap`
* Set `IMAGEVECTOR_OVERWRITE` environment variable whose value must be the path you choose to mount the `ConfigMap`.

To illustrate the changes you can see the following `etcd-druid` Deployment YAML:
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: etcd-druid
  namespace: <etcd-druid-namespace>
spec:
  template:
    spec:
      containers:
      - name: etcd-druid
        env:
        - name: IMAGEVECTOR_OVERWRITE
          value: /imagevector-overwrite/images_overwrite.yaml
        volumeMounts:
        - name: etcd-druid-images-overwrite
          mountPath: /imagevector-overwrite
      volumes:
      - name: etcd-druid-images-overwrite
        configMap:
          name: etcd-druid-images-overwrite
```

> **NOTE:** Image overwrites specified in the mounted `ConfigMap` will be respected by successive reconciliations for this `Etcd` custom resource.

**Option #2**

We provide a generic way to suspend etcd cluster reconciliation via etcd-druid, allowing a human operator to take control. This option should be excercised only in case of troubleshooting or quick fixes which are not possible to do via the reconciliation loop in etcd-druid. However one of the use cases to use this option is to perhaps update the container image to apply a hot patch and speed up recovery of an etcd cluster.

## Manually modify individual etcd cluster resources

`etcd` cluster resources are managed by `etcd-druid` and since v0.23.0 version of `etcd-druid` any changes to these managed resources are protected via a validating webhook. You can find more information about this webhook [here](../concepts/etcd-cluster-resource-protection.md). To be able to manually modify etcd cluster managed resources two things needs to be done:

1. Annotate the target `Etcd` resource suspending any reconciliation by `etcd-druid`. You can do this by invoking the following command:
   ```bash
   kubectl annotate etcd <etcd-name> -n <namespace> druid.gardener.cloud/suspend-etcd-spec-reconcile=
   ```

2. Add another annotation to the target `Etcd` resource disabling managed resource protection via the webhook. You can do this by invoking the following command:

   ```bash
   kubectl annotate etcd etcd-main -n <namespace> druid.gardener.cloud/disable-resource-protection=
   ```

Now you are free to make changes to any managed etcd cluster resource.

> **NOTE:** As long as the above two annotations are there, no reconciliation will be done for this etcd cluster by `etcd-druid`. Therefore it is essential that you remove this annotations eventually.ß
