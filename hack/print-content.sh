#!/bin/bash
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

cat docs/proposals/multi-node/README.md | gh-md-toc - | sed 's/\*/-/g' | sed 's/   /  /g' | sed 's/^  //'
