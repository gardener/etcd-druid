#!/usr/bin/env bash
#
# Copyright (c) 2024 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

# Constants for Azurite credentials and configurations
STORAGE_ACCOUNT="devstoreaccount1"
STORAGE_KEY="Eby8vdM02xNOcqFlqUwJPLlmEtlCDXJ1OUzFT50uSRZ6IFsuFq2UVErCz4I6tq/K1SZFPTOtr/KBHBeksoGMGw=="
AZURITE_ENDPOINT="http://localhost:10000"
AZURITE_HOST="azurite-service.default:10000"
AZURE_STORAGE_CONNECTION_STRING="DefaultEndpointsProtocol=http;AccountName=${STORAGE_ACCOUNT};AccountKey=${STORAGE_KEY};BlobEndpoint=http://${AZURITE_HOST}/${STORAGE_ACCOUNT};"

make kind-up

trap "
  ( make kind-down )
" EXIT

kubectl wait --for=condition=ready node --all

# Setup Azure application credentials
export AZURE_APPLICATION_CREDENTIALS="/tmp/azuriteCredentials"
mkdir -p "${AZURE_APPLICATION_CREDENTIALS}"
echo -n "${STORAGE_ACCOUNT}" > "${AZURE_APPLICATION_CREDENTIALS}/storageAccount"
echo -n "${STORAGE_KEY}" > "${AZURE_APPLICATION_CREDENTIALS}/storageKey"

# Deploy Azurite and run end-to-end tests
make deploy-azurite
make STORAGE_ACCOUNT="${STORAGE_ACCOUNT}" \
  STORAGE_KEY="${STORAGE_KEY}" \
  AZURE_STORAGE_API_ENDPOINT="${AZURITE_ENDPOINT}" \
  EMULATOR_ENABLED="true" \
  AZURITE_HOST="${AZURITE_HOST}" \
  AZURE_STORAGE_CONNECTION_STRING="${AZURE_STORAGE_CONNECTION_STRING}" \
  PROVIDERS="azure" \
  TEST_ID="$BUCKET_NAME" \
  STEPS="setup,deploy,test" \
  test-e2e
