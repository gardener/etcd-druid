# Etcd-Druid Local Setup

This page aims to provide steps on how to setup Etcd-Druid locally with and without storage providers

## Setting up etcd-druid

### Clone the Etcd-Druid Github repo
    
```sh
# clone the repo
git clone https://github.com/gardener/etcd-druid.git
# cd into etcd-druid folder
cd etcd-druid
```

---

>**Note**: To setup Etcd-Druid with backups enabled on a [LocalStack](https://github.com/localstack/localstack) provider, refer [this document](https://github.com/gardener/etcd-druid/blob/master/docs/development/getting-started-locally-localstack.md)

---

### Setting up the Kind Cluster

```sh
# Create a kind cluster
make kind-up
```

This creates a new kind cluster and stores the kubeconfig in the  `./hack/e2e-test/infrastructure/kind/kubeconfig` file.

To target this newly created cluster, set the `KUBECONFIG` environment variable to the kubeconfig file located at `./hack/e2e-test/infrastructure/kind/kubeconfig` by using the following 
```sh
export KUBECONFIG=$PWD/hack/e2e-test/infrastructure/kind/kubeconfig
```

### Setting up Etcd-Druid

Generates the Etcd CRD and deploy an etcd-druid pod into the cluster
```sh
make deploy
```

### Applying the etcd CR

- **Without Backups enabled**

    To setup Etcd-druid without backups enabled, apply the Etcd yaml to the cluster 

    Confirm that the `spec.backup.store` is commented out.
    ```sh
    # Apply the etcd CR yaml
    kubectl apply -f config/samples/druid_v1alpha1_etcd.yaml
    ``` 

- **With Backups enabled (On Cloud Provider Object Stores)**

    Create a secret for cloud provider access. Find the secret yaml templates for different cloud providers [here](https://github.com/gardener/etcd-backup-restore/tree/master/example/storage-provider-secrets). 

    Replace the dummy values with the actual configurations and make sure to add a name and a namespace to the secret as intended. 

    >Note 1): The secret should be applied in the same namespace as druid.
    >
    >Note 2): All the values in the data field of secret yaml should be in base64 encoded format.

    <b>Apply the secret</b> 
    ```sh
    kubectl apply -f path/to/secret
    ``````

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

    * `secretRef.name` is the name of the secret that was applied as mentioned above
    * `store.container` is the object storage bucket name
    * `store.provider`  is the bucket provider. Pick from the options mentioned in comment
    * `store.prefix`    is the folder name that you want to use for your snapshots.

    > Note: Before applying the Etcd yaml, make sure the bucket is created in the corresponding cloud provider


    Create the Etcd CR (Custom Resource) by applying the Etcd yaml to the cluster 
    ```sh
    # Apply the etcd CR yaml
    kubectl apply -f config/samples/druid_v1alpha1_etcd.yaml
    ```

### Annotate the etcd CR 

The above step creates an Etcd resource, however etcd-druid won't pick it up for reconciliation without an annotation. To get etcd-druid to reconcile the etcd CR, annotate it with the following `gardener.cloud/operation:reconcile`.

```sh
# Annotate etcd-test CR to reconcile
kubectl annotate etcd etcd-test gardener.cloud/operation="reconcile"
```
This starts creating the etcd cluster

## Cleaning the setup

```sh
# Delete the cluster
make kind-down
```

This cleans up the cluster i.e deletes the created Etcd and all pods that got created along the way and also other resources such as statefulsets, services, PV's, PVC's, etc.