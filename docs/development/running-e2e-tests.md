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

## Cleaning up e2e test resources

To manually clean up any test namespaces or resources left behind (if `RETAIN_TEST_ARTIFACTS=true` was set), you can run:

```sh
make clean-e2e-test-resources
```

This will delete all namespaces created by the e2e tests, denoted by the `etcd-e2e-` prefix.

## Notes
- No provider-specific configuration (AWS, Azure, GCP, etc.) is required or supported.
- No need to set up or emulate cloud buckets for backup/restore tests.
- All test orchestration is handled natively in Go.
- For more details, see the source code in `test/e2e` and the `Makefile` targets for e2e testing.
