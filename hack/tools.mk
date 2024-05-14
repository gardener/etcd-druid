# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

TOOLS_BIN_DIR              := $(TOOLS_DIR)/bin
KUSTOMIZE                  := $(TOOLS_BIN_DIR)/kustomize
GOTESTFMT 	   	 		   := $(TOOLS_BIN_DIR)/gotestfmt
SKAFFOLD                   := $(TOOLS_BIN_DIR)/skaffold

SYSTEM_NAME                := $(shell uname -s | tr '[:upper:]' '[:lower:]')
SYSTEM_ARCH                := $(shell uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')

# default tool versions
KUSTOMIZE_VERSION := v4.5.7
GOTESTFMT_VERSION ?= v2.5.0
SKAFFOLD_VERSION := v2.11.1

export TOOLS_BIN_DIR := $(TOOLS_BIN_DIR)
export PATH := $(abspath $(TOOLS_BIN_DIR)):$(PATH)

#########################################
# Tools                                 #
#########################################

$(KUSTOMIZE):
	@test -s $(TOOLS_BIN_DIR)/kustomize || GOBIN=$(abspath $(TOOLS_BIN_DIR)) go install sigs.k8s.io/kustomize/kustomize/v4@${KUSTOMIZE_VERSION}

$(GOTESTFMT):
	GOBIN=$(abspath $(TOOLS_BIN_DIR)) go install github.com/gotesttools/gotestfmt/v2/cmd/gotestfmt@$(GOTESTFMT_VERSION)