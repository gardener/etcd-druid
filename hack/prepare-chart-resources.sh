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

# get_k8s_version gets the k8s version by running 'kubectl version' and parsing the output.
function get_k8s_version() {
   echo $(kubectl version -oyaml | yq '(.serverVersion.major) + "." + (.serverVersion.minor)')
}

# regex has been taken from https://semver.org and support for an optional prefix of 'v' has been added and patch version has been also made optional.
SEMANTIC_VERSION_REGEX='^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(\.(0|[1-9][0-9]*))?(-((0|[1-9][0-9]*|[0-9]*[a-zA-Z-][0-9a-zA-Z-]*)(\.(0|[1-9][0-9]*|[0-9]*[a-zA-Z-][0-9a-zA-Z-]*))*))?(\+([0-9a-zA-Z-]+(\.[0-9a-zA-Z-]+)*))?$'

# parse_semver parses the k8s semantic version and extracts major and minor versions.
function parse_semver() {
  local major=0
  local minor=0
  if egrep $SEMANTIC_VERSION_REGEX <<<"${K8S_VERSION}" >/dev/null 2>&1; then
    local tokens=${K8S_VERSION//[!0-9.]/ }
    local token_arr=( ${tokens//\./ } )
    major=${token_arr[0]}
    minor=${token_arr[1]}
  else
    echo "Invalid version format: ${K8S_VERSION}"
    exit 1
  fi
  echo "${major} ${minor}"
}

# is_k8s_version_ge_1_29 checks if the target k8s version is greater than or equal to 1.29
function is_k8s_version_ge_1_29() {
  local major_minor=( $(parse_semver) )
  if [[ ${major_minor[0]} -gt 1 ]] || [[ ${major_minor[0]} -eq 1 && ${major_minor[1]} -ge 29 ]]; then
   true
  else
   false
  fi
}

# get_crd_file_names returns the CRD file names based on the target k8s version.
function get_crd_file_names() {
  if is_k8s_version_ge_1_29; then
   declare -a crds=("druid.gardener.cloud_etcds.yaml" "druid.gardener.cloud_etcdcopybackupstasks.yaml")
  else
   declare -a crds=("druid.gardener.cloud_etcds_without_cel.yaml" "druid.gardener.cloud_etcdcopybackupstasks.yaml")
  fi
  echo "${crds[*]}"
}

# copy_crds copies the CRDs from the API to the helm charts directory.
# It will ensure that CRDs specific to a target k8s version are selected.
function copy_crds() {
  target_path="${PROJECT_ROOT}/charts/crds"
  echo "Creating ${target_path} to copy the CRDs if not present..."
  mkdir -p ${target_path}
  echo "Cleaning up existing CRDs if any..."
  rm -rf "${target_path}"/*
  crd_file_names=($(get_crd_file_names))
  for crd_file_name in "${crd_file_names[@]}"; do
    local source_crd_path="${PROJECT_ROOT}/api/core/crds/${crd_file_name}"
    if [ ! -f ${source_crd_path} ]; then
      echo -e "CRD ${crd_file_name} not found in ${PROJECT_ROOT}/api/core/crds, run 'make generate' first"
      exit 1
    fi
    echo "Copying CRD ${crd_file_name} to ${target_path}"
    cp "${source_crd_path}" "${target_path}"
  done
}

function initialize_pki_resources() {
  local generation_required=false

  target_path="${PROJECT_ROOT}/charts/pki-resources"
  if [ ! -d "${target_path}" ]; then
    mkdir -p "${target_path}"
    generation_required=true
  fi

  if [[ "${generation_required}" == true || $(all_pki_resources_exist) == false || $(has_required_sans) == false ]]; then
    echo "Generating PKI resources..."
    rm -rf ${target_path}/*
    pki::generate_resources "${target_path}" "${NAMESPACE}" "${CERT_EXPIRY}"
  else
    echo "PKI resources are already present and valid"
  fi
}

# all_pki_resources_exist checks if all required PKI resources are present. This function will not check
# the contents of the server certificates but only if the expected files with the required names are present.
function all_pki_resources_exist() {
  sourceDir="${PROJECT_ROOT}/charts/pki-resources/"
  local pki_resources=(
   "ca.crt"
   "ca.key"
   "server.crt"
   "server.key"
  )
  local result=true
  for resource in "${pki_resources[@]}"; do
    local resourcePath="${sourceDir}/${resource}"
    if [[ ! -f ${resourcePath} ]]; then
      result=false
      break
    fi
  done
  echo ${result}
}

# has_required_sans checks if the server certificate has the required SANs
# SAN list should contain names with 'default' namespace and the target namespace.
function has_required_sans() {
  expectedSANs=("etcd-druid" "etcd-druid.default" "etcd-druid.default.svc" "etcd-druid.default.svc.cluster.local" "etcd-druid.${NAMESPACE}" "etcd-druid.${NAMESPACE}.svc" "etcd-druid.${NAMESPACE}.svc.cluster.local")
  actualSANs=()
    # extract SANs from the server certificate
   for san in $(openssl x509 -in ${PROJECT_ROOT}/charts/pki-resources/server.crt -noout -text | grep -A1 'Subject Alternative Name' | tail -n1 | tr -d ','); do
     sanitizedSAN=$(echo "${san}" | cut -f2 -d: )
     actualSANs+=( ${sanitizedSAN} )
   done

  local result=false
  diff_sans=( $(echo ${expectedSANs[@]} ${actualSANs[@]} | tr ' ' '\n' | sort | uniq -u) )
  if [ ${#diff_sans[@]} -eq 0 ]; then
    result=true
  fi
  echo ${result}
}

# prepare_chart_resources prepares the helm chart resources by copying CRDs and generating PKI resources if required.
function prepare_chart_resources() {
  check_prerequisites
  parse_flags "$@"
  if [ -z "${K8S_VERSION}" ]; then
    echo "Attempting to get kubernetes version via kubectl..."
    K8S_VERSION=$(get_k8s_version | tr -d '[:blank:]')
  fi
  if [ -z "${K8S_VERSION}" ]; then
    echo -e "No kubernetes version found. Either target a kubernetes cluster or explicitly specify the k8s version."
    echo -e "${USAGE}"
    exit 1
  fi
  echo "Copying CRDs to helm charts..."
  copy_crds
  echo "Generating PKI resources if not present or expired..."
  initialize_pki_resources
}

USAGE=$(create_usage)
prepare_chart_resources "$@"