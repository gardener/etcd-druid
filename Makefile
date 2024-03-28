# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

ENSURE_GARDENER_MOD := $(shell go get github.com/gardener/gardener@$$(go list -m -f "{{.Version}}" github.com/gardener/gardener))
GARDENER_HACK_DIR   := $(shell go list -m -f "{{.Dir}}" github.com/gardener/gardener)/hack
VERSION             := $(shell cat VERSION)
REPO_ROOT           := $(shell dirname "$(realpath $(lastword $(MAKEFILE_LIST)))")
HACK_DIR            := $(REPO_ROOT)/hack
REGISTRY            := europe-docker.pkg.dev/gardener-project/snapshots
IMAGE_REPOSITORY    := $(REGISTRY)/gardener/etcd-druid
IMAGE_BUILD_TAG     := $(VERSION)
BUILD_DIR           := build
PROVIDERS           := ""
BUCKET_NAME         := "e2e-test"
KUBECONFIG_PATH     := $(HACK_DIR)/e2e-test/infrastructure/kind/kubeconfig

IMG ?= ${IMAGE_REPOSITORY}:${IMAGE_BUILD_TAG}

#########################################
# Tools                                 #
#########################################

TOOLS_DIR := $(HACK_DIR)/tools
include $(GARDENER_HACK_DIR)/tools.mk
include $(HACK_DIR)/tools.mk

.PHONY: tidy
tidy:
	@env GO111MODULE=on go mod tidy
	@GARDENER_HACK_DIR=$(GARDENER_HACK_DIR) bash $(HACK_DIR)/update-github-templates.sh
	@cp $(GARDENER_HACK_DIR)/cherry-pick-pull.sh $(HACK_DIR)/cherry-pick-pull.sh && chmod +xw $(HACK_DIR)/cherry-pick-pull.sh

kind-up kind-down ci-e2e-kind deploy-localstack test-e2e: export KUBECONFIG = $(KUBECONFIG_PATH)

all: druid

# Build manager binary
.PHONY: druid
druid: fmt check
	@env GO111MODULE=on go build -o bin/druid main.go

# Run against the configured Kubernetes cluster in ~/.kube/config
.PHONY: run
run:
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
manifests: $(VGOPATH) $(CONTROLLER_GEN)
	@GARDENER_HACK_DIR=$(GARDENER_HACK_DIR) VGOPATH=$(VGOPATH) go generate ./config/crd/bases
	@find "$(REPO_ROOT)/config/crd/bases" -name "*.yaml" -exec cp '{}' "$(REPO_ROOT)/charts/druid/charts/crds/templates/" \;
	@controller-gen rbac:roleName=manager-role paths="./internal/controller/..."

# Run go fmt against code
.PHONY: fmt
fmt:
	@env GO111MODULE=on go fmt ./...

clean:
	@"$(REPO_ROOT)/vendor/github.com/gardener/gardener/hack/clean.sh" ./api/... ./internal/...

# Check packages
.PHONY: check
check: $(GOLANGCI_LINT) $(GOIMPORTS) set-permissions fmt manifests
	@"$(REPO_ROOT)/vendor/github.com/gardener/gardener/hack/check.sh" --golangci-lint-config=./.golangci.yaml ./api/... ./internal/...

.PHONY: check-generate
check-generate:
	@bash $(GARDENER_HACK_DIR)/check-generate.sh "$(REPO_ROOT)"

# Generate code
.PHONY: generate
generate: set-permissions manifests $(CONTROLLER_GEN) $(GOIMPORTS) $(MOCKGEN)
	@go generate "$(REPO_ROOT)/internal/..."
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
test: $(GINKGO)
	# run ginkgo unit tests. These will be ported to golang native tests over a period of time.
	@"$(REPO_ROOT)/hack/test.sh" ./api/... \
	./internal/controller/etcdcopybackupstask/... \
	./internal/controller/predicate/... \
	./internal/controller/secret/... \
	./internal/controller/utils/... \
	./internal/mapper/... \
	./internal/metrics/...
	# run the golang native unit tests.
	@go test -v -coverprofile cover.out ./internal/controller/etcd/... ./internal/operator/... ./internal/utils/... ./internal/webhook/...
	@go tool cover -func=cover.out

.PHONY: test-cov
test-cov: $(GINKGO) $(SETUP_ENVTEST)
	@TEST_COV="true" bash $(HACK_DIR)/test.sh --skip-package=./test/e2e

.PHONY: test-cov-clean
test-cov-clean:
	@bash $(GARDENER_HACK_DIR)/test-cover-clean.sh

.PHONY: test-e2e
test-e2e: $(KUBECTL) $(HELM) $(SKAFFOLD) $(KUSTOMIZE)
	@bash $(HACK_DIR)/e2e-test/run-e2e-test.sh $(PROVIDERS)

.PHONY: test-integration
test-integration: set-permissions $(GINKGO) $(SETUP_ENVTEST)
	@"$(REPO_ROOT)/hack/test.sh" ./test/integration/...
	@export KUBEBUILDER_ASSETS="$(${SETUP_ENVTEST} --arch=amd64 use --use-env -p path 1.22)"
	@echo "using envtest tools installed at '${KUBEBUILDER_ASSETS}'"
	@export KUBEBUILDER_CONTROLPLANE_START_TIMEOUT=2m
	@go test -v ./test/it/...

.PHONY: update-dependencies
update-dependencies:
	@env GO111MODULE=on go get -u
	@make tidy

.PHONY: add-license-headers
add-license-headers: $(GO_ADD_LICENSE)
	@bash $(HACK_DIR)/addlicenseheaders.sh ${YEAR}

.PHONY: kind-up
kind-up: $(KIND)
	@printf "\n\033[0;33m📌 NOTE: To target the newly created KinD cluster, please run the following command:\n\n    export KUBECONFIG=$(KUBECONFIG_PATH)\n\033[0m\n"
	@GARDENER_HACK_DIR=$(GARDENER_HACK_DIR) bash $(HACK_DIR)/kind-up.sh

.PHONY: kind-down
kind-down: $(KIND)
	$(KIND) delete cluster --name etcd-druid-e2e

.PHONY: deploy-localstack
deploy-localstack: $(KUBECTL)
	@bash $(HACK_DIR)/deploy-localstack.sh

.PHONY: ci-e2e-kind
ci-e2e-kind:
	@BUCKET_NAME=$(BUCKET_NAME) bash $(HACK_DIR)/ci-e2e-kind.sh
