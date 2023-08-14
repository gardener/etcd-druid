#!/usr/bin/env bash
# 
# Copyright (c) 2023 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

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
