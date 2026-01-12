# SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

DRUIDCTL_MODULE_ROOT        := $(shell dirname "$(realpath $(lastword $(MAKEFILE_LIST)))")
REPO_ROOT                   := $(shell dirname "$(DRUIDCTL_MODULE_ROOT)")
HACK_DIR					:= $(DRUIDCTL_MODULE_ROOT)/hack
DRUIDCTL_MODULE_TOOLS_DIR 	:= $(HACK_DIR)/tools


include $(HACK_DIR)/tools.mk

# Application build variables
GO 				:= go
BIN_DIR 		:= $(DRUIDCTL_MODULE_ROOT)/bin
BIN_NAME 		:= druidctl
PLUGIN_BIN_NAME := kubectl-druid
GO_FILES 		:= $(shell find . -name '*.go' -not -path './vendor/*')
INSTALL_DIR 	:= $(or $(GOBIN),$(shell $(GO) env GOPATH)/bin,$(HOME)/go/bin)
VERSION_FILE 	:= $(DRUIDCTL_MODULE_ROOT)/version

.PHONY: build build-plugin

build:
	@$(MAKE) $(BIN_DIR)/$(BIN_NAME)

build-plugin:
	@$(MAKE) $(BIN_DIR)/$(PLUGIN_BIN_NAME)

$(BIN_DIR)/$(BIN_NAME): $(GO_FILES) go.mod $(VERSION_FILE) $(HACK_DIR)/ld-flags.sh
	@echo "Building druidctl with version information..."
	@mkdir -p $(BIN_DIR)
	@chmod +x $(HACK_DIR)/ld-flags.sh
	$(GO) build \
		-o $@ \
		-ldflags "$$($(HACK_DIR)/ld-flags.sh)" \
		.
	@echo "Built: $@"
	@echo "Version: $$($(BIN_DIR)/$(BIN_NAME) version --short)"

$(BIN_DIR)/$(PLUGIN_BIN_NAME): $(BIN_DIR)/$(BIN_NAME)
	@echo "Creating kubectl plugin..."
	@cp $(BIN_DIR)/$(BIN_NAME) $@
	@echo "Created: $@"

.PHONY: plugin-install
plugin-install: ## Install kubectl-druid plugin
	@$(MAKE) $(BIN_DIR)/$(PLUGIN_BIN_NAME)
	@mkdir -p $(INSTALL_DIR)
	@cp $(BIN_DIR)/$(PLUGIN_BIN_NAME) $(INSTALL_DIR)/$(PLUGIN_BIN_NAME)
	@echo "Installed $(PLUGIN_BIN_NAME) to $(INSTALL_DIR)"
	@echo "Test with: kubectl druid --help"

.PHONY: install
install: ## Install druidctl to $GOPATH/bin
	@$(MAKE) $(BIN_DIR)/$(BIN_NAME)
	@echo "Installing druidctl..."
	@mkdir -p $(shell go env GOPATH)/bin
	@cp $(BIN_DIR)/$(BIN_NAME) $(shell go env GOPATH)/bin/$(BIN_NAME)
	@echo "Installed druidctl to $(shell go env GOPATH)/bin/$(BIN_NAME)"

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

.PHONY: sast
sast: $(GOSEC)
	@echo "> Running gosec for druidctl module"
	@$(GOSEC) -exclude-generated ./...

.PHONY: clean clean-all
clean:
	rm -rf $(BIN_DIR) coverage.out

clean-all: clean clean-tools-bin
