#!/usr/bin/env bash
#
# Copyright (c) 2022 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

set -o errexit
set -o nounset
set -o pipefail

function containsElement () {
  array=(${1//,/ })
  for i in "${!array[@]}"
  do
      if [[ "${array[i]}" == "$2" ]]; then
        return 0
      fi
  done
  return 1
}

function skaffold_run_or_deploy {
  if [[ -n ${IMAGE_NAME:=""} ]] && [[ -n ${IMAGE_TEST_TAG:=""} ]]; then
    skaffold deploy --images ${IMAGE_NAME}:${IMAGE_TEST_TAG} $@
  else
    skaffold run $@
  fi
}

function create_namespace {
cat <<EOF | kubectl apply -f -
  apiVersion: v1
  kind: Namespace
  metadata:
    labels:
      gardener.cloud/purpose: druid-e2e-test
    name: $TEST_ID
EOF
}

function delete_namespace {
  kubectl delete namespace $TEST_ID --wait=true --ignore-not-found=true
}

function teardown_trap {
  if [[ ${teardown_done:="false"} != "true" ]]; then
    teardown
  fi
  if [[ ${undeploy_done:="false"} != "true" ]]; then
    undeploy_druid
  fi
  return 1
}

function teardown {
  if containsElement $STEPS "cleanup" && [[ $profile_setup != "" ]]; then
      echo "-------------------"
      echo "Tearing down environment"
      echo "-------------------"

      create_namespace
      skaffold_run_or_deploy -p ${profile_cleanup} -m druid-e2e -n $TEST_ID --status-check=false
      delete_namespace
  fi
  teardown_done="true"
}

function undeploy_druid {
  if containsElement $STEPS "deploy"; then
    skaffold delete -m etcd-druid
  fi
  undeploy_done="true"
}

function run_e2e {
  : ${STEPS:="setup,deploy,test,cleanup"}
  : ${teardown_done:="false"}
  : ${undeploy_done:="false"}
  : ${profile_setup:=""}

  trap teardown_trap INT TERM

  if containsElement $STEPS "setup" && [[ $profile_setup != "" ]]; then
    echo "-------------------"
    echo "Setting up environment"
    echo "-------------------"
    create_namespace
    skaffold_run_or_deploy -p ${profile_setup} -m druid-e2e -n $TEST_ID --status-check=false
  fi

  if containsElement $STEPS "deploy"; then
    if [[ ${deployed:="false"} != "true" ]] || true; then
      echo "-------------------"
      echo "Deploying Druid"
      echo "-------------------"
      skaffold_run_or_deploy -m etcd-druid
      deployed="true"
    fi
  fi

  if containsElement $STEPS "test"; then
    echo "-------------------"
    echo "Running e2e tests"
    echo "-------------------"

    STORAGE_CONTAINER=$TEST_ID \
    SOURCE_PATH=$PWD \
    go test -timeout=0 -mod=vendor ./test/e2e --v -args -ginkgo.v -ginkgo.progress
  fi

  teardown
}

function usage_aws {
    cat <<EOM
Usage:
run-e2e-test.sh aws

Please make sure the following environment variables are set:

    AWS_ACCESS_KEY_ID       Key ID of the user.
    AWS_SECRET_ACCESS_KEY   Access key of the user.
    AWS_REGION              Region in which the test bucket is created.
    TEST_ID                 ID of the test, used for test objects and assets.
EOM
    exit 0
}

function run_aws_e2e {
  ( [[ -z ${AWS_ACCESS_KEY_ID:-""} ]] || [[ -z ${AWS_SECRET_ACCESS_KEY:=""} ]]  || [[ -z ${AWS_REGION:=""} ]] || [[ -z ${TEST_ID:=""} ]] ) && usage_aws

  export INFRA_PROVIDERS="aws"
  profile_setup="aws-setup"
  profile_cleanup="aws-cleanup"
  run_e2e
}

function usage_azure {
    cat <<EOM
Usage:
run-e2e-test.sh azure

Please make sure the following environment variables are set:

    STORAGE_ACCOUNT     Storage account used for managing the storage container.
    STORAGE_KEY         Key of storage account.
    TEST_ID             ID of the test, used for test objects and assets.
EOM
    exit 0
}

function run_azure_e2e {
  ( [[ -z ${STORAGE_ACCOUNT:-""} ]] || [[ -z ${STORAGE_KEY:=""} ]] || [[ -z ${TEST_ID:=""} ]] ) && usage_azure

  export INFRA_PROVIDERS="azure"
  profile_setup="azure-setup"
  profile_cleanup="azure-cleanup"
  run_e2e
}

function usage_gcp {
    cat <<EOM
Usage:
run-e2e-test.sh gcp

Please make sure the following environment variables are set:

    GCP_SERVICEACCOUNT_JSON_PATH      Patch to the service account json file used for this test.
    GCP_PROJECT_ID                    ID of the GCP project.
    TEST_ID                           ID of the test, used for test objects and assets.
EOM
    exit 0
}

function run_gcp_e2e {
  ( [[ -z ${GCP_SERVICEACCOUNT_JSON_PATH:-""} ]] || [[ -z ${GCP_PROJECT_ID:=""} ]]  || [[ -z ${TEST_ID:=""} ]] ) && usage_gcp

  export INFRA_PROVIDERS="gcp"
  export GOOGLE_APPLICATION_CREDENTIALS=$GCP_SERVICEACCOUNT_JSON_PATH
  profile_setup="gcp-setup"
  profile_cleanup="gcp-cleanup"
  run_e2e
}

function usage_local {
    cat <<EOM
Usage:
run-e2e-test.sh local

Please make sure the following environment variables are set:

    TEST_ID             ID of the test, used for test objects and assets.
EOM
    exit 0
}

function run_local_e2e {
  [[ -z ${TEST_ID:=""} ]] && usage_local

  export INFRA_PROVIDERS="local"
  run_e2e
}

for p in ${1//,/ }; do
  case $p in
    all)
      run_aws_e2e
      run_azure_e2e
      run_gcp_e2e
      run_local_e2e
      ;;
    aws)
      run_aws_e2e
      ;;
    azure)
      run_azure_e2e
      ;;
    gcp)
      run_gcp_e2e
      ;;
    local)
      run_local_e2e
      ;;
    *)
      echo "Provider ${1} is not supported."
      ;;
    esac
done
undeploy_druid
