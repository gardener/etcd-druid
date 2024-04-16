#!/bin/bash -e

# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

# The test-machinery testrunner injects two environment variables into test runs to specify which version should be
# tested (TM_GIT_SHA and TM_GIT_REF, see https://github.com/gardener/test-infra/blob/master/docs/testmachinery/GetStarted.md#input-contract)
# Use those to calculate the terraformer version in order to tell the e2e test (via ldflags), which terraformer image
# should be deployed and tested.
if [ -n "$TM_GIT_REF" ] ; then
  # running e2e test in a release job, use TM_GIT_REF as image tag (is set to git release tag name)
  echo "$TM_GIT_REF"
  exit 0
fi

VERSION="$(cat "$(dirname $0)/../VERSION")"

if [ -n "$TM_GIT_SHA" ] ; then
  # running e2e test for a PR, calculate image tag by concatenating VERSION and commit sha.
  echo "$VERSION-$TM_GIT_SHA"
  exit 0
fi

if [[ "$VERSION" = *-dev ]] ; then
  VERSION="$VERSION-$(git rev-parse HEAD)"
fi

# .dockerignore ignores all files unrelevant for build (e.g. example/*) to only copy relevant source files to the build
# container. Hence, git will always detect a dirty work tree when building in a container (many deleted files).
# This command filters out all deleted files that are ignored by .dockerignore to only detect changes to relevant files
# as a dirty work tree.
# Additionally, it filters out changes to the `VERSION` file, as this is currently the only way to inject the
# version-to-build in our pipelines (see https://github.com/gardener/cc-utils/issues/431).
TREE_STATE="$([ -z "$(git status --porcelain 2>/dev/null | grep -vf <(git ls-files -o --deleted --ignored --exclude-from=.dockerignore) -e 'VERSION')" ] && echo clean || echo dirty)"

if [ "$TREE_STATE" = dirty ] ; then
  VERSION="$VERSION-dirty"
fi

echo "$VERSION"