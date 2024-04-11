#!/usr/bin/env bash
#
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

set -o errexit
set -o nounset
set -o pipefail

CRD_FILENAME_PREFIX=""
CRD_DELETION_PROTECTION_LABEL=false
REPO_ROOT="$(git rev-parse --show-toplevel)"

function create_usage() {
  usage=$(printf '%s\n' "
    usage: $(basename $0) [Options]
    Options:
      -p, --prefix <prefix>  (Optional) Prefix for the CRD name
      -l, --label <label>    (Optional) If this flag is set, the CRD will have the label gardener.cloud/deletion-protected: true
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
    -l | --label)
      CRD_DELETION_PROTECTION_LABEL=true
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

  etcd_api_types_file="${REPO_ROOT}/api/v1alpha1/types_etcd.go"
  # Create a local copy outside the mod cache path in order to patch the types crd_file via sed.
  etcd_api_types_backup="$(mktemp -d)/types_etcd.go"
  cp "${etcd_api_types_file}" "${etcd_api_types_backup}"
  chmod +w "${etcd_api_types_file}" "${REPO_ROOT}/api/v1alpha1/"

  trap 'revert_changes "${etcd_api_types_backup}" "${etcd_api_types_file}"' EXIT

  # /scale subresource is intentionally removed from this CRD, although it is specified in the original CRD from
  # etcd-druid, due to adverse interaction with VPA.
  # See https://github.com/gardener/gardener/pull/6850 and https://github.com/gardener/gardener/pull/8560#discussion_r1347470394
  # TODO(shreyas-s-rao): Remove this workaround as soon as the scale subresource is supported properly.
  sed -i '/\/\/ +kubebuilder:subresource:scale:specpath=.spec.replicas,statuspath=.status.replicas,selectorpath=.status.labelSelector/d' "${etcd_api_types_file}"

  # generate crds
  controller-gen crd paths="github.com/gardener/etcd-druid/api/v1alpha1" output:crd:dir="${output_dir}" output:stdout

  # rename files adding prefix if any specified
  find "${output_dir}" -maxdepth 1 -name "druid.gardener.cloud*" -type f -print0 |
    while IFS= read -r -d '' crd_file; do
      crd_out_file="${output_dir}/${CRD_FILENAME_PREFIX}$(basename "$crd_file")"
      mv "${crd_file}" "${crd_out_file}"
      # add deletion protection label if specified
      if ${CRD_DELETION_PROTECTION_LABEL}; then
        sed -i '4 a\  labels:\n\    gardener.cloud/deletion-protected: "true"' "${crd_out_file}"
      fi
    done
}

function revert_changes() {
  if [[ $# -ne 2 ]]; then
    echo -e "${FUNCNAME[0]} expects two arguments: <etcd_api_types_backup> <etcd_api_types_file>"
    exit 1
  fi
  local backup_file=$1
  local updated_file=$2
  echo "Reverting changes to ${updated_file} from ${backup_file}"
  cp "${backup_file}" "${updated_file}"
  chmod -w "${REPO_ROOT}/api/v1alpha1/"
}

export GO111MODULE=off
echo "Generating CRDs for druid.gardener.cloud"
check_prerequisites
parse_flags "$@"
generate_crds
