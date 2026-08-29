#!/usr/bin/env bash
# SPDX-FileCopyrightText: Contributors to the Gardener project
#
# SPDX-License-Identifier: Apache-2.0

set -o errexit
set -o nounset
set -o pipefail

kubectl apply -f ./hack/e2e-test/infrastructure/fake-gcs-server/fake-gcs-server.yaml
kubectl rollout status deploy/fake-gcs
kubectl wait --for=condition=ready pod -l app=fake-gcs --timeout=240s