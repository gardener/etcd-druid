---
title: Local e2e Tests
---

# Go-native e2e Test Suite

The Etcd-Druid project now features a comprehensive, Go-native e2e test suite (see [PR #1060](https://github.com/gardener/etcd-druid/pull/1060)) that comprehensively covers various aspects of Etcd-Druid's management of Etcd clusters. The suite is designed for reliability, speed, and ease of use, and does not require skaffold or any provider-specific setup.

## Running e2e tests against an existing cluster and etcd-druid

The main entrypoint for running the e2e tests against an existing Kubernetes cluster which already has a running installation of etcd-druid, is as follows:

```sh
make \
  [RETAIN_TEST_ARTIFACTS=true|false \]
  [PROVIDERS="none,local" \]
  [GO_TEST_ARGS="..." \]
  test-e2e 
```

This command runs all e2e tests in the `test/e2e` package against the cluster and Etcd-Druid installation pointed to by your `KUBECONFIG`.

### Available Tests
- `TestBasic` tests creation, hibernation, unhibernation and deletion of Etcd clusters of varying replicas and TLS configurations.
- `TestScaleOut` tests scale out of an Etcd cluster from 1 to 3 replicas along with label changes.
- `TestTLSAndLabelUpdates` tests TLS changes along with label changes in an Etcd cluster.
- `TestRecovery` tests for recovery of an Etcd cluster upon simulated disasters.
- `TestClusterUpdate` tests for zero downtime during cluster updates.
- `TestSnapshotCompaction` tests snapshot compaction.
- `TestSecretFinalizers` tests addition and removal of finalizer on referred secrets in Etcd resources.

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
  - Set `RETAIN_TEST_ARTIFACTS=true` to retain all test resources for debugging.
- **Providers:**
  - Only `none` and `local` providers are supported. These can be tested either individually or together in the same test run by setting `PROVIDERS="none,local"` No cloud provider credentials or emulators are needed.
- **KUBECONFIG:**
  - Set the `KUBECONFIG` environment variable to point to the target cluster.
- **Test Output:**
  - All logs and test output are streamed to the console.

### Test Lifecycle
- Tests expect an existing Etcd-Druid installation in the cluster.
- Each test creates its own namespace and resources, and cleans them up by default. Set `RETAIN_TEST_ARTIFACTS=true` to retain all test resources for debugging.

## Running e2e tests in a local KinD cluster

For a fully automated local e2e test run, use:

```sh
make \
  [RETAIN_TEST_ARTIFACTS=true|false \]
  [RETAIN_KIND_CLUSTER=true|false \]
  [PROVIDERS="none,local" \]
  [GO_TEST_ARGS="..." \]
  ci-e2e-kind
```

This will:
1. Spin up a local KinD cluster, whose `KUBECONFIG` is made available at `hack/kind/kubeconfig`
2. Install Etcd-Druid into the cluster
3. Run the full e2e test suite (see [make test-e2e](#running-e2e-tests-against-an-existing-cluster-and-etcd-druid) above)
4. Cleanup resources, based on the `RETAIN_TEST_ARTIFACTS` flag.
5. Tear down the KinD cluster, based on the `RETAIN_KIND_CLUSTER` flag.

This is the recommended way to validate changes in a clean, reproducible environment.

### Environment Variables

- `RETAIN_TEST_ARTIFACTS`: Set to `true` to retain test namespaces and resources after tests finish (default: false).
- `RETAIN_KIND_CLUSTER`: Set to `true` to retain the KinD cluster after tests finish (default: false, only applicable for `make ci-e2e-kind`).
- `GO_TEST_ARGS`: Additional arguments to pass to `go test` (e.g. `-run`, `-v`, `-timeout`).

## e2e test with local storage emulators [AWS, GCP, AZURE]

The e2e test suite supports running tests with local storage emulators for AWS, GCP, and Azure. To use these emulators, set the `PROVIDERS` environment variable when running `make test-e2e`:

```sh
make \
  [RETAIN_TEST_ARTIFACTS=true|false \]
  [PROVIDERS="aws,gcp,azure" \]
  [GO_TEST_ARGS="..." \]
  test-e2e
```

> [!NOTE]
> ci-e2e-kind target does not currently support emulators, and can only be run against local provider and/or no provider.

### Setting up emulators

While etcd-backup-restore generally expects a storage bucket from a real infrastructure provider, there are tools such as [localstack](https://docs.localstack.cloud/user-guide/aws/s3/), [fake-gcs-server](https://github.com/fsouza/fake-gcs-server) and [azurite](https://github.com/Azure/Azurite) that enable developers to run e2e tests locally with emulators for AWS S3, Google GCS and Azure ABS respectively. These emulators can be run either as standalone services or as workload in the same KinD cluster used for e2e tests.

Port mappings for the services deployed for these emulators are defined in the [KinD cluster configuration file](../../hack/kind-up.sh), which allows developers to use the storage providers' respective CLIs to access and inspect the storage buckets created when deploying these emulators.

#### Localstack setup (AWS)

The [docker image](https://hub.docker.com/r/localstack/localstack/tags?name=s3) for [localstack](https://www.localstack.cloud/localstack-for-aws) is deployed as a pod inside the KIND cluster through `hack/e2e-test/infrastructure/localstack/localstack.yaml` via the following make target:

```sh
make deploy-localstack-kind
```

Once deployed, the localstack S3 endpoint can be accessed at `http://localstack-service.default.svc.cluster.local:4566` and may be used for running e2e tests.

#### Fake GCS Server setup (GCP)

The [docker image](https://hub.docker.com/r/fsouza/fake-gcs-server) for [fake-gcs-server](https://github.com/fsouza/fake-gcs-server) is deployed as a pod inside the KIND cluster through `hack/e2e-test/infrastructure/fake-gcs-server/fake-gcs-server.yaml` via the following make target:

```sh
make deploy-fake-gcs-server-kind
```

Once deployed, the fake-gcs-server endpoint can be accessed at `http://fake-gcs-server.default.svc.cluster.local:4443` and may be used for running e2e tests.

#### Azurite setup (Azure)

The [docker image](https://mcr.microsoft.com/en-us/artifact/mar/azure-storage/azurite/tags) for [azurite](https://github.com/Azure/Azurite) is deployed as a pod inside the KIND cluster through `hack/e2e-test/infrastructure/azurite/azurite.yaml` via the following make target:

```sh
make deploy-azurite-kind
```

Once deployed, the azurite blob service endpoint can be accessed at `http://azurite-service.default.svc.cluster.local:10000/devstoreaccount1` and may be used for running e2e tests.

## Cleaning up e2e test resources

To manually clean up any test namespaces or resources left behind (if `RETAIN_TEST_ARTIFACTS=true` was set), you can run:

```sh
make clean-e2e-test-resources
```

This will delete all namespaces created by the e2e tests, denoted by the `etcd-e2e-` prefix. Additionally, all PKI artifacts created for the tests will also be removed.

## Notes
- Provider-specific setup and configuration (AWS, Azure, GCP, etc.) is not mandatory for running e2e tests, but can be enabled and used as mentioned [here](#e2e-test-with-local-storage-emulators-aws-gcp-azure).
- All test orchestration is handled natively in Go.
- For more details, see the source code in `test/e2e` and the `Makefile` targets for e2e testing.
