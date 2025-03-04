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
NAMESPACE="default"
CERT_EXPIRY=365
K8S_VERSION=""
USAGE=""

function create_usage() {
  usage=$(printf '%s\n' "
  usage: $(basename $0) [Options]
  Options:
    --namespace | -n  <namespace>     Target namespace where resources will be deployed. These will be used to generate PKI resources. Defaults to 'default'
    --cert-expiry | -e <expiry>       Expiry of the certificates in days. Defaults to 365 days.
    --k8s-version | -v <k8s version>  Target Kubernetes version. This is optional, if not specified it will query by running 'kubectl version'.
  ")
  echo "${usage}"
}

function check_prerequisites() {
  if ! command -v kubectl &> /dev/null; then
    echo "kubectl is not installed. Please install kubectl from https://kubernetes.io/docs/tasks/tools/"
    exit 1
  fi
  if ! command -v yq &> /dev/null; then
    echo "yq is not installed. Please install yq from https://mikefarah.gitbook.io/yq/"
    exit 1
  fi
}

function parse_flags() {
  while test $# -gt 0; do
    case "$1" in
      --namespace | -n)
        shift
        NAMESPACE=$1
        ;;
      --cert-expiry | -e)
        shift
        CERT_EXPIRY=$1
        ;;
      --k8s-version | -v)
        shift
        K8S_VERSION=$1
        ;;
      -h | --help)
        shift
        echo "${USAGE}"
        exit 0
        ;;
    esac
    shift
  done
}

function get_k8s_version() {
   echo $(kubectl version -oyaml | yq '(.serverVersion.major) + "." + (.serverVersion.minor)')
}

# regex has been taken from https://semver.org and support for an optional prefix of 'v' has been added and patch version has been also made optional.
SEMANTIC_VERSION_REGEX='^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(\.(0|[1-9][0-9]*))?(-((0|[1-9][0-9]*|[0-9]*[a-zA-Z-][0-9a-zA-Z-]*)(\.(0|[1-9][0-9]*|[0-9]*[a-zA-Z-][0-9a-zA-Z-]*))*))?(\+([0-9a-zA-Z-]+(\.[0-9a-zA-Z-]+)*))?$'

function parse_semver() {
  local major=0
  local minor=0
  if egrep $SEMANTIC_VERSION_REGEX <<<"${K8S_VERSION}" >/dev/null 2>&1; then
    local tokens=${K8S_VERSION//[!0-9.]/ }
    local token_arr=(${tokens//\./ })
    major=${token_arr[0]}
    minor=${token_arr[1]}
  else
    echo "Invalid version format: ${K8S_VERSION}"
    exit 1
  fi
  echo "${major} ${minor}"
}

function is_k8s_version_ge_1_29() {
  local major_minor=($(parse_semver))
  if [[ ${major_minor[0]} -gt 1 ]] || [[ ${major_minor[0]} -eq 1 && ${major_minor[1]} -ge 29 ]]; then
   true
  else
   false
  fi
}

function get_crd_file_names() {
  if is_k8s_version_ge_1_29; then
   declare -a crds=("druid.gardener.cloud_etcds.yaml" "druid.gardener.cloud_etcdcopybackupstasks.yaml")
  else
   declare -a crds=("druid.gardener.cloud_etcds_without_cel.yaml" "druid.gardener.cloud_etcdcopybackupstasks.yaml")
  fi
  echo "${crds[*]}"
}

function copy_crds() {
  target_path="${PROJECT_ROOT}/charts/crds"
  echo "Creating ${target_path} to copy the CRDs if not present..."
  mkdir -p ${target_path}

  crd_file_names=($(get_crd_file_names))
  for crd_file_name in "${crd_file_names[@]}"; do
    local source_crd_path="${PROJECT_ROOT}/api/core/crds/${crd_file_name}"
    if [ ! -f ${source_crd_path} ]; then
      echo -e "CRD ${crd_file_name} not found in ${PROJECT_ROOT}/api/core/crds, run 'make generate' first"
      exit 1
    fi
    echo "Copying CRD ${crd_file_name} to ${target_path}"
    cp ${source_crd_path} ${target_path}
  done
}

function initialize_pki_resources() {
  local generation_required=false

  target_path="${PROJECT_ROOT}/charts/pki-resources"
  if [ ! -d ${target_path} ]; then
    mkdir -p ${target_path}
    generation_required=true
  fi

  if ${generation_required} || ! all_pki_resources_exist; then
    echo "Generating PKI resources..."
    rm -rf ${target_path}/*
    pki::generate_resources "${target_path}" "${NAMESPACE}" "${CERT_EXPIRY}"
  fi
}

function all_pki_resources_exist() {
  sourceDir="${PROJECT_ROOT}/charts/pki-resources/"
  local pki_resources=(
   "ca.crt"
   "ca.key"
   "server.crt"
   "server.key"
  )
  for resource in "${pki_resources[@]}"; do
    local resourcePath="${sourceDir}/${resource}"
    if [ ! -f ${resourcePath} ]; then
      return 1
    fi
  done
  return 0
}

function prepare_chart_resources() {
  check_prerequisites
  parse_flags "$@"
  if [ -z "${K8S_VERSION}" ]; then
    echo "Attempting to get kubernetes version via kubectl..."
    K8S_VERSION=$(get_k8s_version | tr -d '[:blank:]')
  fi
  echo "Copying CRDs to helm charts..."
  copy_crds
  echo "Generating PKI resources if not present or expired..."
  initialize_pki_resources
}

USAGE=$(create_usage)
prepare_chart_resources "$@"