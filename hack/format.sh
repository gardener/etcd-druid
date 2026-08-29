#!/usr/bin/env bash
#
# SPDX-FileCopyrightText: Contributors to the Gardener project
#
# SPDX-License-Identifier: Apache-2.0

set -o errexit
set -o nounset
set -o pipefail

echo "> Format"

for p in "$@" ; do
  goimports-reviser -rm-unused \
   -imports-order "std,company,project,general,blanked,dotted" \
   -format \
   -recursive $p
done
