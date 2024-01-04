#!/usr/bin/env bash
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


set -e

echo "> Adding Apache License header to all go files where it is not present"

# Uses the tool https://github.com/google/addlicense
YEAR=$1
if [[ -z "$1" ]]; then
  cat << EOF
Unspecified 'YEAR' argument.
Usage: addlicenceheaders.sh <YEAR>
EOF
  exit 1
fi

addlicense -v -c "SAP SE or an SAP affiliate company" -y "${YEAR}" -l apache -ignore "**/*.md" -ignore "**/*.yaml" -ignore "**/Dockerfile" .
