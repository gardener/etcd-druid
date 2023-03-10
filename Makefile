# Copyright (c) 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Image URL to use all building/pushing image targets
VERSION             := $(shell cat VERSION)
REPO_ROOT           := $(shell dirname "$(realpath $(lastword $(MAKEFILE_LIST)))")
REGISTRY            := eu.gcr.io/gardener-project/gardener
IMAGE_REPOSITORY    := $(REGISTRY)/etcd-druid
IMAGE_BUILD_TAG     := $(VERSION)
BUILD_DIR           := build
PROVIDERS           := ""

IMG ?= ${IMAGE_REPOSITORY}:${IMAGE_BUILD_TAG}

#########################################
# Tools                                 #
#########################################

TOOLS_DIR := hack/tools
include $(REPO_ROOT)/hack/tools.mk
include $(REPO_ROOT)/vendor/github.com/gardener/gardener/hack/tools.mk


.PHONY: set-permissions
set-permissions:
	@chmod +x "$(REPO_ROOT)/vendor/github.com/gardener/gardener/hack/clean.sh"
	@chmod +x "$(REPO_ROOT)/vendor/github.com/gardener/gardener/hack/check.sh"
	@chmod +x "$(REPO_ROOT)/vendor/github.com/gardener/gardener/hack/check-generate.sh"
	@chmod +x "$(REPO_ROOT)/vendor/github.com/gardener/gardener/hack/generate-crds.sh"

.PHONY: revendor
revendor: set-permissions
	@env GO111MODULE=on go mod tidy
	@env GO111MODULE=on go mod vendor
	@"$(REPO_ROOT)/hack/update-github-templates.sh"
	@make set-permissions

all: druid

# Build manager binary
.PHONY: druid
druid: fmt check
	@env GO111MODULE=on go build -o bin/druid main.go

# Run against the configured Kubernetes cluster in ~/.kube/config
.PHONY: run
run: fmt check
	go run ./main.go

# Install CRDs into a cluster
.PHONY: install
install: manifests
	kubectl apply -f config/crd/bases

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
.PHONY: deploy
deploy: manifests $(KUSTOMIZE)
	kubectl apply -f config/crd/bases
	kustomize build config/default | kubectl apply -f -

# Generate manifests e.g. CRD, RBAC etc.
.PHONY: manifests
manifests: $(CONTROLLER_GEN)
	@go generate ./config/crd/bases
	@find "$(REPO_ROOT)/config/crd/bases" -name "*.yaml" -exec cp '{}' "$(REPO_ROOT)/charts/druid/charts/crds/templates/" \;
	@controller-gen rbac:roleName=manager-role paths="./controllers/..."

# Run go fmt against code
.PHONY: fmt
fmt:
	@env GO111MODULE=on go fmt ./...

.PHONY: clean
clean: set-permissions
	@"$(REPO_ROOT)/vendor/github.com/gardener/gardener/hack/clean.sh" ./api/... ./controllers/... ./pkg/...

# Check packages
.PHONY: check
check: $(GOLANGCI_LINT) $(GOIMPORTS) set-permissions
	@"$(REPO_ROOT)/vendor/github.com/gardener/gardener/hack/check.sh" --golangci-lint-config=./.golangci.yaml ./api/... ./pkg/... ./controllers/...

.PHONY: check-generate
check-generate: set-permissions
	@"$(REPO_ROOT)/vendor/github.com/gardener/gardener/hack/check-generate.sh" "$(REPO_ROOT)"

# Generate code
.PHONY: generate
generate: set-permissions $(CONTROLLER_GEN) $(GOIMPORTS) $(MOCKGEN)
	@go generate "$(REPO_ROOT)/pkg/..."
	@"$(REPO_ROOT)/hack/update-codegen.sh"

# Build the docker image
.PHONY: docker-build
docker-build:
	docker build . -t ${IMG} --rm
	@echo "updating kustomize image patch file for manager resource"
	sed -i'' -e 's@image: .*@image: '"${IMG}"'@' ./config/default/manager_image_patch.yaml

# Push the docker image
.PHONY: docker-push
docker-push:
	docker push ${IMG}

# Run tests
.PHONY: test
test: set-permissions $(GINKGO) $(SETUP_ENVTEST) fmt check manifests
	@"$(REPO_ROOT)/hack/test.sh" ./api/... ./controllers/... ./pkg/...

.PHONY: test-cov
test-cov: set-permissions $(GINKGO) $(SETUP_ENVTEST)
	@TEST_COV="true" "$(REPO_ROOT)/hack/test.sh" --skip-package=./test/e2e

.PHONY: test-cov-clean
test-cov-clean: set-permissions
	@"$(REPO_ROOT)/vendor/github.com/gardener/gardener/hack/test-cover-clean.sh"

.PHONY: test-e2e
test-e2e: set-permissions $(KUBECTL) $(HELM) $(SKAFFOLD)
	@"$(REPO_ROOT)/hack/e2e-test/run-e2e-test.sh" $(PROVIDERS)

.PHONY: test-integration
test-integration: set-permissions $(GINKGO) $(SETUP_ENVTEST) fmt check manifests
	@"$(REPO_ROOT)/hack/test.sh" ./test/integration/...

.PHONY: update-dependencies
update-dependencies:
	@env GO111MODULE=on go get -u
	@make revendor

.PHONY: add-license-headers
add-license-headers: $(GO_ADD_LICENSE)
	@./hack/addlicenseheaders.sh ${YEAR}
