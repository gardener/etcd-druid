#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

# Wraps ci-e2e-kind.sh to deploy etcd-druid with the QuorumAwareUpdatesWithOnDelete feature gate enabled, 
# then runs the full e2e suite. The gate activates the OnDelete-managed rollout path AND the
# TestOnDeleteRollout suite, which is skipped when the gate is off.

set -o errexit
set -o nounset
set -o pipefail

export QUORUM_AWARE_UPDATES_WITH_ONDELETE=true
exec "$(dirname "$0")/ci-e2e-kind.sh" "$@"
