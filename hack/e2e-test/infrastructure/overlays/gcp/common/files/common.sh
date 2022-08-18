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

function setup_gcloud() {
  if $(which gcloud > /dev/null); then
    return
  fi
  echo "Installing gcloud..."
  cd $HOME
  apt update > /dev/null
  apt install -y curl > /dev/null
  apt install -y python > /dev/null
  curl -Lo "google-cloud-sdk.tar.gz" https://dl.google.com/dl/cloudsdk/channels/rapid/downloads/google-cloud-sdk-346.0.0-$(uname -s | tr '[:upper:]' '[:lower:]')-$(uname -m | sed 's/aarch64/arm/').tar.gz
  tar -xzf google-cloud-sdk.tar.gz
  ./google-cloud-sdk/install.sh -q
  export PATH=$PATH:${HOME}/google-cloud-sdk/bin
  cd "${SOURCE_PATH}"
  echo "Successfully installed gcloud."
}

function configure_gcloud() {
  echo "Configuring gcloud..."
  gcloud auth activate-service-account --key-file "$GCP_SERVICEACCOUNT_JSON_PATH" --project "$GCP_PROJECT_ID"
  echo "Successfully configured gcloud."
}

function create_gcs_bucket() {
  result=$(gsutil list gs://${TEST_ID} 2>&1 || true)
  if [[ $result  == *"404"* ]]; then
    echo "Creating GCS bucket ${TEST_ID} ..."
    gsutil mb -b on gs://${TEST_ID}
    echo "Successfully created GCS bucket ${TEST_ID} ."
  else
    if [[ $result =~ ${TEST_ID} ]] || [[ $result == "" ]]; then
      echo "GCS bucket already exists, nothing todo."
      exit 0
    fi
    echo $result
    exit 1
  fi
}

function delete_gcs_bucket() {
  result=$(gsutil list gs://${TEST_ID} 2>&1 || true)
  if [[ $result  == *"404"* ]]; then
    echo "GCS bucket is already gone."
    return
  fi
  echo "Deleting GCS bucket ${TEST_ID} ..."
  gsutil rm -r gs://"${TEST_ID}"/
  echo "Successfully deleted GCS bucket ${TEST_ID} ."
}