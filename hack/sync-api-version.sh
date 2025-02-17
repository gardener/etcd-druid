#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

set -o errexit
set -o nounset
set -o pipefail

VERSION="$(cat "$(dirname $0)/../VERSION")"

echo "Setting etcd-druid API dependency version to etcd-druid current version..."
go get github.com/gardener/etcd-druid/api@${VERSION}
