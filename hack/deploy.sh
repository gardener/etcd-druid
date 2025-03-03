#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0


function get_k8s_version() {
    string_version=$(kubectl version 2>/dev/null | grep Server)
    version=${string_version:17:4}
    # int_version=$(echo ${version} | tr -d ".")
    # echo ${int_version}
    # if [[ "int_version" -ge 129 ]] ; then
    #     return 0
    # else
    #     return 1
    # fi
    echo ${version}
}

function prepare_helm_charts() {
    namespace="$1"
    cert_expiry_days="$2"
    hack_dir="$3"

    echo "Preparing Helm charts..."
    cd "$hack_dir" && ./prepare-chart-resources.sh "$namespace" "$cert_expiry_days" "$4"
    if [[ $? -ne 0 ]]; then
        echo "Error: Failed to prepare Helm charts."
        exit 1
    fi
    cd ..
}


echo "Fetching Kubernetes server version..."
KUBE_VERSION=$(get_k8s_version)
echo "Kubernetes server version: $KUBE_VERSION"

echo "Preparing Helm charts..."
prepare_helm_charts $5 $4 $6 "$KUBE_VERSION"

echo "Deploying..."
# $1 run -m etcd-druid -n "$5" --tag "$3" --set "gitSha=7"

SKAFFOLD=$1
VERSION=$3
NAMESPACE=$5
GIT_SHA=$7

VERSION=$VERSION GIT_SHA=$GIT_SHA $SKAFFOLD run -m etcd-druid -n $NAMESPACE