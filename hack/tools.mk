# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

SYSTEM_NAME                := $(shell uname -s | tr '[:upper:]' '[:lower:]')
SYSTEM_ARCH                := $(shell uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')
TOOLS_BIN_DIR              := $(TOOLS_DIR)/bin

SKAFFOLD                   := $(TOOLS_BIN_DIR)/skaffold
KUSTOMIZE                  := $(TOOLS_BIN_DIR)/kustomize
GOLANGCI_LINT              := $(TOOLS_BIN_DIR)/golangci-lint
GOIMPORTS                  := $(TOOLS_BIN_DIR)/goimports
CONTROLLER_GEN             := $(TOOLS_BIN_DIR)/controller-gen
CODE_GENERATOR	           := $(TOOLS_BIN_DIR)/code-generator
GINKGO                     := $(TOOLS_BIN_DIR)/ginkgo
GOSEC                      := $(TOOLS_BIN_DIR)/gosec
MOCKGEN                    := $(TOOLS_BIN_DIR)/mockgen
SETUP_ENVTEST              := $(TOOLS_BIN_DIR)/setup-envtest
KIND                       := $(TOOLS_BIN_DIR)/kind
KUBECTL                    := $(TOOLS_BIN_DIR)/kubectl
HELM                       := $(TOOLS_BIN_DIR)/helm
VGOPATH                    := $(TOOLS_BIN_DIR)/vgopath
GO_ADD_LICENSE             := $(TOOLS_BIN_DIR)/addlicense
GOIMPORTS_REVISER          := $(TOOLS_BIN_DIR)/goimports-reviser
YQ						   := $(TOOLS_BIN_DIR)/yq
CRD_REF_DOCS			   := $(TOOLS_BIN_DIR)/crd-ref-docs

# default tool versions
SKAFFOLD_VERSION := v2.15.0
KUSTOMIZE_VERSION := v4.5.7
GOLANGCI_LINT_VERSION ?= v1.64.8
CONTROLLER_GEN_VERSION ?= $(call version_gomod,sigs.k8s.io/controller-tools)
CODE_GENERATOR_VERSION ?= $(call version_gomod,k8s.io/api)
CONTROLLER_RUNTIME_VERSION ?= $(call version_gomod,sigs.k8s.io/controller-runtime)
GINKGO_VERSION ?= $(call version_gomod,github.com/onsi/ginkgo/v2)
GOSEC_VERSION ?= v2.22.2
MOCKGEN_VERSION ?= $(call version_gomod,go.uber.org/mock)
KIND_VERSION ?= v0.27.0
HELM_VERSION ?= v3.17.2
KUBECTL_VERSION ?= v1.32.3
VGOPATH_VERSION ?= v0.1.7
GO_ADD_LICENSE_VERSION ?= v1.1.1
GOIMPORTS_REVISER_VERSION ?= v3.9.1
YQ_VERSION ?= v4.45.1
CRD_REF_DOCS_VERSION ?= v0.1.0
export TOOLS_BIN_DIR := $(TOOLS_BIN_DIR)
export PATH := $(abspath $(TOOLS_BIN_DIR)):$(PATH)

#########################################
# Common                                #
#########################################

# Use this function to get the version of a go module from go.mod
version_gomod = $(shell go list -mod=mod -f '{{ .Version }}' -m $(1))

.PHONY: clean-tools-bin
clean-tools-bin:
	rm -rf $(TOOLS_BIN_DIR)/*

#########################################
# Tools                                 #
#########################################

$(KUSTOMIZE):
	@test -s $(TOOLS_BIN_DIR)/kustomize || GOBIN=$(abspath $(TOOLS_BIN_DIR)) go install sigs.k8s.io/kustomize/kustomize/v4@${KUSTOMIZE_VERSION}

$(GOIMPORTS):
	go build -o $(GOIMPORTS) golang.org/x/tools/cmd/goimports

$(GOLANGCI_LINT):
	@# CGO_ENABLED has to be set to 1 in order for golangci-lint to be able to load plugins
	@# see https://github.com/golangci/golangci-lint/issues/1276
	GOBIN=$(abspath $(TOOLS_BIN_DIR)) CGO_ENABLED=1 go install github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

$(CONTROLLER_GEN):
	GOBIN=$(abspath $(TOOLS_BIN_DIR)) go install sigs.k8s.io/controller-tools/cmd/controller-gen@${CONTROLLER_GEN_VERSION}

CODE_GENERATOR_ROOT = $(shell go env GOMODCACHE)/k8s.io/code-generator@$(CODE_GENERATOR_VERSION)
$(CODE_GENERATOR):
	go build
	GOBIN=$(abspath $(TOOLS_BIN_DIR)) GO111MODULE=on go install k8s.io/code-generator/cmd/client-gen@$(CODE_GENERATOR_VERSION)
	cp -f $(CODE_GENERATOR_ROOT)/kube_codegen.sh $(TOOLS_BIN_DIR)/

$(GINKGO):
	go build -o $(GINKGO) github.com/onsi/ginkgo/v2/ginkgo

$(GOSEC): $(call tool_version_file,$(GOSEC),$(GOSEC_VERSION))
	@GOSEC_VERSION=$(GOSEC_VERSION) $(TOOLS_DIR)/install-gosec.sh

$(MOCKGEN):
	GOBIN=$(abspath $(TOOLS_BIN_DIR)) go install go.uber.org/mock/mockgen@$(MOCKGEN_VERSION)

$(SETUP_ENVTEST):
	curl -Lo $(SETUP_ENVTEST) https://github.com/kubernetes-sigs/controller-runtime/releases/download/$(CONTROLLER_RUNTIME_VERSION)/setup-envtest-$(SYSTEM_NAME)-$(SYSTEM_ARCH)
	chmod +x $(SETUP_ENVTEST)

$(SKAFFOLD):
	curl -Lo $(SKAFFOLD) https://storage.googleapis.com/skaffold/releases/$(SKAFFOLD_VERSION)/skaffold-$(SYSTEM_NAME)-$(SYSTEM_ARCH)
	chmod +x $(SKAFFOLD)

$(KIND):
	curl -Lo $(KIND) https://kind.sigs.k8s.io/dl/$(KIND_VERSION)/kind-$(SYSTEM_NAME)-$(SYSTEM_ARCH)
	chmod +x $(KIND)

$(YQ):
	curl -Lo $(YQ) https://github.com/mikefarah/yq/releases/download/$(YQ_VERSION)/yq_$(SYSTEM_NAME)_$(SYSTEM_ARCH)
	chmod +x $(YQ)

$(HELM):
	curl -sSfL https://raw.githubusercontent.com/helm/helm/master/scripts/get-helm-3 | HELM_INSTALL_DIR=$(TOOLS_BIN_DIR) USE_SUDO=false bash -s -- --version $(HELM_VERSION)

$(VGOPATH):
	go build -o $(VGOPATH) github.com/ironcore-dev/vgopath

$(GO_ADD_LICENSE):
	GOBIN=$(abspath $(TOOLS_BIN_DIR)) go install github.com/google/addlicense@$(GO_ADD_LICENSE_VERSION)

$(KUBECTL):
	curl -Lo $(KUBECTL) https://dl.k8s.io/release/$(KUBECTL_VERSION)/bin/$(SYSTEM_NAME)/$(SYSTEM_ARCH)/kubectl
	chmod +x $(KUBECTL)

$(GOIMPORTS_REVISER):
	GOBIN=$(abspath $(TOOLS_BIN_DIR)) go install github.com/incu6us/goimports-reviser/v3@$(GOIMPORTS_REVISER_VERSION)

$(CRD_REF_DOCS):
	GOBIN=$(abspath $(TOOLS_BIN_DIR)) go install github.com/elastic/crd-ref-docs@$(CRD_REF_DOCS_VERSION)