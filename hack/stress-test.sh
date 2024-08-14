#!/usr/bin/env bash
#
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

set -o errexit
set -o nounset
set -o pipefail

echo "> Stress test"


PACKAGE=""
FUNC=""
PARAMS=""

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
TEST_WORK_DIR="${SCRIPT_DIR}/tools/stress-work"
mkdir -p "${TEST_WORK_DIR}"

for p in "$@" ; do
  IFS='=' read -r key val <<< "$p"
  case $key in
   test-package)
    PACKAGE="$val"
      ;;
   test-func)
    FUNC="$val"
    ;;
   tool-params)
     PARAMS="$val"
    ;;
  esac
done

STRESS_TEST_FILE="${TEST_WORK_DIR}/pkg-stress.test"

# compile test binary
rm -f "${STRESS_TEST_FILE}"
go test -c "${PACKAGE}" -o "${STRESS_TEST_FILE}"
chmod +x "${STRESS_TEST_FILE}"

# run the stress tool
stress ${PARAMS} ${STRESS_TEST_FILE} -test.run=${FUNC}