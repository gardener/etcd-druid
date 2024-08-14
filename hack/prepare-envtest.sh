#!/usr/bin/env bash
#
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

set -o errexit
set -o nounset
set -o pipefail

ENVTEST_K8S_VERSION=${ENVTEST_K8S_VERSION:-"1.30"}

echo "> Installing envtest tools@${ENVTEST_K8S_VERSION} with setup-envtest if necessary"
if ! command -v setup-envtest &> /dev/null ; then
  >&2 cat <<EOF
  setup-envtest not available
  You can either directly install it by following the instructions at https://pkg.go.dev/sigs.k8s.io/controller-runtime/tools/setup-envtest#section-readme
  Or you can use `make test-integration` which will implicitly install it for you.
EOF
  exit 1
fi

# --use-env allows overwriting the envtest tools path via the KUBEBUILDER_ASSETS env var
export KUBEBUILDER_ASSETS="$(setup-envtest use --use-env -p path ${ENVTEST_K8S_VERSION})"
echo "using envtest tools installed at '$KUBEBUILDER_ASSETS'"

export $(grep -v '^#' "$(dirname "$0")/common-test-integration.env" | xargs -0)