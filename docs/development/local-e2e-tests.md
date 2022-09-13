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

> Valid credentials need to be provided when tests happen with mentioned cloud providers.

## Flow

An e2e test execution involves the following steps:

| Step   	  | Description                                                                                                                                                                                                |
|-----------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `setup`	  | Create a storage bucket which is used for etcd backups (only with cloud providers). 	                                                                                                                      |
| `deploy`	 | Build Docker image, upload it to registry (if remote cluster - see [Docker build](https://skaffold.dev/docs/pipeline-stages/builders/docker/)), deploy Helm chart (`charts/druid`) to Kubernetes cluster.	 |
| `test`    | Execute e2e tests as defined in `test/e2e`.                                                                                                                                                                |
| `undeploy` | Remove the deployed artifacts from Kubernetes cluster.                                                                                                                                                     |
| `cleanup` | Delete storage bucket and Druid deployment from test cluster.	                                                                                                                                             |

### Make target

Executing e2e-tests is as easy as executing the following command **with defined Env-Vars as desribed in the following
section and as needed for your test scenario**.

```shell
make test-e2e
```

### Common Env Variables

The following environment variables influence how the flow described above is executed:

- `PROVIDERS`:  Providers used for testing (`all`, `aws`, `azure`, `gcp`). Multiple entries must be comma separated. 
    > **Note**: Some tests will use very first entry from env `PROVIDERS` for e2e testing (ex: multi-node tests). So for multi-node tests to use specific provider, specify that provider as first entry in env `PROVIDERS`.
- `KUBECONFIG`: Kubeconfig pointing to cluster where Etcd-Druid will be deployed (preferably [KinD](https://kind.sigs.k8s.io)).
- `TEST_ID`:    Some ID which is used to create assets for and during testing.
- `STEPS`:      Steps executed by `make` target (`setup`, `deploy`, `test`, `undeploy`, `cleanup` - default: all steps).

### AWS Env Variables

- `AWS_ACCESS_KEY_ID`:       Key ID of the user.
- `AWS_SECRET_ACCESS_KEY`:   Access key of the user.
- `AWS_REGION`:              Region in which the test bucket is created.

Example:

```
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

```
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

```
make \
  GCP_SERVICEACCOUNT_JSON_PATH="/var/lib/secrets/serviceaccount.json" \
  GCP_PROJECT_ID="xyz-project" \
  KUBECONFIG="$HOME/.kube/config" \
  PROVIDERS="gcp" \
  TEST_ID="some-test-id" \
  STEPS="setup,deploy,test,undeploy,cleanup" \
test-e2e
```