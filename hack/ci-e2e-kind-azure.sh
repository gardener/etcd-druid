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
AZURITE_DOMAIN="azurite-service.default:10000"
AZURITE_DOMAIN_LOCAL="127.0.0.1:10000"
AZURE_STORAGE_CONNECTION_STRING="DefaultEndpointsProtocol=http;AccountName=${STORAGE_ACCOUNT};AccountKey=${STORAGE_KEY};BlobEndpoint=http://${AZURITE_DOMAIN}/${STORAGE_ACCOUNT};"

make kind-up

trap '{
  kind export logs "${ARTIFACTS:-/tmp}/etcd-druid-e2e" --name etcd-druid-e2e || true
  make clean-chart-resources
  make kind-down
}' EXIT

kubectl wait --for=condition=ready node --all

# Setup Azure application credentials
export AZURE_APPLICATION_CREDENTIALS="/tmp/azuriteCredentials"
rm -rf "${AZURE_APPLICATION_CREDENTIALS}"
mkdir -p "${AZURE_APPLICATION_CREDENTIALS}"
echo -n "${STORAGE_ACCOUNT}" > "${AZURE_APPLICATION_CREDENTIALS}/storageAccount"
echo -n "${STORAGE_KEY}" > "${AZURE_APPLICATION_CREDENTIALS}/storageKey"
# echo -n "true" > "${AZURE_APPLICATION_CREDENTIALS}/emulatorEnabled"
# echo -n "${AZURITE_DOMAIN_LOCAL}" > "${AZURE_APPLICATION_CREDENTIALS}/domain"

# Deploy Azurite and run end-to-end tests
make deploy-azurite
make STORAGE_ACCOUNT="${STORAGE_ACCOUNT}" \
  STORAGE_KEY="${STORAGE_KEY}" \
  AZURITE_DOMAIN="${AZURITE_DOMAIN}" \
  AZURE_STORAGE_CONNECTION_STRING="${AZURE_STORAGE_CONNECTION_STRING}" \
  PROVIDERS="azure" \
  TEST_ID="$BUCKET_NAME" \
  STEPS="setup,deploy,test" \
  test-e2e
