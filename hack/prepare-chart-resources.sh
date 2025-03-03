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



function compare_k8s_version() {
  k_version="$1"
  version=$(echo "$k_version" | tr -d ".")
  # minor_version=$(echo "k_version" | cut -d "." -f 2)

  if [[ "$version" -ge 129 ]] ; then
    return 0
  else
    return 1
  fi

}

function copy_crds() {
  
  if compare_k8s_version "$1"; then 
    echo "Kubernetes version is greater than 1.29."
    declare -a crds=("druid.gardener.cloud_etcds.yaml" "druid.gardener.cloud_etcdcopybackupstasks.yaml")
  else 
    echo "Kubernetes version less than 1.29"
    declare -a crds=("druid.gardener.cloud_etcds_without_cel.yaml" "druid.gardener.cloud_etcdcopybackupstasks.yaml")
  fi
  target_path="${PROJECT_ROOT}/charts/crds"
  echo "Creating ${target_path} to copy the CRDs if not present..."
  mkdir -p ${target_path}
  for crd in "${crds[@]}"; do
    local crd_path="${PROJECT_ROOT}/api/core/crds/${crd}"
    if [ ! -f ${crd_path} ]; then
      echo -e "CRD ${crd} not found in ${PROJECT_ROOT}/api/core/crds, run 'make generate' first"
      exit 1
    fi
    echo "Copying CRD ${crd} to ${target_path}"
    cp ${crd_path} ${target_path}
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