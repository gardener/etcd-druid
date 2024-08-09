# Setting up etcd-druid locally with FakeGCS and Kind

This guide provides step-by-step instructions on how to set up etcd-druid with [fake-gcs](https://github.com/fsouza/fake-gcs-server) and Kind on your local machine. FakeGCS emulates the GCS services locally, which allows the etcd cluster to interact with GCS without the need for an actual GCP connection. This setup is ideal for local development and testing.

## Prerequisites

- Docker (installed and running)
- [gsutil](https://cloud.google.com/storage/docs/gsutil)

## Environment Setup

### Step 1: Provision the Kind Cluster

Execute the command below to provision a `kind` cluster. This command also forwards port `8000`(HTTP) and `4443`(HTTPS) from the [kind cluster](../../hack/e2e-test/infrastructure/kind/cluster.yaml) to your local machine, enabling access to fake-gcs running inside a pod in the KIND cluster:

```bash
make kind-up
```

### Step 2: Deploy FakeGCS

Deploy FakeGCS onto the Kubernetes cluster using the command below:

```bash
make deploy-fakegcs
```

### Step 3: Create a GCS bucket for etcd-druid backup purposes

```bash
gsutil -o "Credentials:gs_json_host=127.0.0.1" -o "Credentials:gs_json_port=4443" -o "Boto:https_validate_certificates=False" mb "gs://etcd-bucket"
```

### Step 4: Deploy etcd-druid

Deploy etcd-druid onto the Kind cluster using the command below:

```bash
make deploy
```

### Step 5: Configure etcd with FakeGCS object store

Apply the required Kubernetes manifests to create an etcd custom resource (CR) and a secret for GCP credentials, facilitating FakeGCS access:

```bash
export KUBECONFIG=hack/e2e-test/infrastructure/kind/kubeconfig
kubectl apply -f config/samples/etcd-secret-fakegcs.yaml -f config/samples/druid_v1alpha1_etcd_fakegcs.yaml
```

To validate the buckets, execute the following command:

```bash
gsutil -o "Credentials:gs_json_host=127.0.0.1" -o "Credentials:gs_json_port=4443" -o "Boto:https_validate_certificates=False" ls "gs://etcd-bucket"
```

### Cleanup

To clean the setup, execute the following commands:

```bash
make kind-down
```
