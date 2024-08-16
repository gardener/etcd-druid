# Raising a Pull Request

We welcome active contributions from the community. This document details out the things-to-de-done in order for us to consider a PR for review. Contributors should follow the guidelines mentioned in this document to minimize the time it takes to get the PR reviewed.

## 00-Prerequisites

In order to make code contributions you must setup your development environment. Follow the [Prepare Dev Environment Guide](../prepare-dev-environment.md) for detailed instructions.

## 01-Raise an Issue

For every pull-request, it is ***mandatory*** to raise an [Issue](https://github.com/gardener/etcd-druid/issues) which should describe the problem in detail. We have created a few categories, each having its own dedicated [template](https://github.com/gardener/etcd-druid/tree/master/.github/ISSUE_TEMPLATE).

## 03-Prepare Code Changes

* It is ***not*** recommended to create a branch on the main repository for raising pull-requests. Instead you must fork the `etcd-druid` [repository](https://github.com/gardener/etcd-druid) and create a branch in the fork. You can follow the [detailed instructions](https://docs.github.com/en/pull-requests/collaborating-with-pull-requests/working-with-forks/fork-a-repo) on how to fork a repository and set it up for contributions.

* Ensure that you follow the [coding guidelines](https://google.github.io/styleguide/go/decisions) while introducing new code.

* If you are making changes to the API then please read [Changing-API documentation](changing-api.md).

* If you are introducing new go mod dependencies then please read [Dependency Management documentation](dependency-management.md).

* If you are introducing a new `Etcd` cluster component then please read [Add new Cluster Component documentation](add-new-etcd-cluster-component.md).

* For guidance on testing, follow the detailed instructions [here](testing.md).

* Before you submit your PR, please ensure that the following is done:

  * Run `make check` which will do the following:

    * Runs `make format` - this target will ensure a common formatting of the code and ordering of imports across all source files.
    * Runs `make manifests` - this target will re-generate manifests if there are any changes in the API.
    * Only when the above targets have run without errorrs, then `make check` will be run linters against the code. The rules for the linter are configured [here](https://github.com/gardener/etcd-druid/blob/3383e0219a6c21c6ef1d5610db964cc3524807c8/.golangci.yaml).

  * Ensure that all the tests pass by running the following `make` targets:

    * `make test-unit` - this target will run all unit tests.
    * `make test-integration` - this target will run all integration tests (controller level tests) using `envtest` framework.
    * `make ci-e2e-kind` or any of its variants - these targets will run etcd-druid e2e tests.

    > **Note:** Please ensure that after introduction of new code the code coverage does not reduce. An increase in code coverage is always welcome.
  
* If you add new features, make sure that you create relevant documentation under `/docs`.

## 04-Raise a pull request

* Create *Work In Progress [WIP]* pull requests only if you need a clarification or an explicit review before you can continue your work item.
* Ensure that you have rebased your fork's development branch with `upstream` main/master branch.
* Squash all commits into a minimal number of commits.
* Fill in the PR template with appropriate details and provide the link the `Issue` for which a PR has been raised.
* If your patch is not getting reviewed, or you need a specific person to review it, you can @-reply a reviewer asking for a review in the pull request or a comment.

## 05-Post review

* If a reviewer requires you to change your commit(s), please test the changes again.
* Amend the affected commit(s) and force push onto your branch.
* Set respective comments in your GitHub review as resolved.
* Create a general PR comment to notify the reviewers that your amendments are ready for another round of review.

## 06-Merging a pull request

* Merge can only be done if the PR has approvals from atleast 2 reviewers.
* Add an appropriate release note detailing what is introduced as part of this PR.
* Before merging the PR, ensure that you squash and then merge.