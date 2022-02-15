#!/usr/bin/env bash
#
# Copyright (c) 2021 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

# Usage:
# generate-seed-crds.sh <file-name-prefix> [<group> ...]
#     Generate manifests for all CRDs that are present on a Seed cluster to the current working directory.
#     Useful for development purposes.
#
#     <file-name-prefix> File name prefix for manifest files (e.g. '10-crd-')
#     <group>            List of groups to generate (generate all if unset)

if ! command -v controller-gen &> /dev/null ; then
  >&2 echo "controller-gen not available"
  exit 1
fi

output_dir="$(pwd)"
file_name_prefix="$1"

get_group_package () {
  case "$1" in
  "extensions.gardener.cloud")
    echo "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
    ;;
  "resources.gardener.cloud")
    echo "github.com/gardener/gardener/pkg/apis/resources/v1alpha1"
    ;;
  "druid.gardener.cloud")
    echo "github.com/gardener/etcd-druid/api/v1alpha1"
    ;;
  *)
    >&2 echo "unknown group $1"
    return 1
  esac
}

generate_group () {
  local group="$1"
  echo "Generating CRDs for $group group"

  local package="$(get_group_package "$group")"
  if [ -z "$package" ] ; then
    exit 1
  fi
  local package_path="$(go list -f '{{ .Dir }}' "$package")"
  if [ -z "$package_path" ] ; then
    exit 1
  fi

  # clean all generated files for this group to account for changed prefix or removed resources
  if ls "$output_dir"/*${group}_*.yaml >/dev/null 2>&1; then
    rm "$output_dir"/*${group}_*.yaml
  fi

  controller-gen crd paths="$package_path" output:crd:dir="$output_dir" output:stdout

  while IFS= read -r crd; do
    crd_out="$output_dir/$file_name_prefix$(basename $crd)"
    if [ "$crd" != "$crd_out" ]; then
      mv "$crd" "$crd_out"
    fi
  done < <(ls "$output_dir/${group}"_*.yaml)
}

if [ -n "${2:-}" ]; then
  while [ -n "${2:-}" ] ; do
    generate_group "$2"
    shift
  done
else
  generate_group extensions.gardener.cloud
  generate_group resources.gardener.cloud
  generate_group druid.gardener.cloud
fi
