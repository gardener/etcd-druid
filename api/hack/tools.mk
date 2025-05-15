# SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

SYSTEM_NAME                := $(shell uname -s | tr '[:upper:]' '[:lower:]')
SYSTEM_ARCH                := $(shell uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
TOOLS_BIN_DIR              := $(API_MODULE_TOOLS_DIR)/bin
CONTROLLER_GEN             := $(TOOLS_BIN_DIR)/controller-gen
GOLANGCI_LINT              := $(TOOLS_BIN_DIR)/golangci-lint
GOIMPORTS                  := $(TOOLS_BIN_DIR)/goimports
GOIMPORTS_REVISER          := $(TOOLS_BIN_DIR)/goimports-reviser
CRD_REF_DOCS               := $(TOOLS_BIN_DIR)/crd-ref-docs
GO_APIDIFF                 := $(TOOLS_BIN_DIR)/go-apidiff
YQ                         := $(TOOLS_BIN_DIR)/yq

# default tool versions
CONTROLLER_GEN_VERSION ?= $(call version_gomod,sigs.k8s.io/controller-tools)
GOLANGCI_LINT_VERSION ?= v1.64.8
GOIMPORTS_REVISER_VERSION ?= v3.9.1
CRD_REF_DOCS_VERSION ?= v0.1.0
GO_APIDIFF_VERSION ?= v0.8.2
YQ_VERSION ?= v4.45.1

export TOOLS_BIN_DIR := $(TOOLS_BIN_DIR)
export PATH := $(abspath $(TOOLS_BIN_DIR)):$(PATH)

# Use this function to get the version of a go module from go.mod
version_gomod = $(shell go list -f '{{ .Version }}' -m $(1))

.PHONY: clean-tools-bin
clean-tools-bin:
	rm -rf $(TOOLS_BIN_DIR)/*

$(CONTROLLER_GEN):
	GOBIN=$(abspath $(TOOLS_BIN_DIR)) go install sigs.k8s.io/controller-tools/cmd/controller-gen@${CONTROLLER_GEN_VERSION}

$(GOLANGCI_LINT):
	@# CGO_ENABLED has to be set to 1 in order for golangci-lint to be able to load plugins
	@# see https://github.com/golangci/golangci-lint/issues/1276
	GOBIN=$(abspath $(TOOLS_BIN_DIR)) CGO_ENABLED=1 go install github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

$(GOIMPORTS):
	go build -o $(GOIMPORTS) golang.org/x/tools/cmd/goimports

$(GOIMPORTS_REVISER):
	GOBIN=$(abspath $(TOOLS_BIN_DIR)) go install github.com/incu6us/goimports-reviser/v3@$(GOIMPORTS_REVISER_VERSION)

$(CRD_REF_DOCS):
	GOBIN=$(abspath $(TOOLS_BIN_DIR)) go install github.com/elastic/crd-ref-docs@$(CRD_REF_DOCS_VERSION)

$(GO_APIDIFF):
	GOBIN=$(abspath $(TOOLS_BIN_DIR)) go install github.com/joelanford/go-apidiff@$(GO_APIDIFF_VERSION)

$(YQ):
	curl -Lo $(YQ) https://github.com/mikefarah/yq/releases/download/$(YQ_VERSION)/yq_$(SYSTEM_NAME)_$(SYSTEM_ARCH)
	chmod +x $(YQ)
