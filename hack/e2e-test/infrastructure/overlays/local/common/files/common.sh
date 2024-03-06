#!/usr/bin/env bash
# 
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

MOUNT_PATH="/host-dir-etc"
HOST_PATH="/etc"
BACKUP_BUCKETS_DIR="gardener/local-backupbuckets"

function create_local_bucket() {
  echo "Creating Local bucket ${TEST_ID} at host directory ${HOST_PATH}/${BACKUP_BUCKETS_DIR} ."
  mkdir -p "${MOUNT_PATH}/${BACKUP_BUCKETS_DIR}/${TEST_ID}"
  echo "Successfully created Local bucket ${TEST_ID} at host directory ${HOST_PATH}/${BACKUP_BUCKETS_DIR} ."
}

function delete_local_bucket() {
  echo "About to delete Local bucket ${TEST_ID} from host directory ${HOST_PATH}/${BACKUP_BUCKETS_DIR} ."
  rm -rf "${MOUNT_PATH:?}/${BACKUP_BUCKETS_DIR}/${TEST_ID}"
  echo "Successfully deleted Local bucket ${TEST_ID} from host directory ${HOST_PATH}/${BACKUP_BUCKETS_DIR} ."
}
