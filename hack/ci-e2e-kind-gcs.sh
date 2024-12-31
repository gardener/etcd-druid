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
echo -n "http://localhost:8000/storage/v1/" > /tmp/storageAPIEndpoint

# Deploy fake-gcs and run e2e tests
make deploy-fakegcs
make GCP_SERVICEACCOUNT_JSON_PATH="/tmp/svc_acc.json" \
  GOOGLE_APPLICATION_CREDENTIALS="/tmp/svc_acc.json" \
  GCP_PROJECT_ID="e2e-test" \
  FAKEGCS_HOST="fake-gcs.default:8000" \
  PROVIDERS="gcp" \
  TEST_ID="$BUCKET_NAME" \
  STEPS="setup,deploy,test" \
  test-e2e