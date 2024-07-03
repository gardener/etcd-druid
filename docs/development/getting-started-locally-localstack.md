# Getting Started with etcd-druid, LocalStack, and Kind

This guide provides step-by-step instructions on how to set up etcd-druid with [LocalStack](https://localstack.cloud/) and Kind on your local machine. LocalStack emulates AWS services locally, which allows the etcd cluster to interact with AWS S3 without the need for an actual AWS connection. This setup is ideal for local development and testing.

## Prerequisites

- Docker (installed and running)
- AWS CLI (version `>=1.29.0` or `>=2.13.0`)

## Environment Setup

### Step 1: Provision the Kind Cluster

Execute the command below to provision a `kind` cluster. This command also forwards port `4566` from the [kind cluster](../../hack/e2e-test/infrastructure/kind/cluster.yaml) to your local machine, enabling LocalStack access:

```bash
make kind-up
```

### Step 2: Deploy LocalStack

Deploy LocalStack onto the Kubernetes cluster using the command below:

```bash
make deploy-localstack
```

### Step 3: Set up an S3 Bucket

1. Set up the AWS CLI to interact with LocalStack by setting the necessary environment variables. This configuration redirects S3 commands to the LocalStack endpoint and provides the required credentials for authentication:

```bash
export AWS_ENDPOINT_URL_S3="http://localhost:4566"
export AWS_ACCESS_KEY_ID=ACCESSKEYAWSUSER
export AWS_SECRET_ACCESS_KEY=sEcreTKey
export AWS_DEFAULT_REGION=us-east-2
```

2. Create an S3 bucket for etcd-druid backup purposes:

```bash
aws s3api create-bucket --bucket etcd-bucket --region us-east-2 --create-bucket-configuration LocationConstraint=us-east-2 --acl private
```

### Step 4: Deploy etcd-druid

Deploy etcd-druid onto the Kind cluster using the command below:

```bash
make deploy
```

### Step 5: Configure etcd with LocalStack Store

Apply the required Kubernetes manifests to create an etcd custom resource (CR) and a secret for AWS credentials, facilitating LocalStack access:

```bash
export KUBECONFIG=hack/e2e-test/infrastructure/kind/kubeconfig
kubectl apply -f config/samples/druid_v1alpha1_etcd_localstack.yaml -f config/samples/etcd-secret-localstack.yaml
```

### Step 6: Reconcile the etcd

Initiate etcd reconciliation by annotating the etcd resource with the `gardener.cloud/operation=reconcile` annotation:

```bash
kubectl annotate etcd etcd-test gardener.cloud/operation=reconcile
```

Congratulations! You have successfully configured `etcd-druid`, `LocalStack`, and `kind` on your local machine. Inspect the etcd-druid logs and LocalStack to ensure the setup operates as anticipated.

To validate the buckets, execute the following command:

```bash
aws s3 ls etcd-bucket/etcd-test/v2/
```

### Cleanup

To dismantle the setup, execute the following command:

```bash
make kind-down
unset AWS_ENDPOINT_URL_S3 AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY AWS_DEFAULT_REGION KUBECONFIG

```