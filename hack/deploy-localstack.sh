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
set -o errexit
set -o nounset
set -o pipefail
set -x

kubectl apply -f ./hack/e2e-test/infrastructure/localstack/localstack.yaml

cat <<EOF | kubectl apply -f -
apiVersion: v1
data:
  Corefile: |
    .:53 {
        errors
        health {
           lameduck 5s
        }
        ready
        rewrite name $BUCKET_NAME.localstack.default localstack.default.svc.cluster.local
        
        kubernetes cluster.local in-addr.arpa ip6.arpa {
           pods insecure
           fallthrough in-addr.arpa ip6.arpa
           ttl 30
        }
        prometheus :9153
        forward . /etc/resolv.conf {
           max_concurrent 1000
        }
        cache 30
        loop
        reload
        loadbalance
    }
kind: ConfigMap
metadata:
  name: coredns
  namespace: kube-system
EOF

kubectl delete pods -n kube-system -l k8s-app=kube-dns
kubectl delete pods -n kube-system -l k8s-app=kube-dns
kubectl wait --for=condition=ready pod -n kube-system -l k8s-app=kube-dns --timeout=120s
kubectl wait --for=condition=ready pod -l app=localstack --timeout=240s

if ! grep -q -x "127.0.0.1 localstack.default" /etc/hosts; then
   # Hostname for Localstack 'localstack.default' is missing in /etc/hosts.
   # To access localstack and run e2e tests, you have to extend your /etc/hosts file.
   echo "Adding Locastack host 'localstack.default' in /etc/hosts"
   printf "\n127.0.0.1 localstack.default\n" >>/etc/hosts
fi
