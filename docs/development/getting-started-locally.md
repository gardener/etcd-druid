# Etcd-Druid Local Setup

This page aims to provide steps on how to setup Etcd-Druid locally with and without storage providers.

## Clone the etcd-druid github repo

```sh
# clone the repo
git clone https://github.com/gardener/etcd-druid.git
# cd into etcd-druid folder
cd etcd-druid
```

---

> **Note:**
>
>- Etcd-druid uses [kind](https://kind.sigs.k8s.io/) as it's local Kubernetes engine. The local setup is configured for kind due to it's convenience but any other kubernetes setup would also work.
>- To setup Etcd-Druid with backups enabled on a [LocalStack](https://github.com/localstack/localstack) provider, refer [this document](getting-started-locally-localstack.md)
>- In the section [Annotate Etcd CR with the reconcile annotation](#annotate-etcd-cr-with-the-reconcile-annotation), the flag `ignore-operation-annotation` is set to false, which means a special annotation is required  on the Etcd CR, for etcd-druid to reconcile it. To disable this behavior and allow auto-reconciliation of the Etcd CR for any change in the Etcd spec, set the `ignoreOperationAnnotation` flag to `true` in the `values.yaml` located at [`charts/druid/values.yaml`](../../charts/druid/values.yaml). Or if etcd-druid is being run as a process, then while starting the process, set the CLI flag `--ignore-operation-annotation=true` for it.

## Setting up the kind cluster

```sh
# Create a kind cluster
make kind-up
```

This creates a new kind cluster and stores the kubeconfig in the  `./hack/e2e-test/infrastructure/kind/kubeconfig` file.

To target this newly created cluster, set the `KUBECONFIG` environment variable to the kubeconfig file located at `./hack/e2e-test/infrastructure/kind/kubeconfig` by using the following

```sh
export KUBECONFIG=$PWD/hack/e2e-test/infrastructure/kind/kubeconfig
```

## Setting up etcd-druid

```sh
make deploy
```

This generates the `Etcd` CRD and deploys an etcd-druid pod into the cluster

### Prepare the Etcd CR

Etcd CR can be configured in 2 ways. Either to take backups to the store or disable them. Follow the appropriate section below based on the requirement.

The Etcd CR can be found at this location `$PWD/config/samples/druid_v1alpha1_etcd.yaml`

- **Without Backups enabled**

    To setup Etcd-druid without backups enabled, make sure the `spec.backup.store` section of the Etcd CR is commented out.

- **With Backups enabled (On Cloud Provider Object Stores)**

  - **Prepare the secret**

    Create a secret for cloud provider access. Find the secret yaml templates for different cloud providers [here](https://github.com/gardener/etcd-backup-restore/tree/master/example/storage-provider-secrets).

    Replace the dummy values with the actual configurations and make sure to add a name and a namespace to the secret as intended.

    > **Note 1:** The secret should be applied in the same namespace as druid.
    >
    > **Note 2:** All the values in the data field of secret yaml should be in base64 encoded format.

  - **Apply the secret**

    ```sh
    kubectl apply -f path/to/secret
    ```

  - **Adapt `Etcd` resource**

    Uncomment the `spec.backup.store` section of the druid yaml and set the keys to allow backuprestore to take backups by connecting to an object store.

    ```yaml
    # Configuration for storage provider
    store:
        secretRef:
            name: etcd-backup-secret-name
        container: object-storage-container-name
        provider: aws # options: aws,azure,gcp,openstack,alicloud,dell,openshift,local
        prefix: etcd-test
    ```

    Brief explanation of keys:

    - `secretRef.name` is the name of the secret that was applied as mentioned above
    - `store.container` is the object storage bucket name
    - `store.provider`  is the bucket provider. Pick from the options mentioned in comment
    - `store.prefix`    is the folder name that you want to use for your snapshots inside the bucket.

### Applying the Etcd CR

> **Note:** With backups enabled, make sure the bucket is created in corresponding cloud provider before applying the Etcd yaml

Create the Etcd CR (Custom Resource) by applying the Etcd yaml to the cluster

```sh
# Apply the prepared etcd CR yaml
kubectl apply -f config/samples/druid_v1alpha1_etcd.yaml
```

### Annotate Etcd CR with the reconcile annotation

> **Note :** If the `ignore-operation-annotation` flag is set to `true`, this step is not required

The above step creates an Etcd resource, however etcd-druid won't pick it up for reconciliation without an annotation. To get etcd-druid to reconcile the etcd CR, annotate it with the following `gardener.cloud/operation:reconcile`.

```sh
# Annotate etcd-test CR to reconcile
kubectl annotate etcd etcd-test gardener.cloud/operation="reconcile"
```

This starts creating the etcd cluster

### Verify the Etcd cluster

To obtain information regarding the newly instantiated etcd cluster, perform the following step, which gives details such as the cluster size, readiness status of its members, and various other attributes.

```sh
kubectl get etcd -o=wide
```

#### Verify Etcd Member Pods

To check the etcd member pods, do the following and look out for pods starting with the name `etcd-`

```sh
kubectl get pods
```

#### Verify Etcd Pod's Functionality

Verify the working conditions of the etcd pods by putting data through a etcd container and access the db from same/another container depending on single/multi node etcd cluster.

Ideally, you can exec into the etcd container using `kubectl exec -it <etcd_pod> -c etcd -- bash` if it utilizes a base image containing a shell. However, note that the `etcd-wrapper` Docker image employs a [distroless image](https://github.com/GoogleContainerTools/distroless), which lacks a shell. To interact with etcd, use an [Ephemeral container](https://kubernetes.io/docs/concepts/workloads/pods/ephemeral-containers/) as a debug container. Refer to this [documentation](https://github.com/gardener/etcd-wrapper/blob/main/docs/deployment/ops.md#build-image) for building and using an ephemeral container by attaching it to the etcd container.

```sh
# Put a key-value pair into the etcd 
etcdctl put key1 value1
# Retrieve all key-value pairs from the etcd db
etcdctl get --prefix ""
```

For a multi-node etcd cluster, insert the key-value pair from the `etcd` container of one etcd member and retrieve it from the `etcd` container of another member to verify consensus among the multiple etcd members.

#### View Etcd Database File

The Etcd database file is located at `var/etcd/data/new.etcd/snap/db` inside the `backup-restore` container. In versions with an `alpine` base image, you can exec directly into the container. However, in recent versions where the `backup-restore` docker image started using a distroless image, a debug container is required to communicate with it, as mentioned in the previous section.

## Cleaning the setup

```sh
# Delete the cluster
make kind-down
```

This cleans up the entire setup as the kind cluster gets deleted. It deletes the created Etcd, all pods that got created along the way and also other resources such as statefulsets, services, PV's, PVC's, etc.
