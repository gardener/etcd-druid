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

make kind-up

trap "
  ( make kind-down )
" EXIT

kubectl wait --for=condition=ready node --all

export AZURE_APPLICATION_CREDENTIALS="/tmp/azuriteCredentials"

mkdir -p /tmp/azuriteCredentials/
echo -n "devstoreaccount1" > /tmp/azuriteCredentials/storageAccount
echo -n "Eby8vdM02xNOcqFlqUwJPLlmEtlCDXJ1OUzFT50uSRZ6IFsuFq2UVErCz4I6tq/K1SZFPTOtr/KBHBeksoGMGw==" > /tmp/azuriteCredentials/storageKey

make deploy-azurite
make STORAGE_ACCOUNT="devstoreaccount1" \
  STORAGE_KEY="Eby8vdM02xNOcqFlqUwJPLlmEtlCDXJ1OUzFT50uSRZ6IFsuFq2UVErCz4I6tq/K1SZFPTOtr/KBHBeksoGMGw==" \
  AZURE_STORAGE_API_ENDPOINT="http://localhost:10000" \
  AZURE_ENABLE_STORAGE_EMULATOR="true" \
  AZURITE_HOST="azurite-service.default:10000" \
  AZURE_STORAGE_CONNECTION_STRING="DefaultEndpointsProtocol=http;AccountName=devstoreaccount1;AccountKey=Eby8vdM02xNOcqFlqUwJPLlmEtlCDXJ1OUzFT50uSRZ6IFsuFq2UVErCz4I6tq/K1SZFPTOtr/KBHBeksoGMGw==;BlobEndpoint=http://azurite-service.default:10000/devstoreaccount1;" \
  PROVIDERS="azure" \
  TEST_ID="$BUCKET_NAME" \
  STEPS="setup,deploy,test" \
  test-e2e
