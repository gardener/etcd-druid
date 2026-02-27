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

> [!IMPORTANT]
> **For e2e tests with emulators:**
> - The e2e tests expect a bucket/container named **`default.bkp`** to exist in the emulator before running tests.
> - The backup secret is **automatically generated** by the test framework with the name `etcd-backup`. You do **not** need to manually create or apply the secret.
> - When following the emulator setup guides linked below, complete the setup **only up to and including the bucket/container creation step**. Skip the "Configure Secret" section as it is not needed for e2e tests.

### Running tests against backup provider AWS S3

To run e2e tests against AWS S3, set the following environment variables when running `make test-e2e`:

```sh
export PROVIDERS="aws"
export AWS_ACCESS_KEY_ID=your-access-key-id
export AWS_SECRET_ACCESS_KEY=your-secret-access-key
export AWS_REGION=your-aws-region
```

#### Running tests against Localstack S3 emulator

Please refer to the [Localstack S3 emulator documentation](../deployment/getting-started-locally/manage-s3-emulator.md) for detailed instructions on deploying Localstack.

**Setup steps for e2e tests:**

1. Deploy Localstack: `make deploy-localstack`
2. Create the test bucket `default.bkp`:
   ```bash
   export AWS_ENDPOINT_URL_S3="http://localhost:4566"
   export AWS_ACCESS_KEY_ID=ACCESSKEYAWSUSER
   export AWS_SECRET_ACCESS_KEY=sEcreTKey
   export AWS_DEFAULT_REGION=us-east-2
   aws s3api create-bucket --bucket default.bkp --region us-east-2 --create-bucket-configuration LocationConstraint=us-east-2 --acl private
   ```

Once Localstack is deployed and the bucket is created, set the following environment variables when running `make test-e2e`:

```sh
export PROVIDERS="aws"
export AWS_ACCESS_KEY_ID=ACCESSKEYAWSUSER
export AWS_SECRET_ACCESS_KEY=sEcreTKey
export AWS_REGION=us-east-2
export LOCALSTACK_HOST="localstack.default:4566"
```

> [!NOTE]
> The `LOCALSTACK_HOST` must be set to the in-cluster service address `localstack.default:4566` because the etcd-backup-restore sidecar running inside the cluster needs to reach the Localstack service.

### Running tests against backup provider Azure ABS

To run e2e tests against Azure ABS, set the following environment variables when running `make test-e2e`:

```sh
export PROVIDERS="azure"
export AZ_STORAGE_ACCOUNT_NAME=your-storage-account
export AZ_STORAGE_ACCOUNT_KEY=your-storage-key
```

#### Running tests against Azurite emulator

Please refer to the [Azurite emulator documentation](../deployment/getting-started-locally/manage-azurite-emulator.md) for detailed instructions on deploying Azurite.

**Setup steps for e2e tests:**

1. Deploy Azurite: `make deploy-azurite`
2. Create the test container `default.bkp`:

   ```bash
   export AZURE_STORAGE_CONNECTION_STRING="DefaultEndpointsProtocol=http;AccountName=devstoreaccount1;AccountKey=Eby8vdM02xNOcqFlqUwJPLlmEtlCDXJ1OUzFT50uSRZ6IFsuFq2UVErCz4I6tq/K1SZFPTOtr/KBHBeksoGMGw==;BlobEndpoint=http://127.0.0.1:10000/devstoreaccount1;"
   az storage container create -n default.bkp
   ```

Once Azurite is deployed and the container is created, set the following environment variables when running `make test-e2e`:

```sh
export PROVIDERS="azure"
export AZ_STORAGE_ACCOUNT_NAME=devstoreaccount1
export AZ_STORAGE_ACCOUNT_KEY="Eby8vdM02xNOcqFlqUwJPLlmEtlCDXJ1OUzFT50uSRZ6IFsuFq2UVErCz4I6tq/K1SZFPTOtr/KBHBeksoGMGw=="
export AZURITE_DOMAIN="azurite-service.default:10000"
```

> [!NOTE]
> The `AZURITE_DOMAIN` must be set to the in-cluster service address `azurite-service.default:10000` because the etcd-backup-restore sidecar running inside the cluster needs to reach the Azurite service.

### Running tests against backup provider GCP GCS

To run e2e tests against GCP GCS, set the following environment variables when running `make test-e2e`:

```sh
export PROVIDERS="gcp"
export GCP_SERVICEACCOUNT_JSON_PATH=/path/to/your/service-account.json
```

#### Running tests against Fake GCS Server emulator

Please refer to the [fake-gcs-server documentation](../deployment/getting-started-locally/manage-gcs-emulator.md) for detailed instructions on deploying fake-gcs-server.

**Setup steps for e2e tests:**

1. Deploy FakeGCS: `make deploy-fakegcs`
2. Create the test bucket `default.bkp`:
   
   ```bash
   gsutil -o "Credentials:gs_json_host=127.0.0.1" -o "Credentials:gs_json_port=4443" -o "Boto:https_validate_certificates=False" mb "gs://default.bkp"
   ```

3. Create a dummy service account JSON file (FakeGCS doesn't validate credentials):
   
   ```bash
   cat > /tmp/fake-gcs-sa.json << 'EOF'
   {
     "type": "service_account",
     "project_id": "test-project"
   }
   EOF
   ```

Once fake-gcs-server is deployed and the bucket is created, set the following environment variables when running `make test-e2e`:

```sh
export PROVIDERS="gcp"
export GCP_SERVICEACCOUNT_JSON_PATH=/tmp/fake-gcs-sa.json
export FAKEGCS_HOST="fake-gcs-service.default:8000"
```

> [!NOTE]
> The `FAKEGCS_HOST` must be set to the in-cluster service address `fake-gcs-service.default:8000` because the etcd-backup-restore sidecar running inside the cluster needs to reach the FakeGCS service. Port 8000 is used for HTTP access.

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
