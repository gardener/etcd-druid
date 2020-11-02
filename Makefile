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
REPO_ROOT           := $(shell dirname $(realpath $(lastword $(MAKEFILE_LIST))))
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
	@cd $(REPO_ROOT)/api && go mod tidy
	@env GO111MODULE=on go mod tidy
	@env GO111MODULE=on go mod vendor
	@chmod +x $(REPO_ROOT)/vendor/github.com/gardener/gardener/hack/.ci/*


all: druid

# Run tests
test: fmt vet manifests
	@env GO111MODULE=on go test ./api_tests/... ./controllers/... -coverprofile cover.out

# Build manager binary
druid: fmt vet
	@env GO111MODULE=on go build -o bin/druid main.go

# Run against the configured Kubernetes cluster in ~/.kube/config
run: fmt vet
	go run ./main.go

# Install CRDs into a cluster
install: manifests
	kubectl apply -f config/crd/bases

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
deploy: manifests
	kubectl apply -f config/crd/bases
	kustomize build config/default | kubectl apply -f -

# Generate manifests e.g. CRD, RBAC etc.
manifests: controller-gen
	cd $(REPO_ROOT)/api && $(CONTROLLER_GEN) $(CRD_OPTIONS) paths="./..." output:crd:artifacts:config=../config/crd/bases
	$(CONTROLLER_GEN) rbac:roleName=manager-role paths="./controllers/..."

# Run go fmt against code
fmt:
	@env GO111MODULE=on go fmt ./...

# Run go vet against code
vet:
	PACKAGES="$(shell GO111MODULE=on go list -mod=vendor -e ./... | grep -vE '/api|/api/v1alpha1')"
	@env GO111MODULE=on go vet $(PACKAGES)

# Generate code
generate: controller-gen
	cd $(REPO_ROOT)/api && $(CONTROLLER_GEN) object:headerFile=../hack/boilerplate.go.txt paths=./...

# Build the docker image
docker-build:
	docker build . -t ${IMG} --rm
	@echo "updating kustomize image patch file for manager resource"
	sed -i'' -e 's@image: .*@image: '"${IMG}"'@' ./config/default/manager_image_patch.yaml

# Push the docker image
docker-push:
	docker push ${IMG}

# find or download controller-gen
# download controller-gen if necessary
controller-gen:
ifeq (, $(shell which controller-gen))
	go get sigs.k8s.io/controller-tools/cmd/controller-gen@v0.2.5
CONTROLLER_GEN=$(shell go env GOPATH)/bin/controller-gen
else
CONTROLLER_GEN=$(shell which controller-gen)
endif

.PHONY: update-dependencies
update-dependencies:
	@env GO111MODULE=on go get -u
	@make revendor
