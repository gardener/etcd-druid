// Copyright (c) 2018 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package controllers

const (
	scriptWithTLS = `#!/bin/sh
VALIDATION_MARKER=/var/etcd/data/validation_marker
# Add self-signed CA to list of root CA-certificates
cat /var/etcd/ssl/ca/ca.crt >> /etc/ssl/certs/ca-certificates.crt
if [ $? -ne 0 ]
then
  echo "failed to update root certificate list"
  exit 1
fi

trap_and_propagate() {
    PID=$1
    shift
    for sig in "$@" ; do
        trap "kill -$sig $PID" "$sig"
    done
}`

	scriptWithoutTLS = `#!/bin/sh
VALIDATION_MARKER=/var/etcd/data/validation_marker

trap_and_propagate() {
    PID=$1
    shift
    for sig in "$@" ; do
        trap "kill -$sig $PID" "$sig"
    done
}`

	startManagedEtcd = `

start_managed_etcd(){
      rm -rf $VALIDATION_MARKER
      etcd --config-file /var/etcd/config/etcd.conf.yaml &
      ETCDPID=$!
      trap_and_propagate $ETCDPID INT TERM
      wait $ETCDPID
      RET=$?
      echo $RET > $VALIDATION_MARKER
      exit $RET
}`
	checkAndStartEtcdWithoutTLS = `

check_and_start_etcd(){
      while true;
      do
        wget "http://%[1]s-local:%[2]d/initialization/status" -S -O status;
        STATUS=$(cat status);
        case $STATUS in
        "New")
              wget "http://%[1]s-local:%[2]d/initialization/start?mode=$1" -S -O - ;;
        "Progress")
              continue;;
        "Failed")
              continue;;
        "Successful")
              echo "Bootstrap preprocessing end time: $(date)"
              start_managed_etcd
              break
              ;;
        esac;
        sleep 1;
      done
}`

	checkAndStartEtcdWithTLS = `

check_and_start_etcd(){
      while true;
      do
        wget "https://%[1]s-local:%[2]d/initialization/status" -S -O status;
        STATUS=$(cat status);
        case $STATUS in
        "New")
              wget "https://%[1]s-local:%[2]d/initialization/start?mode=$1" -S -O - ;;
        "Progress")
              continue;;
        "Failed")
              continue;;
        "Successful")
              echo "Bootstrap preprocessing end time: $(date)"
              start_managed_etcd
              break
              ;;
        esac;
        sleep 1;
      done
}`

	bootstrapWithValidation = `

echo "Bootstrap preprocessing start time: $(date)"
if [ ! -f $VALIDATION_MARKER ] ;
then
      echo "No $VALIDATION_MARKER file. Perform complete initialization routine and start etcd."
      check_and_start_etcd full
else
      echo "$VALIDATION_MARKER file present. Check return status and decide on initialization"
      run_status=$(cat $VALIDATION_MARKER)
      echo "$VALIDATION_MARKER content: $run_status"
      if [ $run_status = '143' ] || [ $run_status = '130' ] || [ $run_status = '0' ] ; then
            echo "Requesting sidecar to perform sanity validation"
            check_and_start_etcd sanity
      else
            echo "Requesting sidecar to perform full validation"
            check_and_start_etcd full
      fi
fi`
)

var (
	bootstrapScriptWithoutTLS = scriptWithoutTLS + startManagedEtcd + checkAndStartEtcdWithoutTLS + bootstrapWithValidation
	bootstrapScriptWithTLS    = scriptWithTLS + startManagedEtcd + checkAndStartEtcdWithTLS + bootstrapWithValidation
)
