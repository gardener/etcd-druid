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
GINKGO                     := $(TOOLS_BIN_DIR)/ginkgo
MOCKGEN                    := $(TOOLS_BIN_DIR)/mockgen
SETUP_ENVTEST              := $(TOOLS_BIN_DIR)/setup-envtest
KIND                       := $(TOOLS_BIN_DIR)/kind
HELM                       := $(TOOLS_BIN_DIR)/helm
KUBECTL                    := $(TOOLS_BIN_DIR)/kubectl

# default tool versions
SKAFFOLD_VERSION ?= v2.9.0
KUSTOMIZE_VERSION ?= v5.0.0
GOLANGCI_LINT_VERSION ?= v1.57.2
CONTROLLER_GEN_VERSION ?= v0.14.0
KIND_VERSION ?= v0.22.0
MOCKGEN_VERSION ?= $(call version_gomod,go.uber.org/mock)
SETUP_ENVTEST_VERSION ?= $(call version_gomod,sigs.k8s.io/controller-runtime)
HELM_VERSION ?= v3.14.3
KUBECTL_VERSION ?= v1.29.3


export TOOLS_BIN_DIR := $(TOOLS_BIN_DIR)
export PATH := $(abspath $(TOOLS_BIN_DIR)):$(PATH)

# Use this function to get the version of a go module from go.mod
version_gomod = $(shell go list -mod=mod -f '{{ .Version }}' -m $(1))

.PHONY: clean-tools-bin
clean-tools-bin:
	rm -rf $(TOOLS_BIN_DIR)/*

#########################################
# Tools                                 #
#########################################

$(KUSTOMIZE):
	@test -s $(TOOLS_BIN_DIR)/kustomize || GOBIN=$(abspath $(TOOLS_BIN_DIR)) go install sigs.k8s.io/kustomize/kustomize/v5@${KUSTOMIZE_VERSION}

$(GOIMPORTS):
	go build -o $(GOIMPORTS) golang.org/x/tools/cmd/goimports

$(GOLANGCI_LINT):
	@# CGO_ENABLED has to be set to 1 in order for golangci-lint to be able to load plugins
	@# see https://github.com/golangci/golangci-lint/issues/1276
	GOBIN=$(abspath $(TOOLS_BIN_DIR)) CGO_ENABLED=1 go install github.com/golangci/golangci-lint/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

$(CONTROLLER_GEN):
	go install sigs.k8s.io/controller-tools/cmd/controller-gen@${CONTROLLER_GEN_VERSION}

$(GINKGO):
	go build -o $(GINKGO) github.com/onsi/ginkgo/v2/ginkgo

$(MOCKGEN):
	go install go.uber.org/mock/mockgen@$(MOCKGEN_VERSION)

$(SETUP_ENVTEST):
	go install sigs.k8s.io/controller-runtime/tools/setup-envtest@${SETUP_ENVTEST_VERSION}

$(SKAFFOLD):
	curl -Lo $(SKAFFOLD) https://storage.googleapis.com/skaffold/releases/$(SKAFFOLD_VERSION)/skaffold-$(SYSTEM_NAME)-$(SYSTEM_ARCH)
	chmod +x $(SKAFFOLD)

$(KIND):
	curl -Lo $(KIND) https://kind.sigs.k8s.io/dl/$(KIND_VERSION)/kind-$(SYSTEM_NAME)-$(SYSTEM_ARCH)
	chmod +x $(KIND)

$(HELM):
	curl -sSfL https://raw.githubusercontent.com/helm/helm/master/scripts/get-helm-3 | HELM_INSTALL_DIR=$(TOOLS_BIN_DIR) USE_SUDO=false bash -s -- --version $(HELM_VERSION)

$(KUBECTL):
	curl -Lo $(KUBECTL) https://dl.k8s.io/release/$(KUBECTL_VERSION)/bin/$(SYSTEM_NAME)/$(SYSTEM_ARCH)/kubectl
	chmod +x $(KUBECTL)
