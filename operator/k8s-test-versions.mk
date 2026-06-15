## Copyright (c) NVIDIA CORPORATION.  All rights reserved.

## Licensed under the Apache License, Version 2.0 (the "License");
## you may not use this file except in compliance with the License.
## You may obtain a copy of the License at

##     http://www.apache.org/licenses/LICENSE-2.0

## Unless required by applicable law or agreed to in writing, software
## distributed under the License is distributed on an "AS IS" BASIS,
## WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
## See the License for the specific language governing permissions and
## limitations under the License.

## Kubernetes versions used by envtest, local kind clusters, and CI.
## ENVTEST_K8S_VERSION owns setup-envtest binary assets.
ENVTEST_K8S_VERSION ?= 1.36.0
export ENVTEST_K8S_VERSION

## KIND_VERSION is retained for existing local overrides. Kind does not publish
## every Kubernetes patch version, so this default may intentionally differ
## from ENVTEST_K8S_VERSION.
KIND_VERSION ?= 1.35.0
KIND_NODE_IMAGE_VERSION ?= $(KIND_VERSION)
KIND_BINARY_VERSION ?= v0.31.0
export KIND_VERSION
export KIND_NODE_IMAGE_VERSION
export KIND_BINARY_VERSION

## GitHub Actions matrix values. Kind does not publish every Kubernetes patch
## version, so these are intentionally separate from ENVTEST_K8S_VERSION.
CI_KIND_NODE_IMAGE_VERSIONS_JSON ?= ["1.32.11","1.33.7","1.34.3","1.35.0"]
CI_PRIMARY_KIND_NODE_IMAGE_VERSION ?= $(KIND_NODE_IMAGE_VERSION)
export CI_KIND_NODE_IMAGE_VERSIONS_JSON
export CI_PRIMARY_KIND_NODE_IMAGE_VERSION
