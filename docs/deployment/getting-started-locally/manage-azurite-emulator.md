# Manage Azure Blob Storage Emulator

This document is a step-by-step guide on how to configure, deploy and cleanup [Azurite](https://github.com/Azure/Azurite#introduction), the `Azure Blob Storage` emulator, within a [kind](https://kind.sigs.k8s.io/) cluster. This setup is ideal for local development and testing.

## 00-Prerequisites

Ensure that you have setup the development environment as per the [documentation](../../development/prepare-dev-environment.md).

> **Note:** It is assumed that you have already created kind cluster and the `KUBECONFIG` is pointing to this Kubernetes cluster.

### Installing Azure CLI

To interact with `Azurite` you must also install the Azure CLI `(version >=2.55.0)`
On macOS run:

```bash
brew install azure-cli
```

For other OS, please check the [Azure CLI installation documentation](https://learn.microsoft.com/en-us/cli/azure/install-azure-cli).

## 01-Deploy Azurite

```bash
make deploy-azurite
```

The above make target will deploy `Azure` emulator in the target Kubernetes cluster.

## 02-Setup ABS Container

We will be using the `azure-cli` to create an ABS container. Export the connection string to enable `azure-cli` to connect to `Azurite` emulator.
```bash
export AZURE_STORAGE_CONNECTION_STRING="DefaultEndpointsProtocol=http;AccountName=devstoreaccount1;AccountKey=Eby8vdM02xNOcqFlqUwJPLlmEtlCDXJ1OUzFT50uSRZ6IFsuFq2UVErCz4I6tq/K1SZFPTOtr/KBHBeksoGMGw==;BlobEndpoint=http://127.0.0.1:10000/devstoreaccount1;"
```

To create an Azure Blob Storage Container in Azurite, run the following command:
```bash
az storage container create -n <container-name>
```

## 03-Configure Secret

Connection details for an Azure Object Store Container are put into a Kubernetes [Secret](https://kubernetes.io/docs/concepts/configuration/secret/). Apply the Kubernetes Secret manifest through:
```bash
kubectl apply -f examples/objstore-emulator/etcd-secret-azurite.yaml
```

> **Note:** The secret created should be referred to in the `Etcd` CR in `spec.backup.store.secretRef`.

## 04-Cleanup

To clean the setup, unset the environment variable set in step-03 above and delete the Azurite deployment:

```bash
unset AZURE_STORAGE_CONNECTION_STRING
kubectl delete -f ./hack/e2e-test/infrastructure/azurite/azurite.yaml
```
