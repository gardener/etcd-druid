# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

TOOLS_BIN_DIR              := $(TOOLS_DIR)/bin
SKAFFOLD                   := $(TOOLS_BIN_DIR)/skaffold
KUSTOMIZE                  := $(TOOLS_BIN_DIR)/kustomize
GOTEST                     := $(TOOLS_BIN_DIR)/gotest

# default tool versions
SKAFFOLD_VERSION ?= v1.38.0
KUSTOMIZE_VERSION ?= v4.5.7
GOTEST_VERSION ?= v0.0.6

export TOOLS_BIN_DIR := $(TOOLS_BIN_DIR)
export PATH := $(abspath $(TOOLS_BIN_DIR)):$(PATH)

#########################################
# Tools                                 #
#########################################

$(KUSTOMIZE):
	@test -s $(TOOLS_BIN_DIR)/kustomize || GOBIN=$(abspath $(TOOLS_BIN_DIR)) go install sigs.k8s.io/kustomize/kustomize/v5@${KUSTOMIZE_VERSION}

$(GOTEST): $(call tool_version_file,$(GOTEST),$(GOTEST_VERSION))
	go build -o $(GOTEST) github.com/rakyll/gotest
