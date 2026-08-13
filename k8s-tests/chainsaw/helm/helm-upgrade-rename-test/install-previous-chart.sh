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

## Installs a RELEASED chart as the baseline for the upgrade test. The point is to
## start from in-cluster objects the current templates can no longer produce, which
## a local render cannot reproduce.
##
## The chart is extracted from its git tag rather than pulled from the registry:
## no network dependency, and the baseline is pinned by something already in the
## repo. Same mechanism as k8s-tests/migration/lib.sh:materialize_old_chart, and
## it needs the same thing from CI: full history with tags (actions/checkout with
## fetch-depth: 0 and fetch-tags: true), which the tests job already sets.
##
## The baseline brings its own operator image (its appVersion), so the upgrade
## exercises the real operator-and-chart rollover, not just a template diff.

CHART_TAG=$1
REPO_ROOT=$(git rev-parse --show-toplevel)

if [ -n "$GITLAB_CI" ]; then
    HELM=/workspace/bin/helm
else
    HELM=$(which helm)
fi

if ! git -C "$REPO_ROOT" rev-parse --verify --quiet "${CHART_TAG}^{commit}" >/dev/null; then
    echo "tag ${CHART_TAG} is not present in this checkout; fetch tags with full history"
    exit 1
fi

WORKDIR=$(mktemp -d)
trap 'rm -rf "$WORKDIR"' EXIT
git -C "$REPO_ROOT" archive "$CHART_TAG" chart/ | tar -x -C "$WORKDIR"
[ -f "$WORKDIR/chart/Chart.yaml" ] || { echo "no Chart.yaml after extracting $CHART_TAG"; exit 1; }

${HELM} upgrade --install nodewright "$WORKDIR/chart" \
    -n nodewright --create-namespace \
    --set controllerManager.replicas=1 \
    --set controllerManager.podDisruptionBudget.minAvailable=0 \
    --wait --timeout 5m
