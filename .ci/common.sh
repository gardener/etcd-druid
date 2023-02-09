# Copyright 2023 SAP SE or an SAP affiliate company
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -eo pipefail

VCS="github.com"
ORGANIZATION="gardener"
PROJECT="etcd-druid"
REPOSITORY=${VCS}/${ORGANIZATION}/${PROJECT}

# The `go <cmd>` commands requires to see the target repository to be part of a
# Go workspace. Thus, if we are not yet in a Go workspace, let's create one
# temporarily by using symbolic links.
if [[ -z "$SOURCE_PATH" ]]; then
    echo "Environment variable SOURCE_PATH must be provided"
    exit 1
fi

if [[ "${SOURCE_PATH}" != *"src/${REPOSITORY}" ]]; then
  SOURCE_SYMLINK_PATH="${SOURCE_PATH}/tmp/src/${REPOSITORY}"
  if [[ -d "${SOURCE_PATH}/tmp" ]]; then
    rm -rf "${SOURCE_PATH}/tmp"
  fi
  mkdir -p "${SOURCE_PATH}/tmp/src/${VCS}/${ORGANIZATION}"
  ln -s "${SOURCE_PATH}" "${SOURCE_SYMLINK_PATH}"
  cd "${SOURCE_SYMLINK_PATH}"

  export GOPATH="${SOURCE_PATH}/tmp"
  export GOBIN="${SOURCE_PATH}/tmp/bin"
  export PATH="${GOBIN}:${PATH}"
fi

# Turn colors in this script off by setting the NO_COLOR variable in your
# environment to any value:
#
# $ NO_COLOR=1 test.sh
NO_COLOR=${NO_COLOR:-""}
if [ -z "$NO_COLOR" ]; then
  header=$'\e[1;33m'
  reset=$'\e[0m'
else
  header=''
  reset=''
fi

function header_text {
  echo "$header$*$reset"
}
