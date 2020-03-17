# Copyright 2020 The SPDK-CSI Authors.
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
# go source
SOURCE_DIRS := cmd pkg
# goarch for cross building
ifeq ($(origin GOARCH), undefined)
  GOARCH := $(shell go env GOARCH)
endif

# default target
all: spdkcsi test

# build binary
.PHONY: spdkcsi
spdkcsi:
	@echo === building spdkcsi binary
	@CGO_ENABLED=0 GOARCH=$(GOARCH) GOOS=linux go build -o $(OUT_DIR)/spdkcsi ./cmd/

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

.PHONY: clean
clean:
	rm -f $(OUT_DIR)/spdkcsi
	go clean -testcache
