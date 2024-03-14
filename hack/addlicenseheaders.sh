#!/usr/bin/env bash
#  SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
#
#  SPDX-License-Identifier: Apache-2.0


set -e

echo "> Adding Apache License header to all go files where it is not present"

YEAR=$1
if [[ -z "$1" ]]; then
  cat << EOF
Unspecified 'YEAR' argument.
Usage: addlicenceheaders.sh <YEAR>
EOF
  exit 1
fi

# addlicence with a license file (parameter -f) expects no comments in the file.
# boilerplate.go.txt is however also used also when generating go code.
# Therefore we remove '//' from boilerplate.go.txt here before passing it to addlicense.

temp_file=$(mktemp)
trap "rm -f $temp_file" EXIT
sed "s/{YEAR}/${YEAR}/g" hack/boilerplate.go.txt > $temp_file

addlicense \
  -f $temp_file \
  -ignore "**/*.md" \
  -ignore "**/*.yaml" \
  -ignore "**/Dockerfile" \
  .