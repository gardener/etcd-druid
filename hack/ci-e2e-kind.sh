#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

set -o errexit
set -o nounset
set -o pipefail

make kind-up

CERT_EXPIRY=2
. $(dirname $0)/prepare-chart-resources.sh "${BUCKET_NAME}" "${CERT_EXPIRY}"

trap '{
  kind export logs "${ARTIFACTS:-/tmp}/etcd-druid-e2e" --name etcd-druid-e2e || true
  echo "cleaning copied and generated helm chart resources"
  make clean-chart-resources
  make kind-down
}' EXIT

kubectl wait --for=condition=ready node --all
export AWS_APPLICATION_CREDENTIALS_JSON="/tmp/aws.json"
echo "{ \"accessKeyID\": \"ACCESSKEYAWSUSER\", \"secretAccessKey\": \"sEcreTKey\", \"region\": \"us-east-2\", \"endpoint\": \"http://127.0.0.1:4566\", \"s3ForcePathStyle\": true, \"bucketName\": \"${BUCKET_NAME}\" }" >/tmp/aws.json

make deploy-localstack
make LOCALSTACK_HOST="localstack.default:4566" \
  AWS_ACCESS_KEY_ID="ACCESSKEYAWSUSER" \
  AWS_SECRET_ACCESS_KEY="sEcreTKey" \
  AWS_REGION="us-east-2" \
  PROVIDERS="aws" \
  TEST_ID="$BUCKET_NAME" \
  DRUID_E2E_TEST=true \
  DRUID_ENABLE_ETCD_COMPONENTS_WEBHOOK=true \
  STEPS="setup,deploy,test" \
  test-e2e
