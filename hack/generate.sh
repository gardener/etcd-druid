#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
TOOLS_BIN_DIR="${SCRIPT_DIR}/tools/bin"

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
    --boilerplate "${SCRIPT_DIR}/boilerplate.go.txt" \
    "${PROJECT_ROOT}/api/core/v1alpha1"
}

function generate_clientset() {
  kube::codegen::gen_client \
    --with-watch \
    --one-input-api "core/v1alpha1" \
    --output-dir "${PROJECT_ROOT}/client" \
    --output-pkg "github.com/gardener/etcd-druid/client" \
    --boilerplate "${SCRIPT_DIR}/boilerplate.go.txt" \
    "${PROJECT_ROOT}/api"
}

function generate_crds() {
  local output_dir="${PROJECT_ROOT}/api/core/crds"
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

function main() {
  #echo "> Generate..."
  #go generate "${PROJECT_ROOT}/..."

  echo "> Generate deepcopy and defaulting functions..."
  generate_deepcopy_defaulter

  echo "> Generate clientset for Etcd API..."
  generate_clientset

  #check_controller_gen_prereq
  #echo "> Generate CRDs..."
  #generate_crds
}

main