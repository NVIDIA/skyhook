SHELL := /bin/bash

BINARY ?= skyhook
BIN_DIR ?= bin
OUTPUT ?= $(BIN_DIR)/$(BINARY)
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
GIT_SHA ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
LDFLAGS ?= -s -w \
	-X github.com/NVIDIA/skyhook/plugin/pkg/version.Version=$(VERSION) \
	-X github.com/NVIDIA/skyhook/plugin/pkg/version.GitSHA=$(GIT_SHA)

.PHONY: all build install clean test test-unit test-coverage fmt vet help

all: fmt vet test build  ## Run all checks and build

help:  ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)


##@ Testing

test: test-unit  ## Run all tests

test-unit:  ## Run unit tests
	go test -v -race -coverprofile=coverage.out ./...

test-coverage: test-unit  ## Run tests and open coverage report
	go tool cover -html=coverage.out

##@ Build

build:  ## Build binary
	@mkdir -p $(BIN_DIR)
	go build -ldflags "$(LDFLAGS)" -o $(OUTPUT) .

install:  ## Install binary to GOPATH
	go install -ldflags "$(LDFLAGS)" .

##@ Cleanup

clean:  ## Clean build artifacts
	rm -rf $(BIN_DIR)
	rm -f coverage.out

