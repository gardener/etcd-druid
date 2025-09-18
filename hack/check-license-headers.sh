#!/usr/bin/env bash
#
# SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

set -o errexit
set -o nounset
set -o pipefail

echo "> Check license headers"

makefile="$1/Makefile"
check_branch="__check"
initialized_git=false
stashed=false
checked_out=false
generated=false

function delete-check-branch {
  current_branch="$(git rev-parse --abbrev-ref HEAD)"
  if [[ "$current_branch" == "$check_branch" ]]; then
    # Try to checkout to main/master or previous branch before deleting
    if git show-ref --verify --quiet refs/heads/main; then
      git checkout -q main || true
    elif git show-ref --verify --quiet refs/heads/master; then
      git checkout -q master || true
    else
      # Try to checkout to previous branch if possible
      git checkout -q - || true
    fi
  fi
  git rev-parse --verify "$check_branch" &>/dev/null && git branch -q -D "$check_branch" || :
}

function cleanup {
  current_branch="$(git rev-parse --abbrev-ref HEAD)"
  if [[ "$generated" == true ]]; then
    if [[ "$current_branch" == "$check_branch" ]]; then
      # Safe to clean only on __check branch
      git reset --hard -q && git clean -qdf || echo "Could not clean changes"
    else
      echo "WARNING: Skipping destructive cleanup since not on $check_branch (currently on $current_branch)"
    fi
  fi
  if [[ "$checked_out" == true ]]; then
    if [[ "$current_branch" == "$check_branch" ]]; then
      if git show-ref --verify --quiet refs/heads/main; then
        git checkout -q main || true
      elif git show-ref --verify --quiet refs/heads/master; then
        git checkout -q master || true
      else
        git checkout -q - || true
      fi
      current_branch="$(git rev-parse --abbrev-ref HEAD)"
    fi
  fi
  if [[ "$stashed" == true ]]; then
    git stash pop -q || echo "Could not pop stash: The stash entry is kept in case you need it again."
  fi
  if [[ "$initialized_git" == true ]]; then
    rm -rf .git || echo "Could not delete git directory"
  fi
  delete-check-branch
}

trap cleanup EXIT SIGINT SIGTERM

if which git &>/dev/null; then
  if ! git rev-parse --git-dir &>/dev/null; then
    initialized_git=true
    git init -q
    git add --all
    git config --global user.name 'Gardener'
    git config --global user.email 'gardener@cloud'
    git commit -q --allow-empty -m 'initial commit'
  fi

  if [[ "$(git rev-parse --abbrev-ref HEAD)" == "$check_branch" ]]; then
    echo "Already on check branch, aborting"
    exit 1
  fi
  delete-check-branch

  if [[ "$(git status -s)" != "" ]]; then
    stashed=true
    git stash --include-untracked -q
    git stash apply -q &>/dev/null
  fi

  checked_out=true
  git checkout -q -b "$check_branch"
  git add --all
  git commit -q --allow-empty -m 'checkpoint'

  old_status="$(git status -s)"

  echo ">> make add-license-headers"
  generated=true
  if ! out=$(make -f "$makefile" add-license-headers 2>&1); then
    echo "Error during calling make add-license-headers: $out"
    exit 1
  fi
  new_status="$(git status -s)"

  if [[ "$old_status" != "$new_status" ]]; then
    echo "make add-license-headers needs to be run:"
    echo "$new_status"
    exit 1
  fi

else
  echo "No git detected, cannot run make check-generate"
fi
exit 0
