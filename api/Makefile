# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

REPO_ROOT           := $(shell dirname "$(realpath $(lastword $(MAKEFILE_LIST)))")
HACK_DIR		    := $(REPO_ROOT)/hack
TOOLS_DIR		    := $(HACK_DIR)/tools

include $(HACK_DIR)/tools.mk

.PHONY: generate
generate: $(CONTROLLER_GEN)
	@$(HACK_DIR)/generate.sh

.PHONY: check-generate
check-generate:
	@$(HACK_DIR)/check-generate.sh "$(REPO_ROOT)"

.PHONY: clean-generated-code
clean-generated-code:
	@$(HACK_DIR)/clean-generated-code.sh

.PHONY: tidy
tidy:
	@env GO111MODULE=on go mod tidy
