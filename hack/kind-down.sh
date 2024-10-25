#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

set -o errexit
set -o nounset
set -o pipefail

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
KUBECONFIG_PATH="${SCRIPT_DIR}/kind/kubeconfig"
CLUSTER_NAME="etcd-druid-e2e"

function create_usage() {
  usage=$(printf '%s\n' "
  usage: $(basename $0) [Options]
  Options:
    --cluster-name       <cluster-name>   Name of the kind cluster to create. Default value is 'etcd-druid-e2e'
    --kubeconfig-path    <kubeconfig>     Path to the kubeconfig file. Default value is '${KUBECONFIG_PATH}'
  ")
  echo "${usage}"
}

function check_prerequisites() {
  if ! command -v kind &> /dev/null; then
    echo "kind is not installed. Please install kind from https://kind.sigs.k8s.io/docs/user/quick-start/"
    exit 1
  fi
}

function parse_flags() {
  while test $# -gt 0; do
    case "$1" in
      --cluster-name)
        shift
        CLUSTER_NAME=$1
        ;;
      --kubeconfig-path)
        shift
        KUBECONFIG_PATH=$1
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

function delete_kind_cluster() {
	echo "Deleting kind cluster..."
	kind delete cluster --name ${CLUSTER_NAME}
	rm -f ${KUBECONFIG_PATH}
}

function delete_container_registry() {
	local reg_container_name="kind-registry"
	if [ "$(docker ps -qa -f name=${reg_container_name})" ]; then
	  if [ "$(docker ps -q -f name=${reg_container_name})" ]; then
	    echo "Stopping running container $reg_container_name..."
      docker stop "${reg_container_name}" > /dev/null
    fi
    echo "Removing container $reg_container_name..."
    docker rm "${reg_container_name}" > /dev/null
	fi
}

function main() {
  check_prerequisites
  parse_flags "$@"
  delete_kind_cluster
  delete_container_registry
}

USAGE=$(create_usage)
main "$@"

