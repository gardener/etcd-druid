#!/usr/bin/env bash
# 
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0
set -xe

if [[ -n "${LOCALSTACK_HOST}" ]]; then
  export AWS_ENDPOINT_URL_S3="http://${LOCALSTACK_HOST}"
fi

# More information at https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html
function setup_aws() {
  if $(which aws > /dev/null); then
    return
  fi
  echo "Installing awscli..."
  apt update
  apt install -y curl
  apt install -y unzip
  cd $HOME
  curl -Lo "awscliv2.zip" "https://awscli.amazonaws.com/awscli-exe-$(uname -s | tr '[:upper:]' '[:lower:]')-$(uname -m).zip"
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
    aws s3api create-bucket --bucket ${TEST_ID} --region ${AWS_REGION} --create-bucket-configuration LocationConstraint=${AWS_REGION} --acl private
    # Block public access to the S3 bucket
    aws s3api put-public-access-block --bucket ${TEST_ID} --public-access-block-configuration "BlockPublicAcls=true,IgnorePublicAcls=true,BlockPublicPolicy=true,RestrictPublicBuckets=true"
    # Deny non-HTTPS requests to the S3 bucket, except for localstack which is exposed on an HTTP endpoint
    if [[ -z "${LOCALSTACK_HOST}" ]]; then
      aws s3api put-bucket-policy --bucket ${TEST_ID} --policy "{\"Version\":\"2012-10-17\",\"Statement\":[{\"Effect\":\"Deny\",\"Principal\":\"*\",\"Action\":\"s3:*\",\"Resource\":[\"arn:aws:s3:::${TEST_ID}\",\"arn:aws:s3:::${TEST_ID}/*\"],\"Condition\":{\"Bool\":{\"aws:SecureTransport\":\"false\"},\"NumericLessThan\":{\"s3:TlsVersion\":\"1.2\"}}}]}"
    fi
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
