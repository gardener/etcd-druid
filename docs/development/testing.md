# Testing Strategy and Developer Guideline

Intent of this document is to introduce you (the developer) to the following:

- Libraries that are used to write tests.
- Best practices to write tests that are correct, stable, fast and maintainable.
- How to run tests.

The guidelines are not meant to be absolute rules. Always apply common sense and adapt the guideline if it doesn't make much sense for some cases. If in doubt, don't hesitate to ask questions during a PR review (as an author, but also as a reviewer). Add new learnings as soon as we make them!

For any new contributions **tests are a strict requirement**. `Boy Scouts Rule` is followed: If you touch a code for which either no tests exist or coverage is insufficient then it is expected that you will add relevant tests.

## Common guidelines for writing tests

* We use the `Testing` package provided by the standard library in golang for writing all our tests. Refer to its [official documentation](https://pkg.go.dev/testing) to learn how to write tests using `Testing` package. You can also refer to [this](https://go.dev/doc/tutorial/add-a-test) example.

* We use gomega as our matcher or assertion library. Refer to Gomega's [official documentation](https://onsi.github.io/gomega/) for details regarding its installation and application in tests.

* For naming the individual test/helper functions, ensure that the name describes what the function tests/helps-with. Naming is important for code readability even when writing tests - [example-testcase-naming](https://github.com/gardener/etcd-druid/blob/90995898b231a49a8f211e85160600e9e6019fe0/internal/webhook/etcdcomponents/handler_test.go#L157).

* Introduce helper functions for assertions to make test more readable where applicable - [example-assertion-function](https://github.com/gardener/etcd-druid/blob/4e9971aba3c3880a4cb6583d05843eabb8ca1409/test/it/controller/etcd/assertions.go#L117).

* Introduce custom matchers to make tests more readable where applicable - [example-custom-matcher](https://github.com/gardener/etcd-druid/blob/4e9971aba3c3880a4cb6583d05843eabb8ca1409/test/utils/matcher.go#L89).

* Do not use `time.Sleep` and friends as it renders the tests flaky.

* If a function returns a specific error then ensure that the test correctly asserts the expected error instead of just asserting that an error occurred. To help make this assertion consider using [DruidError](https://github.com/gardener/etcd-druid/blob/4e9971aba3c3880a4cb6583d05843eabb8ca1409/internal/errors/errors.go#L24) where possible. [example-test-utility](https://github.com/gardener/etcd-druid/blob/4e9971aba3c3880a4cb6583d05843eabb8ca1409/test/utils/errors.go#L23) & [usage](https://github.com/gardener/etcd-druid/blob/4e9971aba3c3880a4cb6583d05843eabb8ca1409/internal/component/clientservice/clientservice_test.go#L74).

* Creating sample data for tests can be a high effort. Consider writing test utilities to generate sample data instead. [example-test-object-builder](https://github.com/gardener/etcd-druid/blob/4e9971aba3c3880a4cb6583d05843eabb8ca1409/test/utils/etcd.go#L61).

* If tests require any arbitrary sample data then ensure that you create a `testdata` directory within the package and keep the sample data as files in it. From https://pkg.go.dev/cmd/go/internal/test

  > The go tool will ignore a directory named "testdata", making it available to hold ancillary data needed by the tests.

* Avoid defining shared variable/state across tests. This can lead to race conditions causing non-deterministic state. Additionally it limits the capability to run tests concurrently via `t.Parallel()`.

* Do not assume or try and establish an order amongst different tests. This leads to brittle tests as the codebase evolves.

* If you need to have logs produced by test runs (especially helpful in failing tests), then consider using [t.Log](https://pkg.go.dev/testing#T.Log) or [t.Logf](https://pkg.go.dev/testing#T.Logf).

## Unit Tests

* If you need a kubernetes `client.Client`, prefer using [fake client](https://github.com/gardener/etcd-druid/blob/master/test/utils/client.go#L67) instead of mocking the client. You can inject errors when building the client which enables you test error handling code paths.
    * Mocks decrease maintainability because they expect the tested component to follow a certain way to reach the desired goal (e.g., call specific functions with particular arguments).
* All unit tests should be run quickly. Do not use [envtest](https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/envtest) and do not set up a [Kind](https://kind.sigs.k8s.io/) cluster in unit tests.
* If you have common setup for variations of a function, consider using [table-driven](https://go.dev/wiki/TableDrivenTests) tests. See [this](https://github.com/gardener/etcd-druid/blob/4e9971aba3c3880a4cb6583d05843eabb8ca1409/internal/component/rolebinding/rolebinding_test.go#L27) as an example.
* An individual test should only test one and only one thing. Do not try and test multiple variants in a single test. Either use [table-driven](https://go.dev/wiki/TableDrivenTests) tests or write individual tests for each variation.
* If a function/component has multiple steps, its probably better to split/refactor it into multiple functions/components that can be unit tested individually.
* If there are a lot of edge cases, extract dedicated functions that cover them and use unit tests to test them.

### Running Unit Tests

!!! info
    For unit tests we are currently transitioning away from [ginkgo](https://github.com/onsi/ginkgo) to using golang native tests. The `make test-unit` target runs both ginkgo and golang native tests. Once the transition is complete this target will be simplified.

Run all unit tests

```bash
make test-unit
```

Run unit tests of specific packages:

```bash
# if you have not already installed gotestfmt tool then install it once.
# make test-unit target automatically installs this in ./hack/tools/bin. You can alternatively point the GOBIN to this directory and then directly invoke test-go.sh
> go install github.com/gotesttools/gotestfmt/v2/cmd/gotestfmt@v2.5.0
> ./hack/test-go.sh <package-1> <package-2>
```

### De-flaking Unit Tests

If tests have sporadic failures, then trying running `./hack/stress-test.sh` which internally uses [stress tool](https://pkg.go.dev/golang.org/x/tools/cmd/stress).

```bash
# install the stress tool
go install golang.org/x/tools/cmd/stress@latest
# invoke the helper script to execute the stress test
./hack/stress-test.sh test-package=<test-package> test-func=<test-function> tool-params="<tool-params>"
```

An example invocation:

```bash
./hack/stress-test.sh test-package=./internal/utils test-func=TestRunConcurrentlyWithAllSuccessfulTasks tool-params="-p 10"
5s: 877 runs so far, 0 failures
10s: 1906 runs so far, 0 failures
15s: 2885 runs so far, 0 failures
...
```
`stress` tool will output a path to a file containing the full failure message when a test run fails.


## Integration Tests (envtests)

Integration tests in etcd-druid use [envtest](https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/envtest). It sets up a minimal temporary control plane (etcd + kube-apiserver) and runs the test against it. Test suites (group of tests) start their individual `envtest` environment before running the tests for the respective controller/webhook. Before exiting, the temporary test environment is shutdown.

!!! info
    For integration-tests we are currently transitioning away from [ginkgo](https://github.com/onsi/ginkgo) to using golang native tests. All ginkgo integration tests can be found [here](https://github.com/gardener/etcd-druid/tree/4e9971aba3c3880a4cb6583d05843eabb8ca1409/test/integration) and golang native integration tests can be found [here](https://github.com/gardener/etcd-druid/tree/4e9971aba3c3880a4cb6583d05843eabb8ca1409/test/it).

* Integration tests in etcd-druid only targets a single controller. It is therefore advised that code (other than common utility functions should not be shared between any two controllers).
* If you are sharing a common `envtest` environment across tests then it is recommended that an individual test is run in a dedicated `namespace`.
* Since `envtest` is used to setup a minimum environment where no controller (e.g. KCM, Scheduler) other than `etcd` and `kube-apiserver` runs, status updates to resources controller/reconciled by not-deployed-controllers will not happen. Tests should refrain from asserting changes to status. In case status needs to be set as part of a test setup then it must be done explicitly.
* If you have common setup and teardown, then consider using [TestMain](https://pkg.go.dev/testing#hdr-Main) -[example](https://github.com/gardener/etcd-druid/blob/4e9971aba3c3880a4cb6583d05843eabb8ca1409/test/it/controller/etcd/reconciler_test.go#L34).
* If you have to wait for resources to be provisioned or reach a specific state, then it is recommended that you create smaller assertion functions and use Gomega's [AsyncAssertion](https://pkg.go.dev/github.com/onsi/gomega#AsyncAssertion) functions - [example](https://github.com/gardener/etcd-druid/blob/4e9971aba3c3880a4cb6583d05843eabb8ca1409/test/it/controller/etcd/assertions.go#L117-L140).
    * Beware of the default `Eventually` / `Consistently` timeouts / poll intervals: [docs](https://onsi.github.io/gomega/#eventually).
    * Don't forget to call `{Eventually,Consistently}.Should()`, otherwise the assertions always silently succeeds without errors: [onsi/gomega#561](https://github.com/onsi/gomega/issues/561)

### Running Integration Tests

```bash
make test-integration
```

### Debugging Integration Tests

There are two ways in which you can debug Integration Tests:
#### Using IDE

All commonly used IDE's provide in-built or easy integration with [delve](https://pkg.go.dev/github.com/go-delve/delve) debugger. For debugging integration tests the only additional requirement is to set `KUBEBUILDER_ASSETS` environment variable. You can get the value of this environment variable by executing the following command:
```bash
# ENVTEST_K8S_VERSION is the k8s version that you wish to use for testing.
setup-envtest --os $(go env GOOS) --arch $(go env GOARCH) use $ENVTEST_K8S_VERSION -p path
```

!!! tip
    All integration tests usually have a timeout. If you wish to debug a failing integration-test then increase the timeouts.

#### Use standalone envtest

We also provide a capability to setup a stand-alone `envtest` and leverage the cluster to run individual integration-test.  This allows you more control over when this k8s control plane is destroyed and allows you to inspect the resources at the end of the integration-test run using `kubectl`.

> While you can use an existing cluster (e.g., `kind`), some test suites expect that no controllers and no nodes are running in the test environment (as it is the case in `envtest` test environments). Hence, using a full-blown cluster with controllers and nodes might sometimes be impractical, as you would need to stop cluster components for the tests to work.

To setup a standalone `envtest` and run an integration test against it, do the following:

```bash
# In a terminal session use the following make target to setup a standalone envtest
make start-envtest
# As part of output path to kubeconfig will be also be printed on the console.

# In another terminal session setup resource(s) watch:
kubectl get po -A -w # alternatively you can also use `watch -d <command>` utility.

# In another terminal session:
export KUBECONFIG=<envtest-kubeconfig-path>
export USE_EXISTING_K8S_CLUSTER=true

# run the test
go test -run="<regex-for-test>" <package>
# example: go test -run="^TestEtcdDeletion/test deletion of all*" ./test/it/controller/etcd
```

Once you are done the testing you can press `Ctrl+C` in the terminal session where you started `envtest`. This will shutdown the kubernetes control plane.

## End-To-End (e2e) Tests

End-To-End tests are run using [Kind](https://kind.sigs.k8s.io/) cluster and [Skaffold](https://skaffold.dev/). These tests provide a high level of confidence that the code runs as expected by users when deployed to production.

* Purpose of running these tests is to be able to catch bugs which result from interaction amongst different components within etcd-druid.

* In CI pipelines e2e tests are run with S3 compatible [LocalStack](https://www.localstack.cloud/) (in cases where backup functionality has been enabled for an etcd cluster).

  > In future we will only be using a file-system based local provider to reduce the run times for the e2e tests when run in a CI pipeline.

* e2e tests can be triggered either with other cloud provider object-store emulators or they can also be run against actual/remove cloud provider object-store services.

* In contrast to integration tests, in e2e tests, it might make sense to specify higher timeouts for Gomega's [AsyncAssertion](https://pkg.go.dev/github.com/onsi/gomega#AsyncAssertion) calls.

### Running e2e tests locally

Detailed instructions on how to run e2e tests can be found [here](https://github.com/gardener/etcd-druid/blob/4e9971aba3c3880a4cb6583d05843eabb8ca1409/docs/development/local-e2e-tests.md).
