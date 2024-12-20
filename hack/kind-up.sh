#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

set -o errexit
set -o nounset
set -o pipefail

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
KIND_CONFIG_DIR="${SCRIPT_DIR}/kind"
CLUSTER_NAME="etcd-druid-e2e"
DEPLOY_REGISTRY=true
FEATURE_GATES=()
USAGE=""


function create_usage() {
  usage=$(printf '%s\n' "
  usage: $(basename $0) [Options]
  Options:
    --cluster-name  <cluster-name>   Name of the kind cluster to create. Default value is 'etcd-druid-e2e'
    --skip-registry                  Skip creating a local docker registry. Default value is false.
    --feature-gates <feature-gates>  Comma separated list of feature gates to enable on the cluster.
  ")
  echo "${usage}"
}

function check_prerequisites() {
  if ! command -v docker &> /dev/null; then
    echo "docker is not installed. Please install docker from https://docs.docker.com/get-docker/"
    exit 1
  fi
  if ! command -v kind &> /dev/null; then
    echo "kind is not installed. Please install kind from https://kind.sigs.k8s.io/docs/user/quick-start/"
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
      --cluster-name)
        shift
        CLUSTER_NAME=$1
        ;;
      --skip-registry)
        DEPLOY_REGISTRY=false
        shift
        ;;
      --feature-gates)
        shift
        IFS=',' read -r -a FEATURE_GATES <<< "$1"
        unset IFS
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

function clamp_mss_to_pmtu() {
  # https://github.com/kubernetes/test-infra/issues/23741
  if [[ "$OSTYPE" != "darwin"* ]]; then
    iptables -t mangle -A POSTROUTING -p tcp --tcp-flags SYN,RST SYN -j TCPMSS --clamp-mss-to-pmtu
  fi
}

function generate_kind_config() {
  echo "Generating kind cluster config..."
  cat >"${KIND_CONFIG_DIR}/cluster-config.yaml" <<EOF
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  image: kindest/node:v1.32.0
  extraPortMappings:
  - containerPort: 4566
    hostPort: 4566
    protocol: TCP
  - containerPort: 10000
    hostPort: 10000
    protocol: TCP
EOF
  if [ "${DEPLOY_REGISTRY}" = true ]; then
    printf -v reg '[plugins."io.containerd.grpc.v1.cri".registry]
      config_path = "/etc/containerd/certs.d"'; reg="$reg" yq -i '.containerdConfigPatches[0] = strenv(reg)' "${KIND_CONFIG_DIR}/cluster-config.yaml"
  fi
  if [ ${#FEATURE_GATES[@]} -gt 0 ]; then
    for key in "${FEATURE_GATES[@]}"; do
      feature_key="$key" yq -i 'with(.featureGates.[strenv(feature_key)]; . = true | key style="double")' "${KIND_CONFIG_DIR}/cluster-config.yaml"
    done
  fi
}

function create_kind_cluster() {
  kind create cluster --name "${CLUSTER_NAME}" --config "${KIND_CONFIG_DIR}/cluster-config.yaml"
}

# NOTE: Container Registry Creation has been taken from https://kind.sigs.k8s.io/docs/user/local-registry/

REG_NAME='kind-registry'
REG_PORT='5001'

function create_local_docker_registry_container() {
  # create registry container unless it already exists
  if [ "$(docker inspect -f '{{.State.Running}}' "${REG_NAME}" 2>/dev/null || true)" != 'true' ]; then
    echo "Creating local docker registry..."
    docker run \
      -d --restart=always -p "127.0.0.1:${REG_PORT}:5000" \
      --network bridge --name "${REG_NAME}" \
      registry:2
  fi
}

function initialize_registry() {
  # Add the registry config to the node(s)
  # This is necessary because localhost resolves to loopback addresses that are
  # network-namespace local.
  # In other words: localhost in the container is not localhost on the host.
  # We want a consistent name that works from both ends, so we tell containerd to
  # alias localhost:${REG_PORT} to the registry container when pulling images.
  echo "Initializing local docker registry..."
  local registry_dir="/etc/containerd/certs.d/localhost:${REG_PORT}"
  for node in $(kind get nodes --name ${CLUSTER_NAME}); do
    docker exec "${node}" mkdir -p "${registry_dir}"
    cat <<EOF | docker exec -i "${node}" cp /dev/stdin "${registry_dir}/hosts.toml"
  [host."http://${REG_NAME}:5000"]
EOF
  done

  # Connect the registry to the cluster network if not already connected
  # This allows kind to bootstrap the network but ensures they're on the same network
  if [ "$(docker inspect -f='{{json .NetworkSettings.Networks.kind}}' "${REG_NAME}")" = 'null' ]; then
    docker network connect "kind" "${REG_NAME}"
  fi
}

function create_local_container_reg_configmap() {
  # Document the local registry
  # https://github.com/kubernetes/enhancements/tree/master/keps/sig-cluster-lifecycle/generic/1755-communicating-a-local-registry
  cat <<EOF | kubectl apply -f -
  apiVersion: v1
  kind: ConfigMap
  metadata:
    name: local-registry-hosting
    namespace: kube-public
  data:
    localRegistryHosting.v1: |
      host: "localhost:${REG_PORT}"
      help: "https://kind.sigs.k8s.io/docs/user/local-registry/"
EOF
}

function main() {
  check_prerequisites
  parse_flags "$@"
  clamp_mss_to_pmtu
  if [ "${DEPLOY_REGISTRY}" = true ]; then
    create_local_docker_registry_container
  fi
  generate_kind_config
  create_kind_cluster
  if [ "${DEPLOY_REGISTRY}" = true ]; then
    initialize_registry
    create_local_container_reg_configmap
  fi
}

USAGE=$(create_usage)
main "$@"