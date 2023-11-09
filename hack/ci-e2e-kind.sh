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