# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

TOOLS_BIN_DIR              := $(TOOLS_DIR)/bin
SKAFFOLD                   := $(TOOLS_BIN_DIR)/skaffold
KUSTOMIZE                  := $(TOOLS_BIN_DIR)/kustomize
GOTESTFMT 	   	 		   := $(TOOLS_BIN_DIR)/gotestfmt

# default tool versions
SKAFFOLD_VERSION ?= v1.38.0
KUSTOMIZE_VERSION ?= v5.3.0
GOTESTFMT_VERSION ?= v2.5.0

export TOOLS_BIN_DIR := $(TOOLS_BIN_DIR)
export PATH := $(abspath $(TOOLS_BIN_DIR)):$(PATH)

#########################################
# Tools                                 #
#########################################

$(KUSTOMIZE):
	@test -s $(TOOLS_BIN_DIR)/kustomize || GOBIN=$(abspath $(TOOLS_BIN_DIR)) go install sigs.k8s.io/kustomize/kustomize/v5@${KUSTOMIZE_VERSION}

$(GOTESTFMT):
	GOBIN=$(abspath $(TOOLS_BIN_DIR)) go install github.com/gotesttools/gotestfmt/v2/cmd/gotestfmt@$(GOTESTFMT_VERSION)
