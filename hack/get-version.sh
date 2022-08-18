#!/bin/bash -e

# Copyright (c) 2022 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.


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