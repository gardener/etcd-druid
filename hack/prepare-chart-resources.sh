#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0


set -o errexit
set -o nounset
set -o pipefail

source $(dirname $0)/openssl-utils.sh

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"


# regex has been taken from https://semver.org and support for an optional prefix of 'v' has been added.
SVER_REGEX='^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-((0|[1-9][0-9]*|[0-9]*[a-zA-Z-][0-9a-zA-Z-]*)(\.(0|[1-9][0-9]*|[0-9]*[a-zA-Z-][0-9a-zA-Z-]*))*))?(\+([0-9a-zA-Z-]+(\.[0-9a-zA-Z-]+)*))?$'

function parse_semver() {
  if [[ $# -ne 1 ]]; then
    echo -e "${FUNCNAME[0]} requires 1 argument: version"
    exit 1
  fi
  local version="$1"
  local major=0
  local minor=0

  if egrep $SVER_REGEX <<<"${version}" >/dev/null 2>&1; then
    local tokens=${version//[!0-9.]/ }
    local token_arr=(${tokens//\./ })
    major=${token_arr[0]}
    minor=${token_arr[1]}
  else
    echo "Invalid version format: ${version}"
    exit 1
  fi
  echo "${major} ${minor}"
}

function is_k8s_version_ge_1_29() {
  if [[ $# -ne 1 ]]; then
    echo -e "${FUNCNAME[0]} requires 1 argument: version"
    exit 1
  fi
  local version="$1"
  local major_minor=($(parse_semver "${version}"))
  if [[ ${major_minor[0]} -gt 1 ]] || [[ ${major_minor[0]} -eq 1 && ${major_minor[1]} -ge 29 ]]; then
   true
  else
   false
  fi
}

function get_crd_file_names() {
  if [[ $# -ne 1 ]]; then
    echo -e "${FUNCNAME[0]} requires 1 argument: version"
    exit 1
  fi
  local version="$1"
  if is_k8s_version_ge_1_29 "${version}"; then
   declare -a crds=("druid.gardener.cloud_etcds.yaml" "druid.gardener.cloud_etcdcopybackupstasks.yaml")
  else
   declare -a crds=("druid.gardener.cloud_etcds_without_cel.yaml" "druid.gardener.cloud_etcdcopybackupstasks.yaml")
  fi
  echo "${crds[*]}"
}

function copy_crds() {
  if [[ $# -ne 1 ]]; then
    echo -e "${FUNCNAME[0]} requires 1 argument: kubernetes version"
    exit 1
  fi
  local version="$1"

  target_path="${PROJECT_ROOT}/charts/crds"
  echo "Creating ${target_path} to copy the CRDs if not present..."
  mkdir -p ${target_path}

  crd_file_names=($(get_crd_file_names "${version}"))
  for crd_file_name in "${crd_file_names[@]}"; do
    local source_crd_path="${PROJECT_ROOT}/api/core/crds/${crd_file_name}"
    if [ ! -f ${source_crd_path} ]; then
      echo -e "CRD ${crd} not found in ${PROJECT_ROOT}/api/core/crds, run 'make generate' first"
      exit 1
    fi
    echo "Copying CRD ${crd_file_name} to ${target_path}"
    cp ${source_crd_path} ${target_path}
  done
}

function initialize_pki_resources() {
  if [[ $# -ne 3 ]]; then
    echo -e "${FUNCNAME[0]} requires 3 arguments: namespace, cert-expiry and kubernetes version"
    exit 1
  fi

  local namespace="$1"
  local cert_expiry_days="$2"
  local generation_required=false

  target_path="${PROJECT_ROOT}/charts/pki-resources"
  if [ ! -d ${target_path} ]; then
    mkdir -p ${target_path}
    generation_required=true
  fi

  if ${generation_required} || ! all_pki_resources_exist; then
    echo "Generating PKI resources..."
    rm -rf ${target_path}/*
    pki::generate_resources "${target_path}" "${namespace}" "${cert_expiry_days}"
  fi
}

function all_pki_resources_exist() {
  sourceDir="${PROJECT_ROOT}/charts/pki-resources/"
  PKI_RESOURCES=(
   "ca.crt"
   "ca.key"
   "server.crt"
   "server.key"
  )
  for resource in "${PKI_RESOURCES[@]}"; do
    local resourcePath="${sourceDir}/${resource}"
    if [ ! -f ${resourcePath} ]; then
      return 1
    fi
  done
  return 0
}

function prepare_chart_resources() {
  if [[ $# -ne 3 ]]; then
    echo -e "${FUNCNAME[0]} requires 3 arguments: namespace, cert-expiry and kubernetest version"
    exit 1
  fi
  echo "Copying CRDs to helm charts..."
  # echo $3
  copy_crds $3
  echo "Generating PKI resources if not present or expired..."
  initialize_pki_resources "$@"
}

prepare_chart_resources "$@"