# Copyright (c) NVIDIA CORPORATION.  All rights reserved.
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

include deps.mk

## Version of the operator
VERSION ?= $(GIT_TAG_LAST)

# Image URL to use all building/pushing image targets
## TODO: update this to the correct image location
IMG ?= ghcr.io/nvidia/skyhook/operator:latest

## default version of kind to use
KIND_VERSION?=1.30.10

PLATFORM := $(shell uname -s 2>/dev/null || echo unknown)
SKYHOOK_NAMESPACE ?= skyhook

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN := $(shell go env GOPATH)/bin
else
GOBIN := $(shell go env GOBIN)
endif

GIT_SHA 		:= $(shell git rev-parse --short HEAD)
GIT_TAG 		:= $(shell git describe --exact-match --tags 2>/dev/null)
GIT_TAG_LAST 	:= $(shell git describe --tags --abbrev=0 2>/dev/null)

## GO Flags
GO_LDFLAGS  := -ldflags "-X github.com/NVIDIA/skyhook/internal/version.GIT_SHA=$(GIT_SHA) \
	-X github.com/NVIDIA/skyhook/internal/version.VERSION=$(VERSION)"
GOFLAGS 	:= -mod=vendor

# DOCKER_CMD defines the container tool to be used for building images.
# Be aware that the target commands are only tested with Docker which is
# scaffolded by default. However, you might want to replace it to use other
# tools. (i.e. podman)
DOCKER_CMD ?= podman

ifdef CI
	DOCKER_CMD = docker
endif

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk command is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\n\033[1;31mUsage:\033[0m\n  make \033[3;1;36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1;31m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

.PHONY: clean
clean: ## Clears out the local build folder
	chmod -R +w $(LOCALBIN)/* ## some binaries permissions are r+x
	rm -rf $(LOCALBIN)/*
	rm -rf $(REPORTING)/*


define util_confirm_code
$(eval confirm := $(shell read -p "⚠ Are you sure? [y/n] > " -r; echo $$REPLY))
$(if $(filter y Y,$(confirm)),1)
endef
#NOTE: We must call this one to remove \n or any other spaces
util_confirm_ask = $(strip $(util_confirm_code))

.PHONY: confirm
confirm:
	$(if $(util_confirm_ask), echo "User said yes", echo "User said no"; exit 1)

##@ Development

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases
	$(MAKE) license-fmt ## fix up license headers

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object paths="./..."
	$(MAKE) license-fmt ## fix up license headers

.PHONY: generate-mocks 
generate-mocks: mockery ## Generate code for interface mocking
	$(MOCKERY)
	$(MAKE) fmt ## fix up license headers

license-report: go-licenses ## Run run license report
	$(LOCALBIN)/go-licenses report ./... > $(REPORTING)/licenses.csv
	@echo "report at $(REPORTING)/licenses.csv"

.PHONY: license-check
license-check: go-licenses ## Run go-licenses check against code.
	$(LOCALBIN)/go-licenses check  ./... --allowed_licenses=MIT,BSD-2-Clause,BSD-3-Clause,Apache-2.0,ISC,Zlib

.PHONY: license-fmt
license-fmt: ## Run add license header to code.
	python3 ../scripts/format_license.py --root-dir . --license-file ../LICENSE

.PHONY: fmt
fmt: license-fmt ## Run go fmt against code.
	go fmt $(GOFLAGS) ./...


.PHONY: vet
vet: ## Run go vet against code.
	go vet $(GOFLAGS) ./...

REPORTING ?= $(shell pwd)/reporting
.PHONY: reporting
reporting: $(REPORTING)
$(REPORTING):
	mkdir -p $@

.PHONY: test
test:: reporting manifests generate fmt vet lint unit-tests e2e-tests helm-tests ## Run all tests.

ifndef CI
## we double define test so we can do thing different if in ci vs local
test:: merge-coverage
	echo "Total Code Coverage: $(shell go tool cover -func $(REPORTING)/cover.out | grep total | awk '{print $$NF}')"
	go tool cover -html=$(REPORTING)/cover.out
else
test:: merge-coverage gocover-cobertura
	echo "Total Code Coverage: $(shell go tool cover -func $(REPORTING)/cover.out | grep total | awk '{print $$NF}')"
	$(GOCOVER_COBERTURA) < $(REPORTING)/cover.out > $(REPORTING)/coverage.xml
endif

.PHONY: watch-tests
watch-tests: ## watch unit tests and auto run on changes.
	$(GINKGO) watch $(GOFLAGS) -p -vv ./...

.PHONY: unit-test
unit-tests: reporting manifests generate envtest ginkgo kill ## Run unit tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" $(GINKGO) $(GOFLAGS)  --coverprofile=$(REPORTING)/unit.coverprofile -vv --trace --junit-report=$(REPORTING)/unit.xml --keep-going --timeout=180s ./...

## exec time is for things like scripts and commands, if we new more of these, might want to switch to a config file
## https://kyverno.github.io/chainsaw/latest/reference/commands/chainsaw_test/
## https://kyverno.github.io/chainsaw/latest/configuration/file/
CHAINSAW_ARGS:=--exec-timeout 30s --parallel 3

e2e-tests: chainsaw install run ## Run end to end tests.
	## requires a cluster to be running with access
	## locally use kind to create clusters
	## in ci, the plan current is to have a real cluster, and create a node pool for testing
	$(CHAINSAW) test --test-dir ../k8s-tests/chainsaw/skyhook $(CHAINSAW_ARGS)
	$(MAKE) kill 
	go tool covdata textfmt -i=$(REPORTING)/int -o reporting/int.coverprofile

helm-tests: helm chainsaw
	## Here we need to run the operator so that the old CRD can deleted along with
	## any leftover SCRs. Without this the SCRs may have finalizers which rely on
	## the operator and will cause the deletion and tests to hang.
	$(MAKE) run
	$(MAKE) uninstall ignore-not-found=true
	$(MAKE) kill
	$(CHAINSAW) test --test-dir ../k8s-tests/chainsaw/helm $(CHAINSAW_ARGS)


ifeq ($(DOCKER_CMD),docker)
DOCKER_AUTH_FILE=${HOME}/.docker/config.json
else
DOCKER_AUTH_FILE=${HOME}/.config/containers/auth.json
endif

create-kind-cluster: ## deletes and creates a new kind cluster. versions is set via KIND_VERSION
ifdef CI
	@echo $(GITHUB_TOKEN) | docker login ghcr.io -u $(GITHUB_ACTOR) --password-stdin
endif
	kind delete cluster && kind create cluster --image=kindest/node:v$(KIND_VERSION) --config config/local-dev/kind-config.yaml
	$(KUBECTL) label node/kind-worker skyhook.nvidia.com/test-node=skyhooke2e
	
	## sets you local $(DOCKER_CMD) creds into a secret in kind in the skyhook namespace
	$(KUBECTL) create namespace $(SKYHOOK_NAMESPACE) --dry-run=client -o yaml | kubectl apply -f -
	$(KUBECTL) create secret generic node-init-secret --type=kubernetes.io/dockerconfigjson -n $(SKYHOOK_NAMESPACE) \
		--from-file=.dockerconfigjson=$(DOCKER_AUTH_FILE)

podman-create-machine: ## creates a podman machine
	podman machine stop podman-machine-default || true
	podman machine rm -f podman-machine-default || true
	podman machine init --cpus=6 -m=12288 --disk-size=300 podman-machine-default
	podman machine start podman-machine-default

podman-restart:
	podman machine stop && podman machine start

sedrp := sed -r -i
ifeq ($(PLATFORM),Darwin)
	sedrp = sed -r -i ''
endif

.PHONY: merage-coverage
merge-coverage:
	## merge coverage file into one so we can run totals and html reporting
	cat $(REPORTING)/*.coverprofile > reporting/temp-cover.out
	echo "mode: set" > $(REPORTING)/cover.out
	## skip first line with +2
	tail -n +2 $(REPORTING)/temp-cover.out | sed  '/mode: set/d' >> $(REPORTING)/cover.out
	$(sedrp) 's|^/.*skyhook/operator/(.*)$$|github\.com/NVIDIA/skyhook/\1|g' $(REPORTING)/cover.out

.PHONY: lint
lint: golangci-lint license-check ## Run golangci-lint linter & yamllint
	$(GOLANGCI_LINT) run

.PHONY: lint-fix
lint-fix: golangci-lint ## Run golangci-lint linter and perform fixes
	$(GOLANGCI_LINT) run --fix

##@ Build

.PHONY: build
build: manifests generate fmt vet lint ## Build manager binary.
	go build $(GOFLAGS) $(GO_LDFLAGS) -o bin/manager cmd/main.go

.PHONY: run
ENABLE_WEBHOOKS?=false
BACKGROUND?=true
AGENT_IMAGE?=ghcr.io/nvidia/skyhook/agentless:6.2.0
LOG_LEVEL?=info
run: manifests generate fmt vet lint reporting install kill ## Run a controller from your host.
	mkdir -p $(REPORTING)/int
	rm -rf $(REPORTING)/int/*
	go build $(GOFLAGS) -cover $(GO_LDFLAGS) -o $(LOCALBIN)/manager cmd/main.go
ifeq ($(BACKGROUND),true)
	LOG_LEVEL=$(LOG_LEVEL) AGENT_IMAGE=$(AGENT_IMAGE) ENABLE_WEBHOOKS=$(ENABLE_WEBHOOKS) GOCOVERDIR=$(REPORTING)/int nohup $(LOCALBIN)/manager > $(REPORTING)/int/std.out & echo "$$!" > $(REPORTING)/int/run.PID
else 
	LOG_LEVEL=$(LOG_LEVEL) AGENT_IMAGE=$(AGENT_IMAGE) ENABLE_WEBHOOKS=$(ENABLE_WEBHOOKS) GOCOVERDIR=$(REPORTING)/int $(LOCALBIN)/manager > $(REPORTING)/int/std.out
endif

kill:
	! test -s $(REPORTING)/int/run.PID || kill -15 $(shell cat $(REPORTING)/int/run.PID) || rm $(REPORTING)/int/run.PID
	# some times it gets strange, so try to clean that up with a backup pgrep kill
	-pgrep ^manager$$ ; [ "$$?" -ne "0" ] || (echo "kill failed, manager still running"; kill -15 $(shell pgrep ^manager$$))


# If you wish to build the manager image targeting other platforms you can use the --platform flag.
# (i.e. docker build --platform linux/arm64). However, you must enable docker buildKit for it.
# More info: https://docs.docker.com/develop/develop-images/build_enhancements/
.PHONY: docker-build
docker-build: ## Build docker image with the manager.
	$(DOCKER_CMD) build -f ../containers/operator.Dockerfile -t ${IMG} .

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

create-namespace: ## Create the namespace in the K8s cluster specified in ~/.kube/config.
	$(KUBECTL) create namespace $(SKYHOOK_NAMESPACE) --dry-run=client -o yaml | kubectl apply -f -

.PHONY: install
install: manifests kustomize create-namespace license-fmt ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) apply -f -

cert-manager-yaml-url=https://github.com/cert-manager/cert-manager/releases/download/v1.17.0/cert-manager.yaml
.PHONY: install-cert-manager
install-cert-manager: ## Install cert-manager into the K8s cluster specified in ~/.kube/config.
	kubectl apply -f $(cert-manager-yaml-url)
	sleep 30 ## wait for cert-manager to be ready

.PHONY: install-helm-chart
install-helm-chart: helm create-namespace install-cert-manager ## Install helm chart into the K8s cluster specified in ~/.kube/config.
	$(HELM) install skyhook-operator ../chart -n $(SKYHOOK_NAMESPACE) 

.PHONY: uninstall-helm-chart
uninstall-helm-chart: helm ## Uninstall helm chart from the K8s cluster specified in ~/.kube/config.
	$(HELM) uninstall skyhook-operator -n $(SKYHOOK_NAMESPACE)

uninstall-cert-manager: ## Uninstall cert-manager from the K8s cluster specified in ~/.kube/config.
	kubectl delete -f $(cert-manager-yaml-url)

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy
deploy: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | $(KUBECTL) apply -f -

generate-helm: confirm manifests kustomize helm helmify ## Generates new helm chart using helmify. Be-careful, this can break things, it overwrites files, make sure to look at you git diff.
	$(KUSTOMIZE) build config/default | $(HELMIFY) -original-name

.PHONY: undeploy
undeploy: ## Undeploy controller from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/default | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

##@ Build Dependencies

## Tool Binaries
KUBECTL ?= kubectl
