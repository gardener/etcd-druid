#!/bin/bash
#
# Copyright (c) 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

set -e

echo "> Clean"

for source_tree in $@; do
  find "$(dirname "$source_tree")" -type f -name "zz_*.go" -exec rm '{}' \;
  find "$(dirname "$source_tree")" -type f -name "generated.proto" -exec rm '{}' \;
  find "$(dirname "$source_tree")" -type f -name "generated.pb.go" -exec rm '{}' \;
  find "$(dirname "$source_tree")" -type f -name "openapi_generated.go" -exec rm '{}' \;
  grep -lr '// Code generated by MockGen. DO NOT EDIT' "$(dirname "$source_tree")" | xargs rm -f
  grep -lr '// Code generated by client-gen. DO NOT EDIT' "$(dirname "$source_tree")" | xargs rm -f
  grep -lr '// Code generated by informer-gen. DO NOT EDIT' "$(dirname "$source_tree")" | xargs rm -f
  grep -lr '// Code generated by lister-gen. DO NOT EDIT' "$(dirname "$source_tree")" | xargs rm -f
  grep -lr --include="*.go" "//go:generate packr2" "$(dirname "$source_tree")" | xargs -I {} packr2 clean "{}/.."
done

if [ -d "$PWD/hack/api-reference" ]; then
  find ./hack/api-reference/ -type f -name "*.md" -exec rm '{}' \;
fi
