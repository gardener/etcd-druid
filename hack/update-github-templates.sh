#!/bin/bash
#
# SPDX-FileCopyrightText: 2020 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

set -o errexit
set -o nounset
set -o pipefail

mkdir -p "$(dirname $0)/../.github" "$(dirname $0)/../.github/ISSUE_TEMPLATE"

for file in `find "$(dirname $0)"/../vendor/github.com/gardener/gardener/.github -name '*.md'`; do
  cat "$file" |\
    sed 's/operating Gardener/working with etcd-druid/g' |\
    sed 's/to the Gardener project/for etcd-druid/g' |\
    sed 's/to Gardener/to etcd-druid/g' |\
    sed 's/- Gardener version:/- Gardener version (if relevant):\n- etcd-druid version:/g' \
  > "$(dirname $0)/../.github/${file#*.github/}"
done