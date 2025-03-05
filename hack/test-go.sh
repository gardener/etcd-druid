#!/usr/bin/env bash
#
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

set -o errexit
set -o nounset
set -o pipefail

export ENVTEST_K8S_VERSION=${ENVTEST_K8S_VERSION:-"1.29"}

if ${SETUP_ENVTEST:-false}; then
  echo "> Installing envtest tools@${ENVTEST_K8S_VERSION} with setup-envtest"
  if ! command -v setup-envtest &>/dev/null; then
    echo >&2 "setup-envtest not available"
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
  export KUBEBUILDER_CONTROLPLANE_START_TIMEOUT=2m
fi

echo "> Go tests"

if ${TEST_COVER:-false}; then
  mkdir -p test/output
  go test -cover "$@"
  exit 0
fi

go test "$@"
