#
# Copyright (c) 2022 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

function setup_aws() {
  if $(which aws > /dev/null); then
    return
  fi
  echo "Installing awscli..."
  apt update > /dev/null
  apt install -y curl > /dev/null
  apt install -y unzip > /dev/null
  cd $HOME
  curl -Lo "awscliv2.zip" "https://awscli.amazonaws.com/awscli-exe-$(uname -s | tr '[:upper:]' '[:lower:]')-$(uname -m | sed 's/x86_64/amd64/').zip"
  unzip awscliv2.zip > /dev/null
  ./aws/install -i /usr/local/aws-cli -b /usr/local/bin
  echo "Successfully installed awscli."
}

function configure_aws() {
  echo "Creating aws credentials for API access..."
  mkdir ${HOME}/.aws
  echo "[default]
aws_access_key_id = ${AWS_ACCESS_KEY_ID}
aws_secret_access_key = ${AWS_SECRET_ACCESS_KEY}" > ${HOME}/.aws/credentials
  echo "[default]
region = ${AWS_REGION}" > ${HOME}/.aws/config
}

function create_s3_bucket() {
  result=$(aws s3api get-bucket-location --bucket ${TEST_ID} 2>&1 || true)
  if [[ $result == *NoSuchBucket* ]]; then
    echo "Creating S3 bucket ${TEST_ID} in region ${AWS_REGION}"
    aws s3api create-bucket --bucket ${TEST_ID} --region ${AWS_REGION} --create-bucket-configuration LocationConstraint=${AWS_REGION}
  else
    echo $result
    if [[ $result != *${AWS_REGION}* ]]; then
      exit 1
    fi
  fi
}

function delete_s3_bucket() {
  echo "About to delete S3 bucket ${TEST_ID}"
  result=$(aws s3api get-bucket-location --bucket ${TEST_ID} 2>&1 || true)
  if [[ $result == *NoSuchBucket* ]]; then
    echo "Bucket is already gone."
    return
  fi
  aws s3 rb s3://${TEST_ID} --force
}