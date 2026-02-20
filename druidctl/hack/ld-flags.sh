#!/usr/bin/env bash
#
# SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

set -o errexit
set -o nounset
set -o pipefail

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
DRUIDCTL_ROOT="$(dirname "$SCRIPT_DIR")"

VERSION_PACKAGE="github.com/gardener/etcd-druid/druidctl/internal/version"

function get_version_from_file() {
    local version_file="${DRUIDCTL_ROOT}/version"
    if [ ! -f "$version_file" ]; then
        echo "v0.0.0-dev"
        return
    fi
    cat "$version_file" | tr -d '[:space:]'
}

function get_git_commit() {
    if ! git rev-parse HEAD >/dev/null 2>&1; then
        echo "unknown"
        return
    fi
    git rev-parse HEAD
}

function get_git_commit_short() {
    if ! git rev-parse --short HEAD >/dev/null 2>&1; then
        echo "unknown"
        return
    fi
    git rev-parse --short HEAD
}

function get_git_tree_state() {
    if ! git rev-parse HEAD >/dev/null 2>&1; then
        echo "unknown"
        return
    fi
    
    if git_status=$(git status --porcelain 2>/dev/null) && [ -z "${git_status}" ]; then
        echo "clean"
    else
        echo "dirty"
    fi
}

function get_build_date() {
    date -u +'%Y-%m-%dT%H:%M:%SZ'
}

# Build version string
# For dev builds (with -dev suffix): v0.0.1-dev+abc1234
# For releases (without -dev): v0.0.1
function get_version() {
    local file_version
    local git_commit_short
    
    file_version=$(get_version_from_file)
    
    if [[ "$file_version" == *"-dev"* ]]; then
        # Development build: append git commit
        git_commit_short=$(get_git_commit_short)
        echo "${file_version}+${git_commit_short}"
    else
        # Release build: use clean version from file
        echo "${file_version}"
    fi
}

function build_ld_flags() {
    local version
    local git_commit
    local git_tree_state
    local build_date

    version=$(get_version)
    git_commit=$(get_git_commit)
    git_tree_state=$(get_git_tree_state)
    build_date=$(get_build_date)

    echo "-s -w \
-X '${VERSION_PACKAGE}.gitVersion=${version}' \
-X '${VERSION_PACKAGE}.gitCommit=${git_commit}' \
-X '${VERSION_PACKAGE}.gitTreeState=${git_tree_state}' \
-X '${VERSION_PACKAGE}.buildDate=${build_date}'"
}

build_ld_flags
