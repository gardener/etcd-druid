#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

set -o errexit
set -o nounset
set -o pipefail

make kind-up

trap '{
  kind export logs "${ARTIFACTS:-/tmp}/etcd-druid-e2e" --name etcd-druid-e2e || true
  make kind-down
}' EXIT

kubectl wait --for=condition=ready node --all
echo "{ \"serviceaccount.json\": \"\", \"storageAPIEndpoint\": \"http://fake-gcs.default:8000/storage/v1/\", \"emulatorEnabled\": \"true\" }" >/tmp/svc_acc.json

export GOOGLE_STORAGE_API_ENDPOINT="http://localhost:8000/storage/v1/"

make deploy-fakegcs
make GCP_SERVICEACCOUNT_JSON_PATH="/tmp/svc_acc.json" \
  GCP_PROJECT_ID="e2e-test" \
  GOOGLE_EMULATOR_ENABLED="true" \
  GCS_EMULATOR_HOST="fake-gcs.default:8000" \
  PROVIDERS="gcp" \
  TEST_ID="$BUCKET_NAME" \
  STEPS="setup,deploy,test" \
  test-e2e