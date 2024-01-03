#!/usr/bin/env bash
#
# Copyright (c) 2023 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

make kind-up

trap "
  ( make kind-down )
" EXIT

kubectl wait --for=condition=ready node --all
export KUBECONFIG=$KUBECONFIG_PATH
export USE_ETCD_DRUID_FEATURE_GATES=true
echo "{ \"serviceaccount.json\": \"\", \"storageAPIEndpoint\": \"http://fake-gcs.default:8000/storage/v1/\", \"enableGCSEmulator\": \"True\" }" >/tmp/svc_acc.json

export GOOGLE_STORAGE_API_ENDPOINT="http://localhost:8000/storage/v1/"

make deploy-fakegcs
make GCP_SERVICEACCOUNT_JSON_PATH="/tmp/svc_acc.json" \
  GCP_PROJECT_ID="e2e-test" \
  GOOGLE_ENABLE_GCS_EMULATOR="True" \
  GCS_EMULATOR_HOST="fake-gcs.default:8000" \
  PROVIDERS="gcp" \
  TEST_ID="$BUCKET_NAME" \
  STEPS="setup,deploy,test" \
  test-e2e