# Copyright (c) Arm Limited and Contributors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# output dir
OUT_DIR := ./_out
# dir for tools: e.g., golangci-lint
TOOL_DIR := $(OUT_DIR)/tool
# use golangci-lint for static code check
GOLANGCI_VERSION := v1.49.0
GOLANGCI_BIN := $(TOOL_DIR)/golangci-lint
# go source, scripts
SOURCE_DIRS := cmd pkg
SCRIPT_DIRS := scripts deploy e2e
# goarch for cross building
ifeq ($(origin GOARCH), undefined)
  GOARCH := $(shell go env GOARCH)
endif
# csi image info (spdkcsi/spdkcsi:canary)
ifeq ($(origin CSI_IMAGE_REGISTRY), undefined)
  CSI_IMAGE_REGISTRY := spdkcsi
endif
ifeq ($(origin CSI_IMAGE_TAG), undefined)
  CSI_IMAGE_TAG := canary
endif
CSI_IMAGE := $(CSI_IMAGE_REGISTRY)/spdkcsi:$(CSI_IMAGE_TAG)

# default target
all: spdkcsi lint test

# build binary
.PHONY: spdkcsi
spdkcsi:
	@echo === building spdkcsi binary
	@CGO_ENABLED=0 GOARCH=$(GOARCH) GOOS=linux go build -o $(OUT_DIR)/spdkcsi ./cmd/

# static code check, text lint
lint: golangci yamllint shellcheck

.PHONY: golangci
golangci: $(GOLANGCI_BIN)
	@echo === running golangci-lint
	@$(TOOL_DIR)/golangci-lint --config=scripts/golangci.yml run ./...

$(GOLANGCI_BIN):
	@echo === installing golangci-lint
	@curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | bash -s -- -b $(TOOL_DIR) $(GOLANGCI_VERSION)

.PHONY: yamllint
yamllint:
	@echo === running yamllint
	@if hash yamllint 2> /dev/null; then                     \
	     yamllint -s -c scripts/yamllint.yml $(SCRIPT_DIRS); \
	     bash scripts/verify-helm-yamllint.sh;               \
	 else                                                    \
	     echo yamllint not installed, skip test;             \
	 fi

.PHONY: shellcheck
shellcheck:
	@echo === running shellcheck
	@find $(SCRIPT_DIRS) -name "*.sh" -type f | xargs bash -n
	@if hash shellcheck 2> /dev/null; then                               \
	     find $(SCRIPT_DIRS) -name "*.sh" -type f | xargs shellcheck -x; \
	 else                                                                \
	     echo shellcheck not installed, skip test;                       \
	 fi

# tests
test: mod-check unit-test

.PHONY: mod-check
mod-check:
	@echo === runnng go mod verify
	@go mod verify

.PHONY: unit-test
unit-test:
	@echo === running unit test
	@go test -cover $(foreach d,$(SOURCE_DIRS),./$(d)/...)

# e2e test
.PHONY: e2e-test
e2e-test:
	@echo === running e2e test
	@go test -timeout 30m ./e2e

# helm test
.PHONY: helm-test
helm-test:
	@echo === running helm test
	@./scripts/install-helm.sh up
	@./scripts/install-helm.sh install-spdkcsi
	@./scripts/install-helm.sh cleanup-spdkcsi
	@./scripts/install-helm.sh clean

# docker image
image: spdkcsi
	@echo === running docker build
	@if [ -n $(HTTP_PROXY) ]; then \
		proxy_opt="--build-arg http_proxy=$(HTTP_PROXY) --build-arg https_proxy=$(HTTP_PROXY)"; \
	fi; \
	docker build -t $(CSI_IMAGE) $$proxy_opt \
	-f deploy/image/Dockerfile $(OUT_DIR)

.PHONY: clean
clean:
	rm -f $(OUT_DIR)/spdkcsi
	go clean -testcache
