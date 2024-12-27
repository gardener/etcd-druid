---
title: Local e2e Tests
---

# e2e Test Suite

Developers can run extended e2e tests, in addition to unit tests, for Etcd-Druid in or from 
their local environments. This is recommended to verify the desired behavior of several features
and to avoid regressions in future releases.

The very same tests typically run as part of the component's release job as well as on demand, e.g.,
when triggered by Etcd-Druid maintainers for open pull requests.

Testing Etcd-Druid automatically involves a certain test coverage for [gardener/etcd-backup-restore](https://github.com/gardener/etcd-backup-restore/)
which is deployed as a side-car to the actual `etcd` container.

## Prerequisites

The e2e test lifecycle is managed with the help of [skaffold](https://skaffold.dev/). Every involved step like `setup`,
`deploy`, `undeploy` or `cleanup` is executed against a **Kubernetes** cluster which makes it a mandatory prerequisite at the same time.
Only [skaffold](https://skaffold.dev/) itself with involved `docker`, `helm` and `kubectl` executions as well as 
the e2e-tests are executed locally. Required binaries are automatically downloaded if you use the corresponding `make` target,
as described in this document.

It's expected that especially the `deploy` step is run against a Kubernetes cluster which doesn't contain an Druid deployment or any left-overs like `druid.gardener.cloud` CRDs.
The `deploy` step will likely fail in such scenarios.

> Tip: Create a fresh [KinD](https://kind.sigs.k8s.io/) cluster or a similar one with a small footprint before executing the tests.

## Providers

The following providers are supported for e2e tests:

- AWS
- Azure
- GCP
- Local

> Valid credentials need to be provided when tests are executed with mentioned cloud providers.

## Flow

An e2e test execution involves the following steps:

| Step       | Description                                                                                                                                                                                               |
| ---------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `setup`    | Create a storage bucket which is used for etcd backups (only with cloud providers).                                                                                                                       |
| `deploy`   | Build Docker image, upload it to registry (if remote cluster - see [Docker build](https://skaffold.dev/docs/pipeline-stages/builders/docker/)), deploy Helm chart (`charts/druid`) to Kubernetes cluster. |
| `test`     | Execute e2e tests as defined in `test/e2e`.                                                                                                                                                               |
| `undeploy` | Remove the deployed artifacts from Kubernetes cluster.                                                                                                                                                    |
| `cleanup`  | Delete storage bucket and Druid deployment from test cluster.                                                                                                                                             |

### Make target

Executing e2e-tests is as easy as executing the following command **with defined Env-Vars as desribed in the following
section and as needed for your test scenario**.

```shell
make test-e2e
```

### Common Env Variables

The following environment variables influence how the flow described above is executed:

- `PROVIDERS`:  Providers used for testing (`all`, `aws`, `azure`, `gcp`, `local`). Multiple entries must be comma separated. 
    > **Note**: Some tests will use very first entry from env `PROVIDERS` for e2e testing (ex: multi-node tests). So for multi-node tests to use specific provider, specify that provider as first entry in env `PROVIDERS`.
- `KUBECONFIG`: Kubeconfig pointing to cluster where Etcd-Druid will be deployed (preferably [KinD](https://kind.sigs.k8s.io)).
- `TEST_ID`:    Some ID which is used to create assets for and during testing.
- `STEPS`:      Steps executed by `make` target (`setup`, `deploy`, `test`, `undeploy`, `cleanup` - default: all steps).

### AWS Env Variables

- `AWS_ACCESS_KEY_ID`:       Key ID of the user.
- `AWS_SECRET_ACCESS_KEY`:   Access key of the user.
- `AWS_REGION`:              Region in which the test bucket is created.

Example:

```bash
make \
  AWS_ACCESS_KEY_ID="abc" \
  AWS_SECRET_ACCESS_KEY="xyz" \
  AWS_REGION="eu-central-1" \
  KUBECONFIG="$HOME/.kube/config" \
  PROVIDERS="aws" \
  TEST_ID="some-test-id" \
  STEPS="setup,deploy,test,undeploy,cleanup" \
test-e2e
```

### Azure Env Variables

- `STORAGE_ACCOUNT`:     Storage account used for managing the storage container.
- `STORAGE_KEY`:         Key of storage account.

Example:

```bash
make \
  STORAGE_ACCOUNT="abc" \
  STORAGE_KEY="eHl6Cg==" \
  KUBECONFIG="$HOME/.kube/config" \
  PROVIDERS="azure" \
  TEST_ID="some-test-id" \
  STEPS="setup,deploy,test,undeploy,cleanup" \
test-e2e
```

### GCP Env Variables

- `GCP_SERVICEACCOUNT_JSON_PATH`:      Path to the service account json file used for this test.
- `GCP_PROJECT_ID`:                    ID of the GCP project.

Example:

```bash
make \
  GCP_SERVICEACCOUNT_JSON_PATH="/var/lib/secrets/serviceaccount.json" \
  GCP_PROJECT_ID="xyz-project" \
  KUBECONFIG="$HOME/.kube/config" \
  PROVIDERS="gcp" \
  TEST_ID="some-test-id" \
  STEPS="setup,deploy,test,undeploy,cleanup" \
test-e2e
```

### Local Env Variables

No special environment variables are required for running e2e tests with `Local` provider.

Example:

```bash
make \
  KUBECONFIG="$HOME/.kube/config" \
  PROVIDERS="local" \
  TEST_ID="some-test-id" \
  STEPS="setup,deploy,test,undeploy,cleanup" \
test-e2e
```

## e2e test with local storage emulators [AWS, GCP, AZURE]

The above-mentioned e2e tests need storage from real cloud providers to be setup. But there are tools such as [localstack](https://docs.localstack.cloud/user-guide/aws/s3/), [fake-gcs-server](https://github.com/fsouza/fake-gcs-server) and [azurite](https://github.com/Azure/Azurite) that enables to run e2e test with mock AWS storage, mock GCS storage and mock AZURE storage respectively. We can provision a KIND cluster for end-to-end (e2e) tests. By using local emulators alongside the KIND cluster, we eliminate the need for any actual cloud provider infrastructure to be set up for running e2e tests.

### How are the KIND cluster and Emulators set up

KIND or Kubernetes-In-Docker is a kubernetes cluster that is set up inside a docker container. This cluster is with limited capability as it does not have much compute power. But this cluster can easily be setup inside a container and can be tear down easily just by removing a container. That's why KIND cluster is very easy to use for e2e tests. `Makefile` command helps to spin up a KIND cluster and use the cluster to run e2e tests.

#### Localstack setup

There is a docker image for localstack. The image is deployed as pod inside the KIND cluster through `hack/e2e-test/infrastructure/localstack/localstack.yaml`. `Makefile` takes care of deploying the yaml file in a KIND cluster.

The developer needs to run `make ci-e2e-kind` command. This command in turn runs `hack/ci-e2e-kind.sh` which spin up the KIND cluster and deploy localstack in it and then run the e2e tests using localstack as mock AWS storage provider. e2e tests are actually run on host machine but deploy the druid controller inside KIND cluster. Druid controller spawns multinode etcd clusters inside KIND cluster. e2e tests verify whether the druid controller performs its jobs correctly or not. Mock localstack storage is cleaned up after every e2e tests. That's why the e2e tests need to access the localstack pod running inside KIND cluster. The network traffic between host machine and localstack pod is resolved via mapping localstack pod port to host port while setting up the KIND cluster via `hack/e2e-test/infrastructure/kind/cluster.yaml`

##### How to execute e2e tests with localstack and KIND cluster

Run the following `make` command to spin up a KinD cluster, deploy localstack and run the e2e tests with provider `aws`:

```bash
make ci-e2e-kind
```

#### Fake-GCS-Server setup

[Fake-gcs-server](https://github.com/fsouza/fake-gcs-server) is run inside a pod using this [docker image](https://hub.docker.com/r/fsouza/fake-gcs-server) in a KIND cluster.

The user needs to run `make ci-e2e-kind-gcs` to start the e2e tests for druid with GCS emulator as the object storage for etcd backups. The above command internally runs the script `hack/ci-e2e-kind-gcs.sh` which initializes the setup with required steps before going on to create a KIND cluster and deploy fakegcs in it and use that emulator to run e2e tests.

The `fake-gcs-server` running inside the pod serves HTTP requests at port-8000 and HTTPS requests at port-4443. As the e2e tests runs on the host machine while the emulator runs on KIND, both ports i.e 8000 & 4443 needs to be port-forwarded from the host machine to fake-gcs service running inside the KIND cluster. The port forwardings is defined in the `hack/kind-up.sh` file which is used to setup the KIND cluster.

##### How to execute e2e tests with fake-gcs-server and KIND cluster

Run the following `make` command to spin up a KinD cluster, deploy fakegcs and run the e2e tests with provider `gcp`:

```bash
make ci-e2e-kind-gcs
```

#### Azurite setup

[Azurite](https://github.com/Azure/Azurite) is run inside a pod using this [docker image](mcr.microsoft.com/azure-storage/azurite:latest) in a KIND cluster.

The user needs to run `make ci-e2e-kind-azure` to start the e2e tests for druid with Azurite as the object storage for etcd backups. The above command internally runs the script `hack/ci-e2e-kind-azure.sh` which initializes the setup with required steps before going on to create a KIND cluster and deploy Azurite in it and use that emulator to run e2e tests.

The `azurite` running inside the pod serves HTTP requests at port 10000. As the e2e tests runs on the host machine while the emulator runs on KIND cluster, the port 10000 needs to be port-forwarded from the host machine to azurite service running inside the KIND cluster. The port forwardings is defined in the `hack/kind-up.sh` file which is used to setup the KIND cluster.

##### How to execute e2e tests with Azurite and KIND cluster

Run the following `make` command to spin up a KinD cluster, deploy Azurite and run the e2e tests with provider `azure`:

```bash
make ci-e2e-kind-azure
```
