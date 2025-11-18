# SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

TOOLS_BIN_DIR				:= $(DRUIDCTL_MODULE_TOOLS_DIR)/bin
GOLANGCI_LINT				:= $(TOOLS_BIN_DIR)/golangci-lint
GOIMPORTS					:= $(TOOLS_BIN_DIR)/goimports
GOIMPORTS_REVISER			:= $(TOOLS_BIN_DIR)/goimports-reviser

# Tool versions
GOLANGCI_LINT_VERSION ?= v1.64.8
GOIMPORTS_REVISER_VERSION ?= v3.9.1
GOIMPORTS_VERSION ?= latest

export TOOLS_BIN_DIR := $(TOOLS_BIN_DIR)
export PATH := $(abspath $(TOOLS_BIN_DIR)):$(PATH)

.PHONY: clean-tools-bin tools
clean-tools-bin:
	rm -rf $(TOOLS_BIN_DIR)/*

tools: $(GOLANGCI_LINT) $(GOIMPORTS) $(GOIMPORTS_REVISER)

$(GOLANGCI_LINT):
	@# CGO_ENABLED has to be set to 1 in order for golangci-lint to be able to load plugins
	@# see https://github.com/golangci/golangci-lint/issues/1276
	GOBIN=$(abspath $(TOOLS_BIN_DIR)) CGO_ENABLED=1 go install github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

$(GOIMPORTS):
	@# Install goimports without mutating this module's go.mod to avoid tidy/build race.
	GOBIN=$(abspath $(TOOLS_BIN_DIR)) go install golang.org/x/tools/cmd/goimports@$(GOIMPORTS_VERSION)

$(GOIMPORTS_REVISER):
	GOBIN=$(abspath $(TOOLS_BIN_DIR)) go install github.com/incu6us/goimports-reviser/v3@$(GOIMPORTS_REVISER_VERSION)
