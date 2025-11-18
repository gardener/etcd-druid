# SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

DRUIDCTL_MODULE_ROOT        := $(shell dirname "$(realpath $(lastword $(MAKEFILE_LIST)))")
REPO_ROOT                   := $(shell dirname "$(DRUIDCTL_MODULE_ROOT)")
HACK_DIR					:= $(DRUIDCTL_MODULE_ROOT)/hack
DRUIDCTL_MODULE_TOOLS_DIR 	:= $(HACK_DIR)/tools


include $(HACK_DIR)/tools.mk

# Application build variables
GO ?= go
BIN_DIR := $(DRUIDCTL_MODULE_ROOT)/bin
BIN_NAME := druidctl
PLUGIN_BIN_NAME := kubectl-druid
GO_FILES := $(shell find . -name '*.go' -not -path './vendor/*')
INSTALL_DIR ?= $(or $(GOBIN),$(HOME)/.local/bin)

.PHONY: build build-plugin
build: $(BIN_DIR)/$(BIN_NAME) $(BIN_DIR)/$(PLUGIN_BIN_NAME)

build-plugin: $(BIN_DIR)/$(PLUGIN_BIN_NAME)

$(BIN_DIR)/$(BIN_NAME): $(GO_FILES) go.mod
	@mkdir -p $(BIN_DIR)
	$(GO) build -o $@ .

$(BIN_DIR)/$(PLUGIN_BIN_NAME): $(BIN_DIR)/$(BIN_NAME)
	cp $(BIN_DIR)/$(BIN_NAME) $@


.PHONY: plugin-install
plugin-install: $(BIN_DIR)/$(PLUGIN_BIN_NAME)
	@mkdir -p $(INSTALL_DIR)
	cp $(BIN_DIR)/$(PLUGIN_BIN_NAME) $(INSTALL_DIR)/$(PLUGIN_BIN_NAME)
	@echo "Installed $(PLUGIN_BIN_NAME) to $(INSTALL_DIR)"
	@echo "Ensure \"$(INSTALL_DIR)\" is in your PATH. Test with: kubectl druid --help"

.PHONY: tidy
tidy:
	@env GO111MODULE=on go mod tidy

.PHONY: test
test:
	@go test ./...

.PHONY: format
format: $(GOIMPORTS_REVISER)
	@$(REPO_ROOT)/hack/format.sh ./cmd ./internal

.PHONY: check
check: $(GOLANGCI_LINT) $(GOIMPORTS) format
	@$(REPO_ROOT)/hack/check.sh --golangci-lint-config=../.golangci.yaml ./...

.PHONY: clean clean-all
clean:
	rm -rf $(BIN_DIR) coverage.out

clean-all: clean clean-tools-bin
