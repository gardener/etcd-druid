#
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

# More information at https://learn.microsoft.com/en-us/cli/azure/install-azure-cli-linux?pivots=apt
function setup_azcli() {
  if $(which az > /dev/null); then
    return
  fi
  echo "Installing azure-cli..."
  apt update > /dev/null
  apt install -y curl > /dev/null
  curl -sL https://aka.ms/InstallAzureCLIDeb | bash
  echo "Successfully installed azure-cli."
}

function create_azure_bucket() {
  echo "Creating ABS bucket ${TEST_ID} in storage account ${STORAGE_ACCOUNT} ..."
  az storage container create --account-name "${STORAGE_ACCOUNT}" --account-key "${STORAGE_KEY}" --name "${TEST_ID}"
  echo "Successfully created ABS bucket ${TEST_ID} in storage account ${STORAGE_ACCOUNT} ."
}

function delete_azure_bucket() {
  echo "Deleting ABS bucket ${TEST_ID} from storage account ${STORAGE_ACCOUNT} ..."
  az storage container delete --account-name "${STORAGE_ACCOUNT}" --account-key "${STORAGE_KEY}" --name "${TEST_ID}"
  echo "Successfully deleted ABS bucket ${TEST_ID} from storage account ${STORAGE_ACCOUNT} ."
}
