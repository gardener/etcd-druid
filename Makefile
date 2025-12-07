# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

REPO_ROOT           := $(shell dirname "$(realpath $(lastword $(MAKEFILE_LIST)))")
HACK_DIR            := $(REPO_ROOT)/hack
API_HACK_DIR        := $(REPO_ROOT)/api/hack
VERSION             := $(shell $(HACK_DIR)/get-version.sh)
REGISTRY_ROOT       := europe-docker.pkg.dev/gardener-project
REGISTRY            := $(REGISTRY_ROOT)/snapshots
IMAGE_NAME          := gardener/etcd-druid
IMAGE_REPOSITORY    := $(REGISTRY)/$(IMAGE_NAME)
IMAGE_BUILD_TAG     := $(VERSION)
PLATFORM            ?= $(shell docker info --format '{{.OSType}}/{{.Architecture}}')
PROVIDERS           := ""
BUCKET_NAME         := "e2e-test"
IMG                 ?= ${IMAGE_REPOSITORY}:${IMAGE_BUILD_TAG}
TEST_COVER          := "true"
KUBECONFIG_PATH     := $(HACK_DIR)/kind/kubeconfig

# Tools
# -------------------------------------------------------------------------
TOOLS_DIR := $(HACK_DIR)/tools
include $(HACK_DIR)/tools.mk

ifndef CERT_EXPIRY_DAYS
override CERT_EXPIRY_DAYS = 365
endif

# Rules for verification, formatting, linting and cleaning
# -------------------------------------------------------------------------
.PHONY: tidy
tidy:
	@env GO111MODULE=on go mod tidy

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
	@$(HACK_DIR)/addlicenseheaders.sh

# Format code and arrange imports.
.PHONY: format
format: $(GOIMPORTS_REVISER)
	@$(HACK_DIR)/format.sh ./cmd/ ./internal/ ./test/ ./examples/

# Check packages
.PHONY: check
check: $(GOLANGCI_LINT) $(GOIMPORTS) format
	@$(HACK_DIR)/check.sh --golangci-lint-config=./.golangci.yaml ./internal/...

# Check license headers
.PHONY: check-license-headers
check-license-headers: $(GO_ADD_LICENSE)
	@$(HACK_DIR)/check-license-headers.sh

# Check git status is clean
.PHONY: check-git-status
check-git-status:
	@if [[ -n $$(git status --porcelain) ]]; then \
		echo "Repository is dirty. Please commit or stash changes."; \
		git status; \
		git diff; \
		exit 1; \
	else \
		echo "Repository is clean ✓"; \
	fi

.PHONY: sast
sast: $(GOSEC)
	@./hack/sast.sh

.PHONY: sast-report
sast-report: $(GOSEC)
	@./hack/sast.sh --gosec-report true

# Rules for testing (unit, integration and end-2-end)
# -------------------------------------------------------------------------
# Run tests
.PHONY: test-unit
test-unit: $(GINKGO)
	# run ginkgo unit tests. These will be ported to golang native tests over a period of time.
	@TEST_COVER=$(TEST_COVER) "$(HACK_DIR)/test.sh" ./internal/controller/etcdcopybackupstask/... \
	./internal/controller/utils/... \
	./internal/mapper/... \
	./internal/metrics/... \
	./internal/health/... \
	./internal/utils/imagevector/...

	# run the golang native unit tests.
	@TEST_COVER=$(TEST_COVER) "$(HACK_DIR)/test-go.sh" \
	./internal/controller/etcd/... \
	./internal/controller/etcdopstask/... \
	./internal/controller/secret/... \
	./internal/controller/compaction/... \
	./internal/component/... \
	./internal/errors/... \
	./internal/store/... \
	./internal/utils/... \
	./internal/webhook/...

.PHONY: test-integration
test-integration: $(GINKGO) $(SETUP_ENVTEST)
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
test-e2e: $(KUBECTL) $(HELM) $(SKAFFOLD) $(KUSTOMIZE) $(GINKGO)
	@$(HACK_DIR)/prepare-chart-resources.sh -n $(BUCKET_NAME) -e $(CERT_EXPIRY_DAYS)
	@$(HACK_DIR)/e2e-test/run-e2e-test.sh $(PROVIDERS)

.PHONY: ci-e2e-kind
ci-e2e-kind: $(GINKGO) $(YQ) $(KIND)
	@BUCKET_NAME=$(BUCKET_NAME) $(HACK_DIR)/ci-e2e-kind.sh

.PHONY: ci-e2e-kind-azure
ci-e2e-kind-azure: $(GINKGO)
	@BUCKET_NAME=$(BUCKET_NAME) $(HACK_DIR)/ci-e2e-kind-azure.sh

.PHONY: ci-e2e-kind-gcs
ci-e2e-kind-gcs: $(GINKGO)
	@BUCKET_NAME=$(BUCKET_NAME) $(HACK_DIR)/ci-e2e-kind-gcs.sh

# Rules related to binary build, Docker image build and release
# -------------------------------------------------------------------------
# Build manager binary
.PHONY: build
build:
	@GO111MODULE=on CGO_ENABLED=0 go build \
		-v \
		-o bin/etcd-druid \
		-ldflags "$$(bash -c 'source $(HACK_DIR)/ld-flags.sh && build_ld_flags')" \
		cmd/main.go

# Clean go build cache
.PHONY: clean-build-cache
clean-build-cache:
	@go clean -cache

# Build the docker image
.PHONY: docker-build
docker-build:
	docker buildx build --platform=$(PLATFORM) --tag $(IMG) --rm .

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
kind-up kind-down ci-e2e-kind ci-e2e-kind-azure ci-e2e-kind-gcs deploy-localstack deploy-fakegcs deploy-azurite test-e2e deploy deploy-dev deploy-debug undeploy: export KUBECONFIG = $(KUBECONFIG_PATH)

ifndef CLUSTER_NAME
override CLUSTER_NAME = etcd-druid-e2e
endif

.PHONY: kind-up
kind-up: $(KIND)
	@$(HACK_DIR)/kind-up.sh --cluster-name $(CLUSTER_NAME)
	@printf "\n\033[0;33m📌 NOTE: To target the newly created KinD cluster, please run the following command:\n\n    export KUBECONFIG=$(KUBECONFIG_PATH)\n\033[0m\n"

.PHONY: kind-down
kind-down: $(KIND)
	@$(HACK_DIR)/kind-down.sh --cluster-name $(CLUSTER_NAME)

# Make targets to deploy etcd-druid operator using skaffold
# --------------------------------------------------------------------------------------------------------
# Deploy controller to the Kubernetes cluster specified in the environment variable KUBECONFIG
# Modify the Helm template located at charts/druid/templates if any changes are required

ifndef NAMESPACE
override NAMESPACE = default
endif

# While preparing helm charts, it will by default attempt to get the k8s version by invoking kubectl command assuming that you are already connected to a k8s cluster.
# If you wish to specify a specific k8s version, then set K8S_VERSION environment variable to a value of your choice.
.PHONY: prepare-helm-charts
prepare-helm-charts:
ifeq ($(strip $(K8S_VERSION)),)
	@$(HACK_DIR)/prepare-chart-resources.sh --namespce $(NAMESPACE) --cert-expiry $(CERT_EXPIRY_DAYS)
else
	@$(HACK_DIR)/prepare-chart-resources.sh --namespce $(NAMESPACE) --cert-expiry $(CERT_EXPIRY_DAYS) --k8s-version $(K8S_VERSION)
endif

.PHONY: deploy
deploy: $(SKAFFOLD) $(HELM) prepare-helm-charts
	@$(HACK_DIR)/deploy-local.sh run -m etcd-druid -n $(NAMESPACE)

.PHONY: deploy-dev
deploy-dev: $(SKAFFOLD) $(HELM) prepare-helm-charts
	@$(HACK_DIR)/deploy-local.sh dev --cleanup=false -m etcd-druid --trigger='manual' -n $(NAMESPACE)

.PHONY: deploy-debug
deploy-debug: $(SKAFFOLD) $(HELM) prepare-helm-charts
	@$(HACK_DIR)/deploy-local.sh debug --cleanup=false -m etcd-druid -p debug -n $(NAMESPACE)

.PHONY: undeploy
undeploy: $(SKAFFOLD) $(HELM)
	$(SKAFFOLD) delete -m etcd-druid -n $(NAMESPACE)

.PHONY: deploy-localstack
deploy-localstack: $(KUBECTL)
	@$(HACK_DIR)/deploy-localstack.sh

.PHONY: deploy-azurite
deploy-azurite: $(KUBECTL)
	@$(HACK_DIR)/deploy-azurite.sh

.PHONY: deploy-fakegcs
deploy-fakegcs: $(KUBECTL)
	@$(HACK_DIR)/deploy-fakegcs.sh

.PHONY: clean-chart-resources
clean-chart-resources:
	@rm -f $(REPO_ROOT)/charts/crds/*.yaml
	@rm -rf $(REPO_ROOT)/charts/pki-resources/*
