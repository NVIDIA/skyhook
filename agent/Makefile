# Copyright (c) NVIDIA CORPORATION.  All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#	 http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

VENV := ./venv/bin/
REGISTRY_IMAGE = nvcr.io/nvidian/swgpu-baseos/skyhook-agent

.PHONY: all
all: venv test

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\n\033[1;31mUsage:\033[0m\n  make \033[3;1;36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1;31m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

.PHONY: venv
venv: ## Sets up a python venv at `./venv`
	python3 -m venv venv
	$(VENV)pip install hatch coverage
	$(VENV)hatch config set dirs.project "[\"${PWD}\"]"

##@ Test
.PHONY: test
test: venv ## Test using hatch, prints coverage and outputs a report to coverage.xml
	$(VENV)hatch -p skyhook-agent test --cover-quiet
	$(VENV)coverage report --show-missing --data-file=skyhook-agent/.coverage
	$(VENV)coverage xml --data-file=skyhook-agent/.coverage

.PHONY: format
format:
	for file in $(find . -name "*.py" | grep -v venv | grep -v pycache); do \
		echo $(file); \
		python3 tools/license_formatter.py boilerplate/license_header.py.txt $(file) > $(file).tmp; \
		mv $(file).tmp $(file); \
	done

##@ Build
.PHONY: build
build: ## Builds using hatch to `skyhook-agent/dist`
	$(VENV)hatch -p skyhook-agent version ${1:unkown}
	$(VENV)hatch -p skyhook-agent build -c

##@ Publish
.PHONY: publish
publish: ## Publishes using hatch
	$(VENV)hatch -p skyhook-agent publish

BUILD_ARGS ?= 
ifndef GITLAB_CI
BUILD_ARGS = --push
COMMIT_SHORT_SHA := $(shell git rev-parse --short HEAD)
endif

docker-setup:
	test ! $(docker context ls | grep builder) || docker context create builder;
	docker buildx create --platform linux/amd64,linux/arm64 --use builder
	docker run --privileged --rm tonistiigi/binfmt --install amd64,arm64
	@TAGS="-t ${REGISTRY_IMAGE}:$(shell date +%y.%m.%d-%H%M%S)-$(COMMIT_SHORT_SHA)"

.PHONY: docker-build
docker-build docker-setup ## Builds skyhook-agent docker image using docker buildx
	docker buildx build $(BUILD_ARGS) --platform linux/amd64,linux/arm64 $(TAGS) -f docker/Dockerfile 

.PHONY: clean
clean:
	rm -rf __pycache__
	rm -rf venv