#!/usr/bin/env bash
#
# SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

set -o errexit
set -o nounset
set -o pipefail

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
API_GO_MODULE_ROOT="$(dirname "$SCRIPT_DIR")"
PROJECT_ROOT="$(dirname "$API_GO_MODULE_ROOT")"

CODE_GEN_DIR=$(go list -m -f '{{.Dir}}' k8s.io/code-generator)
source "${CODE_GEN_DIR}/kube_codegen.sh"

function check_controller_gen_prereq() {
  if ! command -v controller-gen &>/dev/null; then
    echo >&2 "controller-gen is not available, cannot generate deepcopy/runtime.Object for the API types and cannot generate CRDs"
    exit 1
  fi
}

function generate_deepcopy_defaulter() {
  kube::codegen::gen_helpers \
    --boilerplate "${SCRIPT_DIR}/boilerplate.generatego.txt" \
    "${API_GO_MODULE_ROOT}/core/v1alpha1"
}

function generate_clientset() {
  kube::codegen::gen_client \
    --with-watch \
    --output-dir "${PROJECT_ROOT}/client" \
    --output-pkg "github.com/gardener/etcd-druid/client" \
    --boilerplate "${SCRIPT_DIR}/boilerplate.generatego.txt" \
    "${PROJECT_ROOT}/api"
}

function generate_crds() {
  local output_dir="${API_GO_MODULE_ROOT}/core/crds"
  local package="github.com/gardener/etcd-druid/api/core/v1alpha1"
  local package_path="$(go list -f '{{.Dir}}' "${package}")"

  if [ -z "${package_path}" ]; then
    echo >&2 "Could not locate directory for package: ${package}"
    exit 1
  fi

  if [ -z "${output_dir}" ]; then
    mkdir -p "${output_dir}"
  fi

  # clean all generated crd files
  if ls "${output_dir}/*.yaml" 1> /dev/null 2>&1; then
    rm "${output_dir}/*.yaml"
  fi

  controller-gen crd paths="${package_path}" output:crd:dir="${output_dir}" output:stdout
}

function generate_crd_without_cel_expressions() {
  local output_dir="${API_GO_MODULE_ROOT}/core/crds"
  local source_file="${output_dir}/druid.gardener.cloud_etcds.yaml"
  local target_file="${output_dir}/druid.gardener.cloud_etcds_without_cel.yaml"
  yq 'del(.. | select(has("x-kubernetes-validations")).x-kubernetes-validations)' "${source_file}" > "${target_file}"
}

function getMd5Sum() {
  if [[ $# -ne 1 ]]; then
    echo -e "${FUNCNAME[0]} requires 1 argument: file for which MD5 sum is to be calculated"
    exit 1
  fi
  local file="$1"
  local computed_md5sum=0
  if [[ ! -f ${file} ]]; then
    echo -e "File ${file} not found"
    else
    computed_md5sum=$(md5sum ${file} | awk '{print $1}')
  fi
  echo ${computed_md5sum}
}

function main() {
  echo "> Generate deepcopy and defaulting functions..."
  generate_deepcopy_defaulter

  echo "> Generate clientset for Etcd API..."
  generate_clientset

  lastMd5Sum=$(getMd5Sum "${API_GO_MODULE_ROOT}/core/crds/druid.gardener.cloud_etcds.yaml")
  echo "> Generate CRDs..."
  generate_crds
  latestMd5Sum=$(getMd5Sum "${API_GO_MODULE_ROOT}/core/crds/druid.gardener.cloud_etcds.yaml")
  if [[ ${lastMd5Sum} == ${latestMd5Sum} ]]; then
    echo "CRD file has not changed, skipping generation of CRD without CEL-validations"
    exit 0
  fi
  echo "> Generate CRD without CEL-validations..."
  generate_crd_without_cel_expressions
}

main