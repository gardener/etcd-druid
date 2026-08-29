#!/usr/bin/env bash
# SPDX-FileCopyrightText: Contributors to the Gardener project
#
# SPDX-License-Identifier: Apache-2.0


set -o errexit
set -o nounset
set -o pipefail

source $(dirname $0)/ld-flags.sh

function check_prereq() {
  if ! command -v skaffold &>/dev/null; then
    echo >&2 "skaffold is not installed, please install skaffold from https://skaffold.dev/docs/install/"
    exit 1
  fi
}

function skaffold_deploy() {
  local ld_flags=$(build_ld_flags)
  export LD_FLAGS="${ld_flags}"
  skaffold "$@"
}

function main() {
    echo "Checking prerequisites..."
    check_prereq
    echo "Skaffolding etcd-druid operator..."
    skaffold_deploy "$@"
}

main "$@"
