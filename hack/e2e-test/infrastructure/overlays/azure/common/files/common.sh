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

function setup_azcli() {
  if $(which az > /dev/null); then
    return
  fi
  echo "Installing azure-cli..."
  pip3 install azure-cli
  echo "Successfully installed azure-cli."
}

function create_azure_bucket() {
  echo "Creating ABS bucket ${TEST_ID} in storage account ${STORAGE_ACCOUNT} ..."
  az storage container create --account-name "${STORAGE_ACCOUNT}" --account-key "${STORAGE_KEY}" --name "${TEST_ID}"
  echo "Successfully created ABS bucket ${TEST_ID} in storage account ${STORAGE_ACCOUNT} ."
}

function delete_azure_bucket() {
  echo "Deleting ABS bucket ${TEST_ID} from storage acount ${STORAGE_ACCOUNT} ..."
  az storage container delete --account-name "${STORAGE_ACCOUNT}" --account-key "${STORAGE_KEY}" --name "${TEST_ID}"
  echo "Successfully deleted ABS bucket ${TEST_ID} from storage acount ${STORAGE_ACCOUNT} ."
}