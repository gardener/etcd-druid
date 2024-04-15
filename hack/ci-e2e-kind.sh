#!/usr/bin/env bash
#
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0
set -o errexit
set -o nounset
set -o pipefail

trap "
  ( make kind-down )
" EXIT

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
  STEPS="setup,deploy,test" \
  test-e2e