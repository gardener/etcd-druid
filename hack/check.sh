#!/usr/bin/env bash
#
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

set -e

GOLANGCI_LINT_CONFIG_FILE=""

for arg in "$@"; do
  case $arg in
    --golangci-lint-config=*)
    GOLANGCI_LINT_CONFIG_FILE="-c ${arg#*=}"
    shift
    ;;
  esac
done

echo "> Check"

echo "Executing golangci-lint"
golangci-lint run $GOLANGCI_LINT_CONFIG_FILE --timeout 10m "$@"

echo "Executing gofmt/goimports"
folders=()
for f in "$@"; do
  folders+=( "$(echo $f | sed 's/\.\.\.//')" )
done
unformatted_files="$(goimports -l ${folders[*]})"
if [[ "$unformatted_files" ]]; then
  echo "Unformatted files detected:"
  echo "$unformatted_files"
  exit 1
fi

echo "Checking Go version"
while IFS= read -r line
do
  if [[ $line =~ ^go.*$ ]]; then
    if [[ $line =~ ^go\ [0-9]+\.[0-9]+\.0$ ]]; then
        # Go version is valid, adheres to x.y.0 version
        exit 0
    else
        echo "Go version is invalid, please adhere to x.y.0 version"
        echo "See https://github.com/gardener/etcd-druid/pull/925"
        exit 1
    fi
  fi
done < "go.mod"

echo "All checks successful"
