#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

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

trap '{
  kind export logs "${ARTIFACTS:-/tmp}/etcd-druid-e2e" --name etcd-druid-e2e || true
  make kind-down
}' EXIT

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
  AZURE_EMULATOR_ENABLED="true" \
  AZURITE_HOST="${AZURITE_HOST}" \
  AZURE_STORAGE_CONNECTION_STRING="${AZURE_STORAGE_CONNECTION_STRING}" \
  PROVIDERS="azure" \
  TEST_ID="$BUCKET_NAME" \
  STEPS="setup,deploy,test" \
  test-e2e
