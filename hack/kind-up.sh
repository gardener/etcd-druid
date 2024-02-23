#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0


set -o errexit
set -o nounset
set -o pipefail

source $(dirname "${0}")/../vendor/github.com/gardener/gardener/hack/ci-common.sh

clamp_mss_to_pmtu

kind create cluster --name etcd-druid-e2e --config hack/e2e-test/infrastructure/kind/cluster.yaml
