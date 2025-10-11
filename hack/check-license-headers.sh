#!/usr/bin/env bash
#
# SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

set -o errexit
set -o nounset
set -o pipefail

echo "> Check license headers"

missing_license_header_files="$(addlicense \
  -check \
  -ignore ".git/**" \
  -ignore ".idea/**" \
  -ignore ".vscode/**" \
  -ignore "dev/**" \
  -ignore "**/*.md" \
  -ignore "**/*.toml" \
  -ignore "**/*.html" \
  -ignore "**/*.yaml" \
  -ignore "**/*.yml" \
  -ignore "**/Dockerfile" \
  .)" || true

if [[ "$missing_license_header_files" ]]; then
  echo "Files with no license header detected:"
  echo "$missing_license_header_files"
  echo "Consider running \`make add-license-headers\` to automatically add all missing headers."
  exit 1
fi

echo "All files have license headers, nothing to be done."