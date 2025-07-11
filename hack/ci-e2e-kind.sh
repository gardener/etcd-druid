#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

set -o errexit
set -o nounset
set -o pipefail

export NAMESPACE="etcd-druid-e2e"

if ! kind get clusters | grep -q "^etcd-druid-e2e$"; then
  make kind-up
fi

# if RETAIN_KIND_CLUSTER is set to true, the test artifacts will be retained and the kind cluster will not be deleted
if [[ "${RETAIN_KIND_CLUSTER:-false}" != "true" ]]; then
  trap '{
    kind export logs "${ARTIFACTS:-/tmp}/etcd-druid-e2e" --name etcd-druid-e2e || true
    echo "Cleaning copied and generated helm chart resources"
    make clean-chart-resources
    make kind-down
  }' EXIT
fi

kubectl wait --for=condition=ready node --all

make NAMESPACE=$NAMESPACE \
  CERT_EXPIRY_DAYS=30 \
  prepare-helm-charts
make DRUID_E2E_TEST=true \
  ENABLE_ETCD_COMPONENT_PROTECTION_WEBHOOK=true \
  deploy
make NAMESPACE=$NAMESPACE \
  RETAIN_TEST_ARTIFACTS="$RETAIN_TEST_ARTIFACTS" \
  PROVIDERS="$PROVIDERS" \
  test-e2e
