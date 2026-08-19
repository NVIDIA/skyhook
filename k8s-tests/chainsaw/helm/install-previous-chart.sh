#!/bin/bash -xe

# SPDX-FileCopyrightText: Copyright (c) 2026 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
# SPDX-License-Identifier: Apache-2.0
#
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

## Installs a PUBLISHED chart as the baseline for the tests that cross a release
## boundary (helm-upgrade-rollback-test). The point is to start from in-cluster
## objects the current templates can no longer produce, which a local render
## cannot reproduce.
##
## It pulls the same artifact users install, from the same registry, rather than
## rendering a chart directory out of the repo: the thing under test is an upgrade
## or rollback between two real releases. The baseline also brings its own operator
## image (its appVersion), so this exercises the operator-and-chart rollover, not
## just a template diff.
##
## Usage:
##   install-previous-chart.sh v0.17.1   # a specific release
##   install-previous-chart.sh           # the newest published release
##
## Git is used only to answer "which version", never to source the chart. Callers
## that must land on a specific release pass it explicitly; the rename tests do,
## because their baseline has to stay on the pre-rename side of the boundary and
## would assert nothing against a newer one.

CHART_REPO=oci://ghcr.io/nvidia/nodewright/charts/nodewright
CHART_VERSION=${1:-}

if [ -n "$GITLAB_CI" ]; then
    HELM=/workspace/bin/helm
else
    HELM=$(which helm)
fi

if [ -z "${CHART_VERSION}" ]; then
    ## Newest released chart, from the tags rather than from the registry: `helm show`
    ## cannot list tags on an OCI repository. Pre-releases are excluded because they
    ## are not what a user upgrading from "the last release" would have installed.
    ## Needs full history with tags in CI (actions/checkout with fetch-depth: 0 and
    ## fetch-tags: true), which the tests job already sets.
    CHART_VERSION=$(git tag --list 'chart/v*' --sort=-v:refname \
        | grep -vE -- '-(rc|alpha|beta|test)' \
        | head -1 \
        | sed 's|^chart/||')
    if [ -z "${CHART_VERSION}" ]; then
        echo "no chart/v* tags in this checkout; fetch tags with full history or pass a version"
        exit 1
    fi
fi

## Tolerate being handed the git tag instead of the version it names.
CHART_VERSION=${CHART_VERSION#chart/}

${HELM} upgrade --install nodewright "${CHART_REPO}" --version "${CHART_VERSION}" \
    -n nodewright --create-namespace \
    --set controllerManager.replicas=1 \
    --set controllerManager.podDisruptionBudget.minAvailable=0 \
    --wait --timeout 5m
