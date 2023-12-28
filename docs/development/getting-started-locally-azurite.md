# Getting started with `etcd-druid` using `Azurite`, and `kind`

This document is a step-by-step guide to run `etcd-druid` with [`Azurite`](https://github.com/Azure/Azurite#introduction), the `Azure Blob Storage` emulator, within a [`kind`](https://kind.sigs.k8s.io/) cluster. This setup is ideal for local development and testing.

## Prerequisites

- [`Docker`](https://www.docker.com/products/docker-desktop/) with the daemon running, or Docker Desktop running.
- [`Azure CLI`](https://learn.microsoft.com/en-us/cli/azure/install-azure-cli) (`>=2.55.0`)

## Environment setup

### Step 1: Provisioning the `kind` cluster

Execute the command below to provision a `kind` cluster. This command also forwards port `10000` from the [`kind`](../../hack/e2e-test/infrastructure/kind/cluster.yaml) cluster to your local machine, enabling `Azurite` access:

``` bash
make kind-up
```

Export the `KUBECONFIG` file after running the above command.

### Step 2: Deploy `Azurite`

To start up the `Azurite` emulator in a pod in the `kind` cluster, run:

``` bash
make deploy-azurite
```

### Step 3: Set up a `ABS Container`

1. To use the `Azure CLI` with the `Azurite` emulator running as a pod in the `kind` cluster, export the connection string for the `Azure CLI`.

 ``` bash
 export AZURE_STORAGE_CONNECTION_STRING="DefaultEndpointsProtocol=http;AccountName=devstoreaccount1;AccountKey=Eby8vdM02xNOcqFlqUwJPLlmEtlCDXJ1OUzFT50uSRZ6IFsuFq2UVErCz4I6tq/K1SZFPTOtr/KBHBeksoGMGw==;BlobEndpoint=http://127.0.0.1:10000/devstoreaccount1;"
 ```

1. Create a `Azure Blob Storage Container` in `Azurite`

``` bash
az storage container create -n etcd-bucket
```

### Step 4: Deploy `etcd-druid`

``` bash
make deploy
```

### Step 5: Configure the `Secret` and the `Etcd` manifests

1. Apply the Kubernetes `Secret` manifest through:

``` bash
kubectl apply -f config/samples/etcd-secret-azurite.yaml
```

1. Apply the `Etcd` manifest through:

``` bash
kubectl apply -f config/samples/druid_v1alpha1_etcd_azurite.yaml
```

### Step 6 : Make use of the Azurite emulator however you wish

`etcd-backup-restore` will now use `Azurite` running in `kind` as the remote store to store snapshots if all the previous steps were followed correctly.

### Cleanup

```bash
make kind-down
unset AZURE_STORAGE_CONNECTION_STRING KUBECONFIG
```
