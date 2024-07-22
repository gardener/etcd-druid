#!/usr/bin/env bash
#
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0
set -e

echo "> Clean"

for source_tree in "$@"; do
  find "$(dirname "$source_tree")" -type f -name "zz_*.go" -exec rm '{}' \;
  grep -lr '// Code generated by MockGen. DO NOT EDIT' "$(dirname "$source_tree")" | xargs rm -f
  grep -lr '// Code generated by client-gen. DO NOT EDIT' "$(dirname "$source_tree")" | xargs rm -f
  grep -lr '// Code generated by informer-gen. DO NOT EDIT' "$(dirname "$source_tree")" | xargs rm -f
  grep -lr '// Code generated by lister-gen. DO NOT EDIT' "$(dirname "$source_tree")" | xargs rm -f
done