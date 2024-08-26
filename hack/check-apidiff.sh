#!/usr/bin/env bash
#
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

set -o errexit
set -o nounset
set -o pipefail

# create a temporary workdir to store the api-diff results
workDir=$(mktemp -d)
function cleanup_workdir {
  rm -rf "${workDir}"
}
# clean-up workdir on exit
trap cleanup_workdir EXIT

echo "> Check api-diff"

# PULL_BASE_SHA env variable is set by default in prow presubmit jobs
go-apidiff ${PULL_BASE_SHA:-master} --print-compatible=false --repo-path=. >${workDir}/api-diff-output.txt || true

exported_pkgs=(
  gardener/etcd-druid/api/v1alpha1
)

captureChanges=0

while IFS= read -r line; do
  if [[ $line =~ ^"github.com/gardener/etcd-druid" ]]; then
    captureChanges=0
    for x in ${exported_pkgs[*]}; do
      if [[ $line =~ $x ]]; then
        echo "$line" >>"${workDir}/api-diff-result.txt"
        captureChanges=1
      fi
    done
  else
    if [[ $captureChanges -eq 1 ]]; then
      echo "$line" >>"${workDir}/api-diff-result.txt"
    fi
  fi
done <"${workDir}/api-diff-output.txt"

# if there are incompatible changes detected, then fail the check and print out the message along with the changes
if [[ -s "${workDir}/api-diff-result.txt" ]]; then
  echo >&2 "FAIL: Found incompatible API changes"
  cat "${workDir}/api-diff-result.txt"

  cat <<EOF
  API-DIFF check has failed.
  This check is optional and is not a must to pass before merging a PR.

  API-DIFF check highlights that a PR introduces incompatible changes to the API. This allows
  reviewers to measure the impact of the changes on the consumers of the API. Consumers rely on
  stable API and thus an informed decision must be taken before merging the PR.
  If an incompatibility in the API is unavoidable then ensure that the PR has an appropriate release
  note and the consumers are informed about the changes.
  Event better: consider documenting how to adapt to the breaking changes.
EOF

fi