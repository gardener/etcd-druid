#!/usr/bin/env bash
#
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

set -o errexit
set -o nounset
set -o pipefail

CRD_FILENAME_PREFIX=""

function create_usage() {
  usage=$(printf '%s\n' "
    usage: $(basename $0) [Options]
    Options:
      -p, --prefix <prefix>  (Optional) Prefix for the CRD name
    ")
  echo "$usage"
}

function check_prerequisites() {
  if ! command -v controller-gen &>/dev/null; then
    echo >&2 "controller-gen not available"
    exit 1
  fi
}

function parse_flags() {
  while test $# -gt 0; do
    case "$1" in
    -p | --prefix)
      shift
      CRD_FILENAME_PREFIX="$1"
      ;;
    -h | --help)
      shift
      echo "${USAGE}"
      exit 0
      ;;
    *)
      echo >&2 "Unknown flag: $1"
      exit 1
      ;;
    esac
    shift
  done
}

function generate_crds() {
  local output_dir="$(pwd)"

  # generate crds
  controller-gen crd paths="github.com/gardener/etcd-druid/api/v1alpha1" output:crd:dir="${output_dir}" output:stdout

  # rename files adding prefix if any specified
  if [ -n "${CRD_FILENAME_PREFIX}" ]; then
    find "${output_dir}" -maxdepth 1 -name "druid.gardener.cloud*" -type f -print0 |
      while IFS= read -r -d '' crd_file; do
        crd_out_file="${output_dir}/${CRD_FILENAME_PREFIX}$(basename "$crd_file")"
        mv "${crd_file}" "${crd_out_file}"
      done
  fi
}

export GO111MODULE=off
echo "Generating CRDs for druid.gardener.cloud"
check_prerequisites
parse_flags "$@"
generate_crds