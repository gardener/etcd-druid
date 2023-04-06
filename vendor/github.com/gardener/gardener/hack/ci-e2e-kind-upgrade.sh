#!/usr/bin/env bash
#
# Copyright 2022 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -o nounset
set -o pipefail
set -o errexit

source $(dirname "${0}")/ci-common.sh

VERSION="$(cat VERSION)"
CLUSTER_NAME=""
SEED_NAME=""

# copy_kubeconfig_from_kubeconfig_env_var copies the kubeconfig to apporiate location based on kind setup
function copy_kubeconfig_from_kubeconfig_env_var() {
  case "$SHOOT_FAILURE_TOLERANCE_TYPE" in
  node)
    cp $KUBECONFIG example/provider-local/seed-kind-ha-single-zone/base/kubeconfig
    cp $KUBECONFIG example/gardener-local/kind/ha-single-zone/kubeconfig
    ;;
  zone)
    cp $KUBECONFIG example/provider-local/seed-kind-ha-multi-zone/base/kubeconfig
    cp $KUBECONFIG example/gardener-local/kind/ha-multi-zone/kubeconfig
    ;;
  *)
    cp $KUBECONFIG example/provider-local/seed-kind/base/kubeconfig
    cp $KUBECONFIG example/gardener-local/kind/local/kubeconfig
    ;;
  esac
}

function gardener_up() {
  case "$SHOOT_FAILURE_TOLERANCE_TYPE" in
  node)
    make gardener-ha-single-zone-up
    ;;
  zone)
    make gardener-ha-multi-zone-up
    ;;
  *)
    make gardener-up
    ;;
  esac
}

function gardener_down() {
  case "$SHOOT_FAILURE_TOLERANCE_TYPE" in
  node)
    make gardener-ha-single-zone-down
    ;;
  zone)
    make gardener-ha-multi-zone-down
    ;;
  *)
    make gardener-down
    ;;
  esac
}

function kind_up() {
  case "$SHOOT_FAILURE_TOLERANCE_TYPE" in
  node)
    make kind-ha-single-zone-up
    ;;
  zone)
    make kind-ha-multi-zone-up
    ;;
  *)
    make kind-up
    ;;
  esac
}

function kind_down() {
  case "$SHOOT_FAILURE_TOLERANCE_TYPE" in
  node)
    make kind-ha-single-zone-down
    ;;
  zone)
    make kind-ha-multi-zone-down
    ;;
  *)
    make kind-down
    ;;
  esac
}

function install_previous_release() {
  pushd $GARDENER_RELEASE_DOWNLOAD_PATH/gardener-releases/$GARDENER_PREVIOUS_RELEASE >/dev/null
  copy_kubeconfig_from_kubeconfig_env_var
  gardener_up
  popd >/dev/null
}

function upgrade_to_next_release() {
  # downloads and upgrades to GARDENER_NEXT_RELEASE release if GARDENER_NEXT_RELEASE is not same as version mentioned in VERSION file.
  # if GARDENER_NEXT_RELEASE is same as version mentioned in VERSION file then it is considered as local release and install gardener from local repo.
  if [[ -n $GARDENER_NEXT_RELEASE && $GARDENER_NEXT_RELEASE != $VERSION ]]; then
    # download gardener next release to perform gardener upgrade tests
    $(dirname "${0}")/download_gardener_source_code.sh --gardener-version $GARDENER_NEXT_RELEASE --download-path $GARDENER_RELEASE_DOWNLOAD_PATH/gardener-releases
    export GARDENER_NEXT_VERSION="$(cat $GARDENER_RELEASE_DOWNLOAD_PATH/gardener-releases/$GARDENER_NEXT_RELEASE/VERSION)"
    pushd $GARDENER_RELEASE_DOWNLOAD_PATH/gardener-releases/$GARDENER_NEXT_RELEASE >/dev/null
    copy_kubeconfig_from_kubeconfig_env_var
    gardener_up
    popd >/dev/null
  else
    export GARDENER_NEXT_VERSION=$VERSION
    gardener_up
  fi

}

function set_gardener_upgrade_version_env_variables() {
  if [[ -z "$GARDENER_PREVIOUS_RELEASE" ]]; then
    previous_minor_version=$(echo "$VERSION" | awk -F. '{printf("%s.%d.*\n", $1, $2-1)}')
    # List all the tags that match the previous minor version pattern
    tag_list=$(git tag -l "$previous_minor_version")

    if [ -z "$tag_list" ]; then
      echo "No tags found for the previous minor version ($VERSION) to upgrade Gardener." >&2
      exit 1
    fi
    # Find the most recent tag for the previous minor version
    export GARDENER_PREVIOUS_RELEASE=$(echo "$tag_list" | tail -n 1)
  fi

  if [[ -z "$GARDENER_NEXT_RELEASE" ]]; then
    export GARDENER_NEXT_RELEASE="$VERSION"
  fi
}

function set_cluster_name() {
  case "$SHOOT_FAILURE_TOLERANCE_TYPE" in
  node)
    CLUSTER_NAME="gardener-local-ha-single-zone"
    ;;
  zone)
    CLUSTER_NAME="gardener-local-ha-multi-zone"
    ;;
  *)
    CLUSTER_NAME="gardener-local"
    ;;
  esac
}

function set_seed_name() {
  case "$SHOOT_FAILURE_TOLERANCE_TYPE" in
  node)
    SEED_NAME="local-ha-single-zone"
    ;;
  zone)
    SEED_NAME="local-ha-multi-zone"
    ;;
  *)
    SEED_NAME="local"
    ;;
  esac
}

clamp_mss_to_pmtu
set_gardener_upgrade_version_env_variables
set_cluster_name
set_seed_name

# download gardener previous release to perform gardener upgrade tests
$(dirname "${0}")/download_gardener_source_code.sh --gardener-version $GARDENER_PREVIOUS_RELEASE --download-path $GARDENER_RELEASE_DOWNLOAD_PATH/gardener-releases
export GARDENER_PREVIOUS_VERSION="$(cat $GARDENER_RELEASE_DOWNLOAD_PATH/gardener-releases/$GARDENER_PREVIOUS_RELEASE/VERSION)"

# test setup
kind_up

# export all container logs and events after test execution
trap '{
  rm -rf "$GARDENER_RELEASE_DOWNLOAD_PATH/gardener-releases"
  export_artifacts "$CLUSTER_NAME"
  kind_down
}' EXIT

echo "Installing gardener version '$GARDENER_PREVIOUS_RELEASE'"
install_previous_release

echo "Running gardener pre-upgrade tests"
make test-pre-upgrade GARDENER_PREVIOUS_RELEASE=$GARDENER_PREVIOUS_RELEASE GARDENER_NEXT_RELEASE=$GARDENER_NEXT_RELEASE

echo "Upgrading gardener version '$GARDENER_PREVIOUS_RELEASE' to '$GARDENER_NEXT_RELEASE'"
upgrade_to_next_release

echo "Wait until seed '$SEED_NAME' gets upgraded from version '$GARDENER_PREVIOUS_RELEASE' to '$GARDENER_NEXT_RELEASE'"
kubectl wait seed $SEED_NAME --timeout=5m --for=jsonpath="{.status.gardener.version}=$GARDENER_NEXT_RELEASE"
# TIMEOUT has been increased to 1200 (20 minutes) due to the upgrading of Gardener for seed.
# In a single-zone setup, 2 istio-ingressgateway pods will be running, and it will take 9 minutes to complete the rollout.
# In a multi-zone setup, 6 istio-ingressgateway pods will be running, and it will take 18 minutes to complete the rollout.
TIMEOUT=1200 ./hack/usage/wait-for.sh seed "$SEED_NAME" GardenletReady Bootstrapped SeedSystemComponentsHealthy ExtensionsReady BackupBucketsReady

# The downtime validator considers downtime after 3 consecutive failures, taking a total of 30 seconds.
# Therefore, we're waiting for double that amount of time (60s) to detect if there is any downtime after the upgrade process.
# By waiting for double the amount of time (60 seconds) post-upgrade, the script accounts for the possibility of missing the last 30-second window,
# thus ensuring that any potential downtime after the post-upgrade is detected.
sleep 60

echo "Running gardener post-upgrade tests"
make test-post-upgrade GARDENER_PREVIOUS_RELEASE=$GARDENER_PREVIOUS_RELEASE GARDENER_NEXT_RELEASE=$GARDENER_NEXT_RELEASE

gardener_down
