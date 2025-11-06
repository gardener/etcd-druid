PROJECT_ROOT := $(shell pwd)
BIN_DIR := $(PROJECT_ROOT)/bin
BIN_NAME := druid
PLUGIN_BIN_NAME := kubectl-druid
GO_FILES := $(shell find . -name '*.go' -not -path './vendor/*')

GO ?= go
GOLANGCI_LINT ?= golangci-lint
INSTALL_DIR ?= $(or $(GOBIN),$(HOME)/.local/bin)


.PHONY: build
build: $(BIN_DIR)/$(BIN_NAME) $(BIN_DIR)/$(PLUGIN_BIN_NAME)

$(BIN_DIR)/$(BIN_NAME): $(GO_FILES) go.mod
	@mkdir -p $(BIN_DIR)
	$(GO) build -o $(BIN_DIR)/$(BIN_NAME) .

$(BIN_DIR)/$(PLUGIN_BIN_NAME): $(BIN_DIR)/$(BIN_NAME)
	cp $(BIN_DIR)/$(BIN_NAME) $(BIN_DIR)/$(PLUGIN_BIN_NAME)

.PHONY: plugin-install
plugin-install: build
	@mkdir -p $(INSTALL_DIR)
	cp $(BIN_DIR)/$(PLUGIN_BIN_NAME) $(INSTALL_DIR)/$(PLUGIN_BIN_NAME)
	@echo "Installed $(PLUGIN_BIN_NAME) to $(INSTALL_DIR)"
	@echo "Ensure \"$(INSTALL_DIR)\" is in your PATH. Test with: kubectl druid --help"

.PHONY: tidy
tidy:
	$(GO) mod tidy

.PHONY: deps
deps:
	$(GO) mod verify

.PHONY: fmt
fmt:
	$(GO) fmt ./...

.PHONY: lint
lint:
	@if command -v $(GOLANGCI_LINT) >/dev/null 2>&1; then \
		echo "Running golangci-lint"; \
		$(GOLANGCI_LINT) run ./... || true; \
	else \
		echo "golangci-lint not found (skip). Install: https://golangci-lint.run"; \
	fi

.PHONY: vet
vet:
	$(GO) vet ./...

.PHONY: test
test:
	$(GO) test -race -coverprofile=coverage.out ./...
	@echo "Coverage report generated at coverage.out"

.PHONY: clean
clean:
	rm -rf $(BIN_DIR) coverage.out
