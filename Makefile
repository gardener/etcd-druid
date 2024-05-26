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

#####################################################################
# Rules for verification, formatting, linting, testing and cleaning #
#####################################################################

.PHONY: tidy
tidy:
	@env GO111MODULE=on go mod tidy
	@GARDENER_HACK_DIR=$(GARDENER_HACK_DIR) bash $(HACK_DIR)/update-github-templates.sh
	@cp $(GARDENER_HACK_DIR)/cherry-pick-pull.sh $(HACK_DIR)/cherry-pick-pull.sh && chmod +xw $(HACK_DIR)/cherry-pick-pull.sh

.PHONY: clean
clean:
	@bash $(GARDENER_HACK_DIR)/clean.sh ./api/... ./internal/...

.PHONY: update-dependencies
update-dependencies:
	@env GO111MODULE=on go get -u
	@make tidy

.PHONY: add-license-headers
add-license-headers: $(GO_ADD_LICENSE)
	@bash $(HACK_DIR)/addlicenseheaders.sh ${YEAR}

# Run go fmt against code
.PHONY: fmt
fmt:
	@env GO111MODULE=on go fmt ./...

# Check packages
.PHONY: check
check: $(GOLANGCI_LINT) $(GOIMPORTS) fmt manifests
	@REPO_ROOT=$(REPO_ROOT) bash $(GARDENER_HACK_DIR)/check.sh --golangci-lint-config=./.golangci.yaml ./api/... ./internal/...

.PHONY: check-generate
check-generate:
	@bash $(GARDENER_HACK_DIR)/check-generate.sh "$(REPO_ROOT)"

# Generate manifests e.g. CRD, RBAC etc.
.PHONY: manifests
manifests: $(VGOPATH) $(CONTROLLER_GEN)
	@GARDENER_HACK_DIR=$(GARDENER_HACK_DIR) VGOPATH=$(VGOPATH) go generate ./config/crd/bases
	@find "$(REPO_ROOT)/config/crd/bases" -name "*.yaml" -exec cp '{}' "$(REPO_ROOT)/charts/druid/charts/crds/templates/" \;
	@controller-gen rbac:roleName=manager-role paths="./internal/controller/..."

# Generate code
.PHONY: generate
generate: manifests $(CONTROLLER_GEN) $(GOIMPORTS) $(MOCKGEN)
	@go generate "$(REPO_ROOT)/internal/..."
	@"$(HACK_DIR)/update-codegen.sh"

# Run tests
.PHONY: test
test: $(GINKGO) $(GOTESTFMT)
	# run ginkgo unit tests. These will be ported to golang native tests over a period of time.
	@"$(HACK_DIR)/test.sh" ./internal/controller/etcdcopybackupstask/... \
	./internal/controller/predicate/... \
	./internal/controller/secret/... \
	./internal/controller/utils/... \
	./internal/mapper/... \
	./internal/metrics/...
	# run the golang native unit tests.
	@TEST_COV="true" "$(HACK_DIR)/test-go.sh" ./api/... ./internal/controller/etcd/... ./internal/controller/compaction/... ./internal/component/... ./internal/utils/... ./internal/webhook/...

.PHONY: test-integration
test-integration: $(GINKGO) $(SETUP_ENVTEST) $(GOTESTFMT)
	@SETUP_ENVTEST="true" "$(HACK_DIR)/test.sh" ./test/integration/...
	@SETUP_ENVTEST="true" "$(HACK_DIR)/test-go.sh" ./test/it/...

.PHONY: test-cov
test-cov: $(GINKGO) $(SETUP_ENVTEST)
	@TEST_COV="true" bash $(HACK_DIR)/test.sh --skip-package=./test/e2e

.PHONY: test-cov-clean
test-cov-clean:
	@bash $(GARDENER_HACK_DIR)/test-cover-clean.sh

#################################################################
# Rules related to binary build, Docker image build and release #
#################################################################

# Build manager binary
.PHONY: druid
druid: fmt check
	@env GO111MODULE=on go build -o bin/druid main.go

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

#####################################################################
# Rules for local environment                                       #
#####################################################################

kind-up kind-down ci-e2e-kind deploy-localstack test-e2e deploy deploy-dev deploy-debug undeploy: export KUBECONFIG = $(KUBECONFIG_PATH)

.PHONY: kind-up
kind-up: $(KIND)
	@printf "\n\033[0;33m📌 NOTE: To target the newly created KinD cluster, please run the following command:\n\n    export KUBECONFIG=$(KUBECONFIG_PATH)\n\033[0m\n"
	@GARDENER_HACK_DIR=$(GARDENER_HACK_DIR) bash $(HACK_DIR)/kind-up.sh

.PHONY: kind-down
kind-down: $(KIND)
	$(KIND) delete cluster --name etcd-druid-e2e

# Install CRDs into a cluster
.PHONY: install
install: manifests
	kubectl apply -f config/crd/bases

# Run against the configured Kubernetes cluster in ~/.kube/config or specified by environment variable KUBECONFIG
.PHONY: run
run:
	go run ./main.go

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
.PHONY: deploy-via-kustomize
deploy-via-kustomize: manifests $(KUSTOMIZE)
	kubectl apply -f config/crd/bases
	kustomize build config/default | kubectl apply -f -

# Deploy controller to the Kubernetes cluster specified in the environment variable KUBECONFIG
# Modify the Helm template located at charts/druid/templates if any changes are required
.PHONY: deploy
deploy: $(SKAFFOLD) $(HELM)
	$(SKAFFOLD) run -m etcd-druid

.PHONY: deploy-dev
deploy-dev: $(SKAFFOLD) $(HELM)
	$(SKAFFOLD) dev -m etcd-druid --trigger='manual'

.PHONY: deploy-debug
deploy-debug: $(SKAFFOLD) $(HELM)
	$(SKAFFOLD) debug -m etcd-druid

.PHONY: undeploy
undeploy: $(SKAFFOLD) $(HELM)
	$(SKAFFOLD) delete -m etcd-druid

.PHONY: deploy-localstack
deploy-localstack: $(KUBECTL)
	@bash $(HACK_DIR)/deploy-localstack.sh

.PHONY: test-e2e
test-e2e: $(KUBECTL) $(HELM) $(SKAFFOLD) $(KUSTOMIZE)
	@bash $(HACK_DIR)/e2e-test/run-e2e-test.sh $(PROVIDERS)

.PHONY: ci-e2e-kind
ci-e2e-kind:
	@BUCKET_NAME=$(BUCKET_NAME) bash $(HACK_DIR)/ci-e2e-kind.sh
