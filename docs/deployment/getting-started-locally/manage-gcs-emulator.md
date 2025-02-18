# Manage GCS Emulator

This document is a step-by-step guide on how to configure, deploy and cleanup [GCS Emulator](https://github.com/fsouza/fake-gcs-server), within a [kind](https://kind.sigs.k8s.io/) cluster. GCS Emulator emulates Google Cloud Storage locally, which allows the `Etcd` cluster to interact with GCS. This setup is ideal for local development and testing.

## 00-Prerequisites

Ensure that you have setup the development environment as per the [documentation](../../development/prepare-dev-environment.md).

> **Note:** It is assumed that you have already created kind cluster and the `KUBECONFIG` is pointing to this Kubernetes cluster.

### Installing gsutil

To interact with `GCS Emulator` you must also install the [gsutil](https://cloud.google.com/storage/docs/gsutil) utility. Follow the instructions [here](https://cloud.google.com/storage/docs/gsutil_install) to install `gsutil`.

## 01-Deploy FakeGCS

Deploy FakeGCS onto the Kubernetes cluster using the command below:

```bash
make deploy-fakegcs
```

## 02-Setup GCS Bucket

To create a GCS bucket for `Etcd` backup purposes, execute the following command:

```bash
gsutil -o "Credentials:gs_json_host=127.0.0.1" -o "Credentials:gs_json_port=4443" -o "Boto:https_validate_certificates=False" mb "gs://etcd-bucket"
```

## 03-Configure Secret

Connection details for a GCS Object Store are put into a Kubernetes [Secret](https://kubernetes.io/docs/concepts/configuration/secret/). Apply the Kubernetes Secret manifest through:

```bash
kubectl apply -f examples/objstore-emulator/etcd-secret-fakegcs.yaml
```

> **Note:** The secret created should be referred to in the `Etcd` CR in `spec.backup.store.secretRef`.

## 04-Cleanup

To clean the setup, delete the FakeGCS deployment:

```bash
kubectl delete -f ./hack/e2e-test/infrastructure/fake-gcs-server/fake-gcs-server.yaml
```
