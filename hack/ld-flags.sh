#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0


set -o errexit
set -o nounset
set -o pipefail

# NOTE: This script should be sourced into other scripts.
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

function build_ld_flags() {
    local package_path="github.com/gardener/etcd-druid/internal"
    local version="$(cat "${PROJECT_ROOT}/VERSION")"
    local program_name="etcd-druid"
    local build_date="$(date '+%Y-%m-%dT%H:%M:%S%z' | sed 's/\([0-9][0-9]\)$/:\1/g')"

  # .dockerignore ignores files that are not relevant for the build. It only copies the relevant source files to the
  # build container. Git, on the other hand will always detect a dirty work tree when building in a container (many deleted files).
  # This command filters out all deleted files that are ignored by .dockerignore and only detects changes to files that are included
  # to determine dirty work tree.
  # Use `git status --porcelain $MODULE_ROOT` to only check the status of the directory
  local tree_state="$([ -z "$(git status --porcelain $PROJECT_ROOT 2>/dev/null | grep -vf <(git ls-files -o --deleted --ignored --exclude-from=${PROJECT_ROOT}/.dockerignore))" ] && echo clean || echo dirty)"

  echo "-X $package_path/version.gitVersion=$version
        -X $package_path/version.gitCommit=$(git rev-parse --verify HEAD)
        -X $package_path/version.gitTreeState=$tree_state
        -X $package_path/version.buildDate=$build_date
        -X $package_path/version.programName=$program_name"
}