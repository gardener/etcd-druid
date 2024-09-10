# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

REPO_ROOT           := $(shell dirname "$(realpath $(lastword $(MAKEFILE_LIST)))")
HACK_DIR            := $(REPO_ROOT)/hack
VERSION             := $(shell $(HACK_DIR)/get-version.sh)
GIT_SHA             := $(shell git rev-parse --short HEAD || echo "GitNotFound")
REGISTRY_ROOT	    := europe-docker.pkg.dev/gardener-project
REGISTRY            := $(REGISTRY_ROOT)/snapshots
IMAGE_NAME          := gardener/etcd-druid
IMAGE_REPOSITORY    := $(REGISTRY)/$(IMAGE_NAME)
IMAGE_BUILD_TAG     := $(VERSION)
BUILD_DIR           := build
PROVIDERS           := ""
BUCKET_NAME         := "e2e-test"
KUBECONFIG_PATH     := $(HACK_DIR)/e2e-test/infrastructure/kind/kubeconfig
TEST_COVER 			:= "true"
IMG ?= ${IMAGE_REPOSITORY}:${IMAGE_BUILD_TAG}

# Tools
# -------------------------------------------------------------------------
TOOLS_DIR := $(HACK_DIR)/tools
include $(HACK_DIR)/tools.mk

# Rules for generation (code and manifests)
# -------------------------------------------------------------------------
.PHONY: check-generate
check-generate:
	@$(HACK_DIR)/check-generate.sh "$(REPO_ROOT)"

# Generate manifests e.g. CRD, RBAC etc.
.PHONY: manifests
manifests: $(VGOPATH) $(CONTROLLER_GEN)
	@HACK_DIR=$(HACK_DIR) VGOPATH=$(VGOPATH) go generate ./config/crd/bases
	@find "$(REPO_ROOT)/config/crd/bases" -name "*.yaml" -exec cp '{}' "$(REPO_ROOT)/charts/druid/charts/crds/templates/" \;
	@controller-gen rbac:roleName=manager-role paths="./internal/controller/..."

# Generate code
.PHONY: generate
generate: manifests $(CONTROLLER_GEN) $(GOIMPORTS) $(MOCKGEN)
	@go generate "$(REPO_ROOT)/internal/..."
	@"$(HACK_DIR)/update-codegen.sh"

# Rules for verification, formatting, linting and cleaning
# -------------------------------------------------------------------------
.PHONY: tidy
tidy:
	@env GO111MODULE=on go mod tidy

.PHONY: clean
clean:
	@$(HACK_DIR)/clean.sh ./api/... ./internal/...

# Clean go mod cache
.PHONY: clean-mod-cache
clean-mod-cache:
	@go clean -modcache

.PHONY: update-dependencies
update-dependencies:
	@env GO111MODULE=on go get -u
	@make tidy

.PHONY: add-license-headers
add-license-headers: $(GO_ADD_LICENSE)
	@$(HACK_DIR)/addlicenseheaders.sh ${YEAR}

# Format code and arrange imports.
.PHONY: format
format: $(GOIMPORTS_REVISER)
	@$(HACK_DIR)/format.sh ./api/ ./internal/ ./test/

# Check packages
.PHONY: check
check: $(GOLANGCI_LINT) $(GOIMPORTS) format manifests
	@$(HACK_DIR)/check.sh --golangci-lint-config=./.golangci.yaml ./api/... ./internal/...

.PHONY: check-apidiff
check-apidiff: $(GO_APIDIFF)
	@$(HACK_DIR)/check-apidiff.sh


# Rules for testing (unit, integration and end-2-end)
# -------------------------------------------------------------------------
# Run tests
.PHONY: test-unit
test-unit: $(GINKGO) $(GOTESTFMT)
	# run ginkgo unit tests. These will be ported to golang native tests over a period of time.
	@TEST_COVER=$(TEST_COVER) "$(HACK_DIR)/test.sh" ./internal/controller/etcdcopybackupstask/... \
	./internal/controller/secret/... \
	./internal/controller/utils/... \
	./internal/mapper/... \
	./internal/metrics/... \
	./internal/health/...
	# run the golang native unit tests.
	@TEST_COVER=$(TEST_COVER) "$(HACK_DIR)/test-go.sh" ./api/... \
	./internal/controller/etcd/... \
	./internal/controller/compaction/... \
	./internal/component/... \
	./internal/errors/... \
	./internal/store/... \
	./internal/utils/... \
	./internal/webhook/...

.PHONY: test-integration
test-integration: $(GINKGO) $(SETUP_ENVTEST) $(GOTESTFMT)
	@SETUP_ENVTEST="true" "$(HACK_DIR)/test.sh" ./test/integration/...
	@SETUP_ENVTEST="true" "$(HACK_DIR)/test-go.sh" ./test/it/...

# Starts a stand alone envtest which you can leverage to test an individual integration-test.
.PHONE: start-envtest
start-envtest: $(SETUP_ENVTEST)
	@$(HACK_DIR)/start-envtest.sh

.PHONY: test-cov-clean
test-cov-clean:
	@$(HACK_DIR)/test-cover-clean.sh

.PHONY: test-e2e
test-e2e: $(KUBECTL) $(HELM) $(SKAFFOLD) $(KUSTOMIZE)
	@VERSION=$(VERSION) GIT_SHA=$(GIT_SHA) $(HACK_DIR)/e2e-test/run-e2e-test.sh $(PROVIDERS)

.PHONY: ci-e2e-kind
ci-e2e-kind: $(GINKGO)
	@BUCKET_NAME=$(BUCKET_NAME) $(HACK_DIR)/ci-e2e-kind.sh

.PHONY: ci-e2e-kind-azure
ci-e2e-kind-azure: $(GINKGO)
	@BUCKET_NAME=$(BUCKET_NAME) $(HACK_DIR)/ci-e2e-kind-azure.sh

# Rules related to binary build, Docker image build and release
# -------------------------------------------------------------------------
# Build manager binary
.PHONY: druid
druid: check
	@env GO111MODULE=on go build -o bin/druid main.go

# Clean go build cache
.PHONY: clean-build-cache
clean-build-cache:
	@go clean -cache

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

# Clean up all docker images for etcd-druid
.PHONY: docker-clean
docker-clean:
	docker images | grep -e "$(REGISTRY_ROOT)/.*/$(IMAGE_NAME)" | awk '{print $$3}' | xargs docker rmi -f

# Rules for locale/remote environment
# -------------------------------------------------------------------------
kind-up kind-down ci-e2e-kind ci-e2e-kind-azure deploy-localstack deploy-azurite test-e2e deploy deploy-dev deploy-debug undeploy: export KUBECONFIG = $(KUBECONFIG_PATH)

.PHONY: kind-up
kind-up: $(KIND)
	@printf "\n\033[0;33m📌 NOTE: To target the newly created KinD cluster, please run the following command:\n\n    export KUBECONFIG=$(KUBECONFIG_PATH)\n\033[0m\n"
	@$(HACK_DIR)/kind-up.sh

.PHONY: kind-down
kind-down: $(KIND)
	$(KIND) delete cluster --name etcd-druid-e2e

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
	@VERSION=$(VERSION) GIT_SHA=$(GIT_SHA) $(SKAFFOLD) run -m etcd-druid

.PHONY: deploy-dev
deploy-dev: $(SKAFFOLD) $(HELM)
	@VERSION=$(VERSION) GIT_SHA=$(GIT_SHA) $(SKAFFOLD) dev --cleanup=false -m etcd-druid --trigger='manual'

.PHONY: deploy-debug
deploy-debug: $(SKAFFOLD) $(HELM)
	@VERSION=$(VERSION) GIT_SHA=$(GIT_SHA) $(SKAFFOLD) debug --cleanup=false -m etcd-druid -p debug

.PHONY: undeploy
undeploy: $(SKAFFOLD) $(HELM)
	$(SKAFFOLD) delete -m etcd-druid

.PHONY: deploy-localstack
deploy-localstack: $(KUBECTL)
	@$(HACK_DIR)/deploy-localstack.sh

.PHONY: deploy-azurite
deploy-azurite: $(KUBECTL)
	./hack/deploy-azurite.sh