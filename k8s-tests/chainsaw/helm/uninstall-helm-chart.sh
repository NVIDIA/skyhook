#!/bin/bash -xe

# SPDX-FileCopyrightText: Copyright (c) 2025 NVIDIA CORPORATION & AFFILIATES. All rights reserved.
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


OPERATOR_NAME=$1

## need to specify different paths for the helm binary
## depending on whether or not this is being ran in CI
if [ -n "$GITLAB_CI" ]; then
    HELM=/workspace/bin/helm
else 
    HELM=$(which helm)
fi

## remove operator
${HELM} delete $OPERATOR_NAME -n nodewright

## The nodewright CRDs carry helm.sh/resource-policy: keep so a `helm rollback` cannot
## cascade-delete every NodeWright (#464), which means `helm delete` deliberately leaves
## them behind still stamped meta.helm.sh/release-name=$OPERATOR_NAME. Tests in this
## directory share one namespace but install under DIFFERENT release names, so the next
## test's install would fail with "invalid ownership metadata". Drop them here: keeping
## them is a production guarantee, not a test-fixture one.
kubectl delete crd nodewrights.nodewright.nvidia.com \
    deploymentpolicies.nodewright.nvidia.com --ignore-not-found --timeout=120s
