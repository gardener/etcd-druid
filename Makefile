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
IMAGE_TAG           := $(VERSION)
BUILD_DIR           := build
BIN_DIR             := bin

IMG ?= ${IMAGE_REPOSITORY}:${IMAGE_TAG}
# Produce CRDs that work back to Kubernetes 1.11 (no version conversion)
CRD_OPTIONS ?= "crd:trivialVersions=true"

.PHONY: revendor
revendor:
	@cd "$(REPO_ROOT)/api" && go mod tidy
	@env GO111MODULE=on go mod vendor
	@env GO111MODULE=on go mod tidy
	@chmod +x "$(REPO_ROOT)/vendor/github.com/gardener/gardener/hack/"*
	@chmod +x "$(REPO_ROOT)/vendor/github.com/gardener/gardener/hack/.ci/"*
	@"$(REPO_ROOT)/hack/update-github-templates.sh"

all: druid

# Run tests
.PHONY: test
test: fmt check manifests
	.ci/test

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
deploy: manifests
	kubectl apply -f config/crd/bases
	kustomize build config/default | kubectl apply -f -

# Generate manifests e.g. CRD, RBAC etc.
.PHONY: manifests
manifests: install-requirements
	@cd "$(REPO_ROOT)/api" && controller-gen $(CRD_OPTIONS) paths="./..." output:crd:artifacts:config=../config/crd/bases
	@controller-gen rbac:roleName=manager-role paths="./controllers/..."

# Run go fmt against code
.PHONY: fmt
fmt:
	@env GO111MODULE=on go fmt ./...

# Check packages
.PHONY: check
check:
	@cd "$(REPO_ROOT)/api" && "$(REPO_ROOT)/vendor/github.com/gardener/gardener/hack/check.sh" --golangci-lint-config=../.golangci.yaml ./...
	@"$(REPO_ROOT)/vendor/github.com/gardener/gardener/hack/check.sh" --golangci-lint-config=./.golangci.yaml ./pkg/... ./controllers/...

# Generate code
.PHONY: generate
generate: install-requirements
	cd "$(REPO_ROOT)/api" && controller-gen object:headerFile=../hack/boilerplate.go.txt paths=./...

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

# find or download controller-gen
# download controller-gen if necessary
.PHONY: install-requirements
install-requirements:
	@go install -mod=vendor sigs.k8s.io/controller-tools/cmd/controller-gen
	@"$(REPO_ROOT)/vendor/github.com/gardener/gardener/hack/install-requirements.sh" > /dev/null

.PHONY: update-dependencies
update-dependencies:
	@env GO111MODULE=on go get -u
	@make revendor
