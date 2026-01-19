---
title: Local E2E Tests
---

# Go-Native E2E Test Suite

The Etcd-Druid project now features a comprehensive, Go-native e2e test suite (see [PR #1060](https://github.com/gardener/etcd-druid/pull/1060)) that comprehensively covers various aspects of Etcd-Druid's management of Etcd clusters. The suite is designed for reliability, speed, and ease of use, and does not require kustomize or any mandatory provider-specific setup.

## Available Test Suites

- `TestBasic` tests creation, hibernation, unhibernation and deletion of Etcd clusters of varying replicas and TLS configurations.
- `TestScaleOut` tests scale out of an Etcd cluster from 1 to 3 replicas along with label changes.
- `TestTLSAndLabelUpdates` tests TLS changes along with label changes in an Etcd cluster.
- `TestRecovery` tests for recovery of an Etcd cluster upon simulated disasters.
- `TestClusterUpdate` tests for zero downtime during cluster updates.
- `TestSnapshotCompaction` tests snapshot compaction.
- `TestSecretFinalizers` tests addition and removal of finalizer on referred secrets in Etcd resources.

## Running e2e tests in a local KinD cluster

For a fully automated local e2e test run, use:

```sh
make \
  [RETAIN_TEST_ARTIFACTS=all|failed \]
  [RETAIN_KIND_CLUSTER=true \]
  [PROVIDERS="none,local" \]
  [GO_TEST_ARGS="..." \]
  ci-e2e-kind
```

This will:

1. Spin up a local KinD cluster, whose `KUBECONFIG` is made available at `hack/kind/kubeconfig`.
2. Install Etcd-Druid into the cluster, configured for e2e testing.
3. Run the full e2e test suite (see [make test-e2e](#running-e2e-tests-against-an-existing-cluster) below).
4. Cleanup e2e test resources, based on the `RETAIN_TEST_ARTIFACTS` flag.
5. Tear down the KinD cluster, based on the `RETAIN_KIND_CLUSTER` flag.

This is the recommended way to validate changes in a clean, reproducible environment.

> [!NOTE]
> `make ci-e2e-kind` target only supports providers `local` and `none`. In order to run e2e tests against real or emulated storage providers, please use `make test-e2e` target against an existing cluster with etcd-druid installed as described at [running-e2e-tests-against-an-existing-cluster](#running-e2e-tests-against-an-existing-cluster).

### Environment Variables

See [configurable environment variables](#configurable-environment-variables) above for details on the available environment variables. `KUBECONFIG` is automatically set to point to the KinD cluster's kubeconfig, so this is not user-configurable for KinD-based tests.

## Running e2e tests against an existing cluster

If you wish to run the e2e tests against an existing Kubernetes cluster, you can do so by running the following commands:

```sh
export KUBECONFIG=/path/to/your/kubeconfig
DRUID_E2E_TEST=true make deploy

make \
  [RETAIN_TEST_ARTIFACTS=all|failed \]
  [PROVIDERS="none,local" \]
  [GO_TEST_ARGS="..." \]
  test-e2e
```

This installs etcd-druid configured for e2e tests into the cluster pointed to by your `KUBECONFIG`, and runs all e2e tests in the `test/e2e` package against it.

> [!NOTE]
> Please ensure that etcd-druid is installed with environment variable `DRUID_E2E_TEST=true`, so that it is configured to support the expectations of the e2e tests.
> Check the [etcd-druid deployment documentation](../deployment/getting-started-locally/getting-started-locally.md#02-setting-up-etcd-druid) for more details.

### User Controls & Test Knobs

- **Parallelism:** All test cases are run in parallel of 10 test units by default for speed and to simulate real-world concurrency. Each test is run its own dedicated namespace for easy debugging.
- **Test Selection:**
  - Use Go test flags via `GO_TEST_ARGS` to select or filter tests, e.g.:
    - `make GO_TEST_ARGS='-run TestBasic' test-e2e` to run only the `TestBasic` test.
    - `make GO_TEST_ARGS='-v' test-e2e` for verbose output.
    - `make GO_TEST_ARGS='-count=1' test-e2e` to disable cached test runs.
    - multiple go test flags can be combined as needed.
- **Timeouts:** Each test has a default timeout of 1 hour.
- **Resource Cleanup:**
  - By default, all test-created namespaces and resources are cleaned up after each test.
  - Set `RETAIN_TEST_ARTIFACTS=all` to retain all test resources for debugging, or `RETAIN_TEST_ARTIFACTS=failed` to retain test resources belonging only to the failed test cases.
- **Providers:**
  - `none`: No backup storage provider configured.
  - `local`: Local filesystem-based backup storage provider.
  - `aws`, `s3`: AWS S3 backup storage provider. Refer to the [AWS S3 section](#running-tests-against-backup-provider-aws-s3) for setup details.
  - `azure`, `abs`: Azure Blob Storage backup storage provider. Refer to the [Azure ABS section](#running-tests-against-backup-provider-azure-abs) for setup details.
  - `gcp`, `gcs`: Google Cloud Storage backup storage provider. Refer to the [GCP GCS section](#running-tests-against-backup-provider-gcp-gcs) for setup details.
  - These can be tested either individually or together in the same test run by setting `PROVIDERS="none,local,aws,az,gcp"` as comma-separated values.
- **KUBECONFIG:**
  - Set the `KUBECONFIG` environment variable to point to the target cluster.
- **Test Output:**
  - All logs and test output are streamed to the console.

#### Configurable environment variables

- `KUBECONFIG`: Path to the kubeconfig file for the target cluster (default: `$KUBECONFIG`).
- `RETAIN_TEST_ARTIFACTS`: Set to `all` to retain all test namespaces and resources after tests finish, or `failed` to retain only test namespaces and resources belonging to the failed test cases (default: unset, which will clean up all test artifacts).
- `RETAIN_KIND_CLUSTER`: Set to `true` to retain the KinD cluster after tests finish (default: false, only applicable for `make ci-e2e-kind`).
- `GO_TEST_ARGS`: Additional arguments to pass to `go test` (e.g. `-run`, `-v`, `-timeout`).
- `PROVIDERS`: Comma-separated list of storage providers to test against (default: `none`).

### Test Lifecycle

- Tests expect an existing Etcd-Druid installation in the cluster.
- Each test creates its own namespace and resources, and cleans them up by default. Set `RETAIN_TEST_ARTIFACTS=all` to retain all test resources for debugging, or `RETAIN_TEST_ARTIFACTS=failed` to retain only the test resources belonging to failed test cases.

## Running e2e tests against backup storage providers [AWS, AZURE, GCP]

The e2e test suite supports running tests with real storage providers as well as local storage emulators for AWS, Azure and GCP.

While etcd-backup-restore generally expects a storage bucket from a real infrastructure provider, there are tools such as [localstack](https://docs.localstack.cloud/user-guide/aws/s3/), [azurite](https://github.com/Azure/Azurite) and [fake-gcs-server](https://github.com/fsouza/fake-gcs-server) that enable developers to run e2e tests locally with emulators for AWS S3, Azure ABS and Google GCS respectively. These emulators can be run either as standalone services or as workload in the same KinD cluster used for e2e tests.

Port mappings for the services deployed for these emulators are defined in the [KinD cluster configuration file](../../hack/kind-up.sh), which allows developers to use the storage providers' respective CLIs to access and inspect the storage buckets created when deploying these emulators.

### Running tests against backup provider AWS S3

To run e2e tests against AWS S3, set the following environment variables when running `make test-e2e`:

```sh
export PROVIDERS="aws"
export AWS_ACCESS_KEY_ID=your-access-key-id
export AWS_SECRET_ACCESS_KEY=your-secret-access-key
export AWS_REGION=your-aws-region
```

#### Running tests against Localstack S3 emulator

Please refer to the [Localstack S3 emulator documentation](../deployment/getting-started-locally/manage-s3-emulator.md) for detailed instructions on deploying and configuring Localstack for S3 emulation.

Once Localstack is deployed and running, set the following environment variables when running `make test-e2e`:

```sh
export PROVIDERS="aws"
export AWS_ACCESS_KEY_ID=ACCESSKEYAWSUSER
export AWS_SECRET_ACCESS_KEY=sEcreTKey
export AWS_REGION=us-east-2
export LOCALSTACK_HOST="localhost:4566"
```

### Running tests against backup provider Azure ABS

To run e2e tests against Azure ABS, set the following environment variables when running `make test-e2e`:

```sh
export PROVIDERS="azure"
export AZ_STORAGE_ACCOUNT=your-storage-account
export AZ_STORAGE_KEY=your-storage-key
```

#### Running tests against Azurite emulator

Please refer to the [Azurite emulator documentation](../deployment/getting-started-locally/manage-azurite-emulator.md) for detailed instructions on deploying and configuring Azurite for ABS emulation.

Once Azurite is deployed and running, set the following environment variables when running `make test-e2e`:

```sh
export PROVIDERS="azure"
export AZ_STORAGE_ACCOUNT=your-storage-account
export AZ_STORAGE_KEY=your-storage-key
export AZURITE_DOMAIN="localhost:10000"
```

### Running tests against backup provider GCP GCS

To run e2e tests against GCP GCS, set the following environment variables when running `make test-e2e`:

```sh
export PROVIDERS="gcp"
export GCP_SERVICEACCOUNT_JSON_PATH=/path/to/your/service-account.json
```

#### Running tests against Fake GCS Server emulator

Please refer to the [fake-gcs-server documentation](../deployment/getting-started-locally/manage-gcs-emulator.md) for detailed instructions on deploying and configuring fake-gcs-server for GCS emulation.

Once fake-gcs-server is deployed and running, set the following environment variables when running `make test-e2e`:

```sh
export PROVIDERS="gcp"
export GCP_SERVICEACCOUNT_JSON_PATH=/path/to/your/service-account.json
export FAKEGCS_HOST="localhost:4443"
```

## Cleaning up e2e test resources

To manually clean up any test namespaces or resources left behind (if `RETAIN_TEST_ARTIFACTS` was set to `all`, or if set to `failed` and there were failed test cases), you can run:

```sh
make clean-e2e-test-resources
```

This will delete all namespaces created by the e2e tests, denoted by the `etcd-e2e-` prefix. Additionally, all PKI artifacts created for the tests will also be removed.

## Notes

- Provider-specific setup and configuration (AWS, Azure, GCP, etc.) is not mandatory for running e2e tests, but can be enabled and used as mentioned in [running-e2e-tests-against-backup-storage-providers-aws-azure-gcp](#running-e2e-tests-against-backup-storage-providers-aws-azure-gcp).
- All test orchestration is handled natively in Go.
- For more details, see the source code in `test/e2e` and the `Makefile` targets for e2e testing.
