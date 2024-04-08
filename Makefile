# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

# Image URL to use all building/pushing image targets
VERSION             := $(shell cat VERSION)
REPO_ROOT           := $(shell dirname "$(realpath $(lastword $(MAKEFILE_LIST)))")
REGISTRY            := europe-docker.pkg.dev/gardener-project/snapshots
IMAGE_REPOSITORY    := $(REGISTRY)/gardener/etcd-druid
IMAGE_BUILD_TAG     := $(VERSION)
BUILD_DIR           := build
PROVIDERS           := ""
BUCKET_NAME         := "e2e-test"
KUBECONFIG_PATH     :=$(REPO_ROOT)/hack/e2e-test/infrastructure/kind/kubeconfig

IMG ?= ${IMAGE_REPOSITORY}:${IMAGE_BUILD_TAG}

#########################################
# Tools                                 #
#########################################

TOOLS_DIR := hack/tools
include $(REPO_ROOT)/hack/tools.mk

.PHONY: revendor
revendor:
	@env GO111MODULE=on go mod tidy
	@env GO111MODULE=on go mod vendor
	@hack/update-github-templates.sh

kind-up kind-down ci-e2e-kind deploy-localstack test-e2e: export KUBECONFIG = $(KUBECONFIG_PATH)

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
.PHONY: deploy-via-kustomize
deploy-via-kustomize: manifests $(KUSTOMIZE)
	kubectl apply -f config/crd/bases
	kustomize build config/default | kubectl apply -f -

# Deploy controller to the Kubernetes cluster specified in the environment variable KUBECONFIG
# Modify the Helm template located at charts/druid/templates if any changes are required
.PHONY: deploy
deploy: $(SKAFFOLD) $(HELM)
	$(SKAFFOLD) run -m etcd-druid --kubeconfig=$(KUBECONFIG_PATH)

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
clean:
	@hack/clean.sh ./api/... ./controllers/... ./pkg/...

.PHONY: check
check: $(GOLANGCI_LINT) $(GOIMPORTS) $(GO_ADD_LICENSE)
	@hack/check.sh --golangci-lint-config=./.golangci.yaml ./api/... ./pkg/... ./controllers/...
	@hack/check-license-header.sh

.PHONY: check-generate
check-generate:
	@hack/check-generate.sh $(REPO_ROOT)

# Generate code
.PHONY: generate
generate: manifests $(CONTROLLER_GEN) $(GOIMPORTS) $(MOCKGEN)
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
test: $(GINKGO) $(SETUP_ENVTEST)
	@"$(REPO_ROOT)/hack/test.sh" ./api/... ./controllers/... ./pkg/...

.PHONY: test-cov
test-cov: $(GINKGO) $(SETUP_ENVTEST)
	@TEST_COV="true" "$(REPO_ROOT)/hack/test.sh" --skip-package=./test/e2e

.PHONY: test-cov-clean
test-cov-clean:
	@hack/test-cover-clean.sh

.PHONY: test-e2e
test-e2e: $(KUBECTL) $(HELM) $(SKAFFOLD)
	@"$(REPO_ROOT)/hack/e2e-test/run-e2e-test.sh" $(PROVIDERS)

.PHONY: test-integration
test-integration: $(GINKGO) $(SETUP_ENVTEST)
	@"$(REPO_ROOT)/hack/test.sh" ./test/integration/...

.PHONY: update-dependencies
update-dependencies:
	@env GO111MODULE=on go get -u
	@make revendor

.PHONY: add-license-header
add-license-header: $(GO_ADD_LICENSE)
	@./hack/add-license-header.sh ${YEAR}

.PHONY: kind-up
kind-up: $(KIND)
	@printf "\n\033[0;33m📌 NOTE: To target the newly created KinD cluster, please run the following command:\n\n    export KUBECONFIG=$(KUBECONFIG_PATH)\n\033[0m\n"
	./hack/kind-up.sh

.PHONY: kind-down
kind-down: $(KIND)
	$(KIND) delete cluster --name etcd-druid-e2e

.PHONY: deploy-localstack
deploy-localstack: $(KUBECTL)
	./hack/deploy-localstack.sh

.PHONY: ci-e2e-kind
ci-e2e-kind:
	BUCKET_NAME=$(BUCKET_NAME) ./hack/ci-e2e-kind.sh
