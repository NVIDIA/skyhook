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

SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build ## Build all components.

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\n\033[1;31mUsage:\033[0m\n  make \033[3;1;36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1;31m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Labels

.PHONY: labels
labels: ## Sync GitHub labels from .github/labels.yml (requires gh CLI with repo write access).
	python3 scripts/sync_labels.py

##@ Build

.PHONY: build
build: ## Build operator and agent.
	$(MAKE) -C operator build
	$(MAKE) -C agent build

##@ Test

.PHONY: test
test: ## Run tests for operator and agent.
	$(MAKE) -C operator test
	$(MAKE) -C agent test

##@ Formatting

.PHONY: fmt
fmt: ## Run formatters for operator and agent.
	$(MAKE) -C operator fmt
	$(MAKE) -C agent fmt

.PHONY: license-fmt
license-fmt: ## Run license header formatting for all code.
	python3 scripts/format_license.py --root-dir . --license-file LICENSE

##@ Licenses

.PHONY: notices
notices: ## Regenerate operator/, agent/, and root THIRD_PARTY_NOTICES.md files.
	@python3 scripts/generate-notices.py all

.PHONY: notices-operator
notices-operator: ## Regenerate only operator/THIRD_PARTY_NOTICES.md.
	@python3 scripts/generate-notices.py operator

.PHONY: notices-agent
notices-agent: ## Regenerate only agent/THIRD_PARTY_NOTICES.md.
	@python3 scripts/generate-notices.py agent

.PHONY: notices-rollup
notices-rollup: ## Regenerate only the root THIRD_PARTY_NOTICES.md from component files.
	@python3 scripts/generate-notices.py rollup

##@ Changelog

# Changelogs are generated from git history by scripts/gen-changelog.sh, which
# takes release boundaries from `git tag` (a single `git-cliff --include-path`
# call drops most release sections in this monorepo). CHANGELOG.md is machine-
# owned; hand-authored notes live in the sibling RELEASE_NOTES.md.

.PHONY: changelog
changelog: ## Regenerate a CHANGELOG.md from git history. Interactive: prompts for component + action (regenerate or cut a release). Machine-owned; do not hand-edit.
	@bash scripts/gen-changelog.sh

.PHONY: release-tag
release-tag: ## Interactively cut a release tag: prompts for component + bump (+ optional RC), creates the tag, and optionally pushes it (push triggers the CI release).
	@bash scripts/release-tag.sh

##@ Clean

.PHONY: clean
clean: ## Clean build artifacts for operator and agent.
	$(MAKE) -C operator clean
	$(MAKE) -C agent clean
