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
REGISTRY ?= nvcr.io
AGENT_IMAGE ?= nvidian/swgpu-baseos/skyhook-agent

.PHONY: all
all: venv test

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\n\033[1;31mUsage:\033[0m\n  make \033[3;1;36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1;31m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

.PHONY: venv
venv: ## Sets up a python venv at `./venv`
	python3 -m venv venv
	$(VENV)pip install hatch coverage
	$(VENV)hatch config set dirs.project "[\"$(shell pwd)\"]"

##@ Test
.PHONY: test
test: venv ## Test using hatch, prints coverage and outputs a report to coverage.xml
	$(VENV)hatch -p skyhook-agent test --cover-quiet 
	$(VENV)coverage report --show-missing --data-file=skyhook-agent/.coverage
##	$(VENV)coverage xml --data-file=skyhook-agent/.coverage

.PHONY: format
format:
	for file in $(find . -name "*.py" | grep -v venv | grep -v pycache); do \
		echo $(file); \
		python3 tools/license_formatter.py boilerplate/license_header.py.txt $(file) > $(file).tmp; \
		mv $(file).tmp $(file); \
	done

##@ Build
.PHONY: build
build: ## Builds using hatch to `dist`
	$(VENV)hatch -p skyhook-agent version $(build_version)
	$(VENV)hatch -p skyhook-agent build -c

docker-setup:
	test ! $($(DOCKER_CMD) context ls | grep builder) || $(DOCKER_CMD) context create builder;
	$(DOCKER_CMD) buildx create --platform linux/amd64,linux/arm64 --use builder
	$(DOCKER_CMD) run --privileged --rm tonistiigi/binfmt --install amd64,arm64

ACTUAL_TAGS=$(shell echo "-t $(REGISTRY)/$(AGENT_IMAGE):$(shell date +%y.%m.%d-%H%M%S)-$(COMMIT_SHORT_SHA) $(TAGS)" | tr A-Z a-z)
.PHONY: docker-build-only
docker-build-only:
	@echo "Building skyhook-agent $(DOCKER_CMD) image with tags: $(ACTUAL_TAGS)"
	$(DOCKER_CMD) buildx build $(BUILD_ARGS) --build-arg AGENT_VERSION=$(AGENT_VERSION) --platform linux/amd64,linux/arm64 $(ACTUAL_TAGS) --metadata-file=metadata.json -f ../containers/agent.Dockerfile .

##@ Vendor
.PHONY: vendor
vendor: ## Uses Unearth to vendor all dependencies locally.
	python3 -m venv ./venv_vendor
	./venv_vendor/bin/pip install unearth toml
	dependencies=$(shell python -c 'import toml; print(" ".join(toml.loads(open("skyhook-agent/pyproject.toml","r").read())["project"]["dependencies"]))')
	rm -rf vendor
	mkdir -p vendor
	for dep in $(dependencies); do \
		./venv_vendor/bin/unearth --no-binary -d ./vendor $(dep) >> vendor/lock_file; \
	done

##@ Clean
.PHONY: clean
clean: ## Removes all pycache and venv directories from your local copy.
	rm -rf __pycache__
	rm -rf venv
	rm -rf venv_vendor
