# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

API_MODULE_ROOT       	        := $(shell dirname "$(realpath $(lastword $(MAKEFILE_LIST)))")
REPO_ROOT			            := $(shell dirname "$(API_MODULE_ROOT)")
HACK_DIR		                := $(API_MODULE_ROOT)/hack
API_MODULE_TOOLS_DIR		    := $(HACK_DIR)/tools
REPO_TOOLS_DIR                  := $(REPO_ROOT)/hack/tools


include $(HACK_DIR)/tools.mk

.PHONY: check-generate
check-generate:
	@$(HACK_DIR)/check-generate.sh "$(API_MODULE_ROOT)"

.PHONY: generate-api-docs
generate-api-docs: $(CRD_REF_DOCS)
	@crd-ref-docs --source-path "$(REPO_ROOT)/api" --config "$(HACK_DIR)/api-reference/config.yaml" --output-path "$(REPO_ROOT)/docs/api-reference/etcd-druid-api.md" --renderer markdown

.PHONY: check-apidiff
check-apidiff: $(GO_APIDIFF)
	@$(HACK_DIR)/check-apidiff.sh

.PHONY: generate
generate: $(CONTROLLER_GEN) tidy generate-api-docs
	@$(HACK_DIR)/generate.sh

.PHONY: clean-generated-code
clean-generated-code:
	@$(HACK_DIR)/clean-generated-code.sh

.PHONY: tidy
tidy:
	@env GO111MODULE=on go mod tidy

.PHONY: test-unit
test-unit:
	@go test ./...

.PHONY: format
format: $(GOIMPORTS_REVISER)
	@$(REPO_ROOT)/hack/format.sh ./core ./validation

.PHONY: check
check: $(GOLANGCI_LINT) $(GOIMPORTS) format
	@$(REPO_ROOT)/hack/check.sh --golangci-lint-config=../.golangci.yaml ./...
