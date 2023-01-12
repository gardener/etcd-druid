#!/usr/bin/env bash
#
# Copyright (c) 2021 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

set -o errexit
set -o nounset
set -o pipefail

ENVTEST_K8S_VERSION=${ENVTEST_K8S_VERSION:-"1.22"}

echo "> Installing envtest tools@${ENVTEST_K8S_VERSION} with setup-envtest if necessary"
if ! command -v setup-envtest &> /dev/null ; then
  >&2 echo "setup-envtest not available"
  exit 1
fi

ARCH=
# if using M1 macbook, use amd64 architecture build, as suggested in
# https://github.com/kubernetes-sigs/controller-runtime/issues/1657#issuecomment-988484517
if [[ $(uname) == 'Darwin' && $(uname -m) == 'arm64' ]]; then
  ARCH='--arch=amd64'
fi

# --use-env allows overwriting the envtest tools path via the KUBEBUILDER_ASSETS env var just like it was before
export KUBEBUILDER_ASSETS="$(setup-envtest ${ARCH} use --use-env -p path ${ENVTEST_K8S_VERSION})"
echo "using envtest tools installed at '$KUBEBUILDER_ASSETS'"

echo "> Tests"

if [[ $(uname) == 'Darwin' ]]; then
  SED_BIN="gsed"
else
  SED_BIN="sed"
fi

export KUBEBUILDER_CONTROLPLANE_START_TIMEOUT=2m
export GOMEGA_DEFAULT_EVENTUALLY_TIMEOUT=5s
export GOMEGA_DEFAULT_EVENTUALLY_POLLING_INTERVAL=200ms
GINKGO_COMMON_FLAGS="-r -timeout=1h0m0s --randomize-all --randomize-suites --fail-on-pending --progress"

if ${TEST_COV:-false}; then
  output_dir=test/output
  coverprofile_file=coverprofile.out
  mkdir -p test/output
  ginkgo $GINKGO_COMMON_FLAGS --coverprofile ${coverprofile_file} -covermode=set -outputdir ${output_dir} $@
  ${SED_BIN} -i '/mode: set/d' ${output_dir}/${coverprofile_file}
  {( echo "mode: set"; cat ${output_dir}/${coverprofile_file} )} > ${output_dir}/${coverprofile_file}.temp
  mv ${output_dir}/${coverprofile_file}.temp ${output_dir}/${coverprofile_file}
  go tool cover -func ${output_dir}/${coverprofile_file}
  exit 0
fi

ginkgo -trace $GINKGO_COMMON_FLAGS $@
